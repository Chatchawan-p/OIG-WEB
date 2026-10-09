package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oig-police/oig-web/internal/apperror"
	"github.com/oig-police/oig-web/internal/authz"
	"github.com/oig-police/oig-web/internal/model"
	"github.com/oig-police/oig-web/internal/repository"
	"github.com/oig-police/oig-web/internal/repository/memory"
)

func fixedTime() time.Time { return time.Date(2026, 10, 9, 9, 0, 0, 0, time.UTC) }

func ownerUser() model.User { return model.User{UserID: "usr_owner", Role: model.RoleOwner} }

func TestMemberCreationDuplicateAndAudit(t *testing.T) {
	store := memory.New()
	service := NewMemberService(store, authz.New(), fixedTime)
	command := MemberCommand{PoliceID: "P-001", Generation: 10, FullName: "Officer One", Department: "OIG"}
	member, err := service.Create(context.Background(), ownerUser(), command)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if member.EmploymentStatus != defaultEmploymentStatus || len(store.Audits) != 1 {
		t.Fatalf("unexpected member/audit: %#v audits=%d", member, len(store.Audits))
	}
	_, err = service.Create(context.Background(), ownerUser(), command)
	if !errors.Is(err, apperror.ErrConflict) {
		t.Fatalf("duplicate Create() error = %v", err)
	}
}

func TestBulkCreateIsAtomicForRequestDuplicates(t *testing.T) {
	store := memory.New()
	service := NewMemberService(store, authz.New(), fixedTime)
	_, err := service.BulkCreate(context.Background(), ownerUser(), []MemberCommand{
		{PoliceID: "P-001", Generation: 1, FullName: "One"},
		{PoliceID: "P-001", Generation: 2, FullName: "Two"},
	})
	var validation *apperror.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("BulkCreate() error = %v", err)
	}
	if len(store.Members) != 0 || len(store.Audits) != 0 {
		t.Fatal("invalid bulk request wrote partial state")
	}
}

func TestMemberUpdateCreatesServerDerivedNameHistory(t *testing.T) {
	store := memory.New()
	store.SeedMembers(model.PoliceMember{MemberID: "mem_1", PoliceID: "P-001", Generation: 1, FullName: "Old Name", EmploymentStatus: defaultEmploymentStatus})
	service := NewMemberService(store, authz.New(), fixedTime)
	newName := "New Name"
	updated, err := service.Update(context.Background(), ownerUser(), "mem_1", MemberPatch{FullName: &newName})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.FullName != newName || len(store.History) != 1 || store.History[0].OldName != "Old Name" || store.History[0].ChangedBy != ownerUser().UserID {
		t.Fatalf("unexpected history: %#v", store.History)
	}
}

func TestMemberStatusAndFilters(t *testing.T) {
	store := memory.New()
	store.SeedMembers(
		model.PoliceMember{MemberID: "m1", PoliceID: "P1", Generation: 1, FullName: "Alpha", Department: "A", EmploymentStatus: defaultEmploymentStatus},
		model.PoliceMember{MemberID: "m2", PoliceID: "P2", Generation: 2, FullName: "Beta", Department: "B", EmploymentStatus: defaultEmploymentStatus},
	)
	service := NewMemberService(store, authz.New(), fixedTime)
	updated, err := service.UpdateStatus(context.Background(), ownerUser(), "m2", "ปลดราชการ")
	if err != nil || updated.EmploymentStatus != "ปลดราชการ" {
		t.Fatalf("UpdateStatus() = %#v, %v", updated, err)
	}
	min, max := 1, 1
	items, total, err := service.List(context.Background(), ownerUser(), repository.MemberFilter{Query: "alp", Department: "A", GenerationMin: &min, GenerationMax: &max, Page: repository.Page{Number: 1, Limit: 20}})
	if err != nil || total != 1 || len(items) != 1 || items[0].MemberID != "m1" {
		t.Fatalf("List() = %#v, total=%d, err=%v", items, total, err)
	}
}
