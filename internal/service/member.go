package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/oig-police/oig-web/internal/apperror"
	"github.com/oig-police/oig-web/internal/authz"
	"github.com/oig-police/oig-web/internal/model"
	"github.com/oig-police/oig-web/internal/platform/id"
	"github.com/oig-police/oig-web/internal/repository"
)

const defaultEmploymentStatus = "รับราชการ"

var allowedEmploymentStatuses = map[string]struct{}{
	"รับราชการ": {},
	"ปลดราชการ": {},
}

type MemberCommand struct {
	PoliceID         string
	Generation       int
	Rank             string
	MedicalPrefix    string
	FullName         string
	EmploymentStatus string
	Department       string
	Unit             string
}

type MemberPatch struct {
	Generation    *int
	Rank          *string
	MedicalPrefix *string
	FullName      *string
	Department    *string
	Unit          *string
}

type MemberService struct {
	repo   memberStore
	policy *authz.Policy
	now    Clock
}

type memberStore interface {
	repository.MemberRepository
	RecalculateMemberPenaltyCount(context.Context, string, time.Time) (int, error)
	RecalculateAllPenaltyCounts(context.Context, time.Time) error
}

func NewMemberService(repo memberStore, policy *authz.Policy, now Clock) *MemberService {
	return &MemberService{repo: repo, policy: policy, now: now}
}

func (s *MemberService) List(ctx context.Context, actor model.User, filter repository.MemberFilter) ([]model.PoliceMember, int64, error) {
	if !s.policy.Can(actor.Role, authz.ReadMembers) {
		return nil, 0, apperror.ErrForbidden
	}
	if err := s.repo.RecalculateAllPenaltyCounts(ctx, s.now()); err != nil {
		return nil, 0, err
	}
	return s.repo.ListMembers(ctx, filter)
}

func (s *MemberService) Get(ctx context.Context, actor model.User, memberID string) (model.PoliceMember, []model.NameHistory, error) {
	if !s.policy.Can(actor.Role, authz.ReadMembers) {
		return model.PoliceMember{}, nil, apperror.ErrForbidden
	}
	member, err := s.repo.FindMemberByID(ctx, memberID)
	if err != nil {
		return model.PoliceMember{}, nil, err
	}
	history, err := s.repo.ListNameHistory(ctx, memberID)
	return member, history, err
}

func (s *MemberService) GetByPoliceID(ctx context.Context, actor model.User, policeID string) (model.PoliceMember, error) {
	if !s.policy.Can(actor.Role, authz.ReadMembers) {
		return model.PoliceMember{}, apperror.ErrForbidden
	}
	member, err := s.repo.FindMemberByPoliceID(ctx, strings.TrimSpace(policeID))
	if err != nil {
		return model.PoliceMember{}, err
	}
	count, err := s.repo.RecalculateMemberPenaltyCount(ctx, member.MemberID, s.now())
	if err != nil {
		return model.PoliceMember{}, err
	}
	member.PenaltyCount = count
	return member, nil
}

func validateMemberCommand(command MemberCommand) (MemberCommand, error) {
	validation := &apperror.ValidationError{}
	var err error
	command.PoliceID, err = trimLimited(command.PoliceID, 64)
	if err != nil || command.PoliceID == "" {
		validation.Add("policeId", "is required and must not exceed 64 characters")
	}
	if command.Generation < 1 || command.Generation > 9999 {
		validation.Add("generation", "must be between 1 and 9999")
	}
	command.FullName, err = trimLimited(command.FullName, 200)
	if err != nil || command.FullName == "" {
		validation.Add("fullName", "is required and must not exceed 200 characters")
	}
	fields := []struct {
		name  string
		value *string
		max   int
	}{{"rank", &command.Rank, 100}, {"medicalPrefix", &command.MedicalPrefix, 100}, {"department", &command.Department, 200}, {"unit", &command.Unit, 200}}
	for _, field := range fields {
		*field.value, err = trimLimited(*field.value, field.max)
		if err != nil {
			validation.Add(field.name, err.Error())
		}
	}
	command.EmploymentStatus = strings.TrimSpace(command.EmploymentStatus)
	if command.EmploymentStatus == "" {
		command.EmploymentStatus = defaultEmploymentStatus
	}
	if _, ok := allowedEmploymentStatuses[command.EmploymentStatus]; !ok {
		validation.Add("employmentStatus", "must be รับราชการ or ปลดราชการ")
	}
	return command, validation.OrNil()
}

func (s *MemberService) Create(ctx context.Context, actor model.User, command MemberCommand) (model.PoliceMember, error) {
	if !s.policy.Can(actor.Role, authz.WriteMembers) {
		return model.PoliceMember{}, apperror.ErrForbidden
	}
	command, err := validateMemberCommand(command)
	if err != nil {
		return model.PoliceMember{}, err
	}
	memberID, err := id.New("mem")
	if err != nil {
		return model.PoliceMember{}, err
	}
	now := s.now()
	member := model.PoliceMember{MemberID: memberID, PoliceID: command.PoliceID, Generation: command.Generation, Rank: command.Rank, MedicalPrefix: command.MedicalPrefix, FullName: command.FullName, EmploymentStatus: command.EmploymentStatus, Department: command.Department, Unit: command.Unit, CreatedAt: now, UpdatedAt: now}
	audit, err := newAudit(actor, "member.created", "police_member", memberID, fmt.Sprintf("created member %s", command.PoliceID), now)
	if err != nil {
		return model.PoliceMember{}, err
	}
	if err := s.repo.CreateMemberWithAudit(ctx, member, audit); err != nil {
		return model.PoliceMember{}, err
	}
	return member, nil
}

func (s *MemberService) BulkCreate(ctx context.Context, actor model.User, commands []MemberCommand) ([]model.PoliceMember, error) {
	if !s.policy.Can(actor.Role, authz.WriteMembers) {
		return nil, apperror.ErrForbidden
	}
	if len(commands) == 0 || len(commands) > 100 {
		return nil, &apperror.ValidationError{Details: []apperror.FieldError{{Field: "members", Message: "must contain between 1 and 100 members"}}}
	}
	seen := make(map[string]struct{}, len(commands))
	members := make([]model.PoliceMember, 0, len(commands))
	audits := make([]model.AuditLog, 0, len(commands))
	now := s.now()
	for i, raw := range commands {
		command, err := validateMemberCommand(raw)
		if err != nil {
			var validation *apperror.ValidationError
			if errors.As(err, &validation) {
				for j := range validation.Details {
					validation.Details[j].Field = fmt.Sprintf("members[%d].%s", i, validation.Details[j].Field)
				}
			}
			return nil, err
		}
		if _, duplicate := seen[command.PoliceID]; duplicate {
			return nil, &apperror.ValidationError{Details: []apperror.FieldError{{Field: fmt.Sprintf("members[%d].policeId", i), Message: "duplicate police ID in request"}}}
		}
		seen[command.PoliceID] = struct{}{}
		memberID, err := id.New("mem")
		if err != nil {
			return nil, err
		}
		member := model.PoliceMember{MemberID: memberID, PoliceID: command.PoliceID, Generation: command.Generation, Rank: command.Rank, MedicalPrefix: command.MedicalPrefix, FullName: command.FullName, EmploymentStatus: command.EmploymentStatus, Department: command.Department, Unit: command.Unit, CreatedAt: now, UpdatedAt: now}
		audit, err := newAudit(actor, "member.created", "police_member", memberID, fmt.Sprintf("bulk-created member %s", command.PoliceID), now)
		if err != nil {
			return nil, err
		}
		members = append(members, member)
		audits = append(audits, audit)
	}
	if err := s.repo.BulkCreateMembersWithAudit(ctx, members, audits); err != nil {
		return nil, err
	}
	return members, nil
}

func (s *MemberService) Update(ctx context.Context, actor model.User, memberID string, patch MemberPatch) (model.PoliceMember, error) {
	if !s.policy.Can(actor.Role, authz.WriteMembers) {
		return model.PoliceMember{}, apperror.ErrForbidden
	}
	member, err := s.repo.FindMemberByID(ctx, memberID)
	if err != nil {
		return model.PoliceMember{}, err
	}
	if patch.Generation != nil {
		member.Generation = *patch.Generation
	}
	if patch.Rank != nil {
		member.Rank = *patch.Rank
	}
	if patch.MedicalPrefix != nil {
		member.MedicalPrefix = *patch.MedicalPrefix
	}
	oldName := member.FullName
	if patch.FullName != nil {
		member.FullName = *patch.FullName
	}
	if patch.Department != nil {
		member.Department = *patch.Department
	}
	if patch.Unit != nil {
		member.Unit = *patch.Unit
	}
	validated, err := validateMemberCommand(MemberCommand{PoliceID: member.PoliceID, Generation: member.Generation, Rank: member.Rank, MedicalPrefix: member.MedicalPrefix, FullName: member.FullName, EmploymentStatus: member.EmploymentStatus, Department: member.Department, Unit: member.Unit})
	if err != nil {
		return model.PoliceMember{}, err
	}
	member.Generation, member.Rank, member.MedicalPrefix, member.FullName, member.Department, member.Unit = validated.Generation, validated.Rank, validated.MedicalPrefix, validated.FullName, validated.Department, validated.Unit
	now := s.now()
	member.UpdatedAt = now
	var history *model.NameHistory
	if oldName != member.FullName {
		historyID, err := id.New("nam")
		if err != nil {
			return model.PoliceMember{}, err
		}
		history = &model.NameHistory{HistoryID: historyID, MemberID: member.MemberID, PoliceID: member.PoliceID, OldName: oldName, NewName: member.FullName, Generation: member.Generation, ChangedAt: now, ChangedBy: actor.UserID}
	}
	action := "member.updated"
	if history != nil {
		action = "member.name_changed"
	}
	audit, err := newAudit(actor, action, "police_member", member.MemberID, fmt.Sprintf("updated member %s", member.PoliceID), now)
	if err != nil {
		return model.PoliceMember{}, err
	}
	if err := s.repo.UpdateMemberWithHistoryAndAudit(ctx, member, history, audit); err != nil {
		return model.PoliceMember{}, err
	}
	return member, nil
}

func (s *MemberService) UpdateStatus(ctx context.Context, actor model.User, memberID, status string) (model.PoliceMember, error) {
	if !s.policy.Can(actor.Role, authz.WriteMembers) {
		return model.PoliceMember{}, apperror.ErrForbidden
	}
	status = strings.TrimSpace(status)
	if _, ok := allowedEmploymentStatuses[status]; !ok {
		return model.PoliceMember{}, &apperror.ValidationError{Details: []apperror.FieldError{{Field: "employmentStatus", Message: "must be รับราชการ or ปลดราชการ"}}}
	}
	now := s.now()
	audit, err := newAudit(actor, "member.status_changed", "police_member", memberID, "changed employment status to "+status, now)
	if err != nil {
		return model.PoliceMember{}, err
	}
	return s.repo.UpdateMemberStatusWithAudit(ctx, memberID, status, now, audit)
}

func (s *MemberService) Delete(ctx context.Context, actor model.User, memberID string) error {
	if !s.policy.Can(actor.Role, authz.WriteMembers) {
		return apperror.ErrForbidden
	}
	now := s.now()
	audit, err := newAudit(actor, "member.deleted", "police_member", memberID, "soft-deleted member", now)
	if err != nil {
		return err
	}
	return s.repo.SoftDeleteMemberWithAudit(ctx, memberID, now, audit)
}
