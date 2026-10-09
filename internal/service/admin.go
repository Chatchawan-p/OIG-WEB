package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/oig-police/oig-web/internal/apperror"
	"github.com/oig-police/oig-web/internal/authz"
	"github.com/oig-police/oig-web/internal/model"
	"github.com/oig-police/oig-web/internal/platform/id"
	"github.com/oig-police/oig-web/internal/repository"
)

type RuleCommand struct {
	Name        string
	Category    string
	Fine        string
	Jail        string
	Description string
}

type AdminService struct {
	users    repository.UserRepository
	catalog  repository.CatalogRepository
	audits   repository.AuditRepository
	metadata repository.MetadataRepository
	policy   *authz.Policy
	now      Clock
}

func NewAdminService(store repository.Store, policy *authz.Policy, now Clock) *AdminService {
	return &AdminService{users: store, catalog: store, audits: store, metadata: store, policy: policy, now: now}
}

func validateRule(command RuleCommand) (RuleCommand, error) {
	validation := &apperror.ValidationError{}
	fields := []struct {
		name  string
		value *string
		max   int
		req   bool
	}{
		{"name", &command.Name, 300, true}, {"category", &command.Category, 200, true},
		{"fine", &command.Fine, 200, false}, {"jail", &command.Jail, 200, false}, {"description", &command.Description, 4000, false},
	}
	for _, field := range fields {
		value, err := trimLimited(*field.value, field.max)
		*field.value = value
		if err != nil || (field.req && value == "") {
			message := fmt.Sprintf("must not exceed %d characters", field.max)
			if field.req {
				message = fmt.Sprintf("is required and must not exceed %d characters", field.max)
			}
			validation.Add(field.name, message)
		}
	}
	return command, validation.OrNil()
}

func (s *AdminService) ListRules(ctx context.Context, actor model.User) ([]model.PenaltyRule, error) {
	if !s.policy.Can(actor.Role, authz.ReadCatalog) {
		return nil, apperror.ErrForbidden
	}
	return s.catalog.ListPenaltyRules(ctx)
}

func (s *AdminService) CreateRule(ctx context.Context, actor model.User, command RuleCommand) (model.PenaltyRule, error) {
	if !s.policy.Can(actor.Role, authz.WriteCatalog) {
		return model.PenaltyRule{}, apperror.ErrForbidden
	}
	command, err := validateRule(command)
	if err != nil {
		return model.PenaltyRule{}, err
	}
	ruleID, err := id.New("rul")
	if err != nil {
		return model.PenaltyRule{}, err
	}
	now := s.now()
	rule := model.PenaltyRule{RuleID: ruleID, Name: command.Name, Category: command.Category, Fine: command.Fine, Jail: command.Jail, Description: command.Description, CreatedAt: now, UpdatedAt: now}
	audit, err := newAudit(actor, "penalty_rule.created", "penalty_rule", ruleID, "created penalty rule "+rule.Name, now)
	if err != nil {
		return model.PenaltyRule{}, err
	}
	if err := s.catalog.CreatePenaltyRuleWithAudit(ctx, rule, audit); err != nil {
		return model.PenaltyRule{}, err
	}
	return rule, nil
}

func (s *AdminService) UpdateRule(ctx context.Context, actor model.User, ruleID string, command RuleCommand) (model.PenaltyRule, error) {
	if !s.policy.Can(actor.Role, authz.WriteCatalog) {
		return model.PenaltyRule{}, apperror.ErrForbidden
	}
	command, err := validateRule(command)
	if err != nil {
		return model.PenaltyRule{}, err
	}
	now := s.now()
	rule := model.PenaltyRule{RuleID: ruleID, Name: command.Name, Category: command.Category, Fine: command.Fine, Jail: command.Jail, Description: command.Description, UpdatedAt: now}
	audit, err := newAudit(actor, "penalty_rule.updated", "penalty_rule", ruleID, "updated penalty rule "+rule.Name, now)
	if err != nil {
		return model.PenaltyRule{}, err
	}
	updated, err := s.catalog.UpdatePenaltyRuleWithAudit(ctx, rule, audit)
	if err != nil {
		return model.PenaltyRule{}, err
	}
	return updated, nil
}

func (s *AdminService) DeleteRule(ctx context.Context, actor model.User, ruleID string) error {
	if !s.policy.Can(actor.Role, authz.WriteCatalog) {
		return apperror.ErrForbidden
	}
	now := s.now()
	audit, err := newAudit(actor, "penalty_rule.deleted", "penalty_rule", ruleID, "deleted penalty rule", now)
	if err != nil {
		return err
	}
	return s.catalog.DeletePenaltyRuleWithAudit(ctx, ruleID, audit)
}

func (s *AdminService) ListUsers(ctx context.Context, actor model.User, page repository.Page) ([]model.User, int64, error) {
	if !s.policy.Can(actor.Role, authz.ReadUsers) {
		return nil, 0, apperror.ErrForbidden
	}
	return s.users.ListUsers(ctx, page)
}

func parseRole(value string) (model.Role, bool) {
	switch model.Role(strings.TrimSpace(value)) {
	case model.RoleOwner, model.RoleScreener, model.RoleAdmin, model.RoleGuest:
		return model.Role(strings.TrimSpace(value)), true
	default:
		return "", false
	}
}

func (s *AdminService) UpdateUserRole(ctx context.Context, actor model.User, targetID, roleValue string) (model.User, error) {
	if !s.policy.Can(actor.Role, authz.WriteUserRoles) {
		return model.User{}, apperror.ErrForbidden
	}
	role, ok := parseRole(roleValue)
	if !ok {
		return model.User{}, &apperror.ValidationError{Details: []apperror.FieldError{{Field: "role", Message: "must be Screener, Admin, or Guest"}}}
	}
	target, err := s.users.FindUserByID(ctx, targetID)
	if err != nil {
		return model.User{}, err
	}
	if !s.policy.CanChangeRole(actor, target, role) {
		return model.User{}, apperror.ErrForbidden
	}
	now := s.now()
	audit, err := newAudit(actor, "user.role_changed", "user", targetID, fmt.Sprintf("changed role from %s to %s", target.Role, role), now)
	if err != nil {
		return model.User{}, err
	}
	return s.users.UpdateUserRoleWithAudit(ctx, targetID, role, now, audit)
}

func (s *AdminService) ListAuditLogs(ctx context.Context, actor model.User, filter repository.AuditFilter) ([]model.AuditLog, int64, error) {
	if !s.policy.Can(actor.Role, authz.ReadAudit) {
		return nil, 0, apperror.ErrForbidden
	}
	return s.audits.ListAuditLogs(ctx, filter)
}

func (s *AdminService) Departments(ctx context.Context, actor model.User) ([]string, error) {
	if !s.policy.Can(actor.Role, authz.ReadMetadata) {
		return nil, apperror.ErrForbidden
	}
	return s.metadata.ListDepartments(ctx)
}
