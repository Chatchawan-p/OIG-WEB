package service

import (
	"context"
	"errors"
	"testing"

	"github.com/oig-police/oig-web/internal/apperror"
	"github.com/oig-police/oig-web/internal/authz"
	"github.com/oig-police/oig-web/internal/model"
	"github.com/oig-police/oig-web/internal/repository/memory"
)

func TestUserRoleChangeUsesHierarchyAndCreatesAudit(t *testing.T) {
	store := memory.New()
	owner := ownerUser()
	target := model.User{UserID: "usr_target", Role: model.RoleAdmin}
	store.SeedUsers(owner, target)
	service := NewAdminService(store, authz.New(), fixedTime)
	updated, err := service.UpdateUserRole(context.Background(), owner, target.UserID, string(model.RoleScreener))
	if err != nil {
		t.Fatalf("UpdateUserRole() error = %v", err)
	}
	if updated.Role != model.RoleScreener || updated.TokenVersion != 1 || len(store.Audits) != 1 {
		t.Fatalf("unexpected update: %#v audits=%#v", updated, store.Audits)
	}
}

func TestUserRoleChangePreventsEscalationAndSelfChange(t *testing.T) {
	store := memory.New()
	admin := model.User{UserID: "usr_admin", Role: model.RoleAdmin}
	store.SeedUsers(admin)
	service := NewAdminService(store, authz.New(), fixedTime)
	_, err := service.UpdateUserRole(context.Background(), admin, admin.UserID, string(model.RoleOwner))
	if !errors.Is(err, apperror.ErrForbidden) {
		t.Fatalf("UpdateUserRole() error = %v", err)
	}
}

func TestCatalogWriteRestrictionAtServiceBoundary(t *testing.T) {
	store := memory.New()
	service := NewAdminService(store, authz.New(), fixedTime)
	_, err := service.CreateRule(context.Background(), model.User{UserID: "s", Role: model.RoleScreener}, RuleCommand{Name: "Rule", Category: "Conduct"})
	if !errors.Is(err, apperror.ErrForbidden) {
		t.Fatalf("CreateRule() error = %v", err)
	}
}
