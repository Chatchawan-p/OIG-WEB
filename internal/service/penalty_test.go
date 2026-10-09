package service

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oig-police/oig-web/internal/apperror"
	"github.com/oig-police/oig-web/internal/authz"
	"github.com/oig-police/oig-web/internal/evidence"
	"github.com/oig-police/oig-web/internal/model"
	"github.com/oig-police/oig-web/internal/repository/memory"
)

var testPNG = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52}

func penaltyFixture(t *testing.T) (*PenaltyService, *memory.Store) {
	t.Helper()
	store := memory.New()
	store.SeedMembers(model.PoliceMember{MemberID: "mem_1", PoliceID: "P-001", Generation: 1, FullName: "Officer", EmploymentStatus: defaultEmploymentStatus})
	store.SeedPenaltyTypes(model.PenaltyType{TypeID: "pty_1", LegacyID: "PT001", Name: "Warning"})
	storage, err := evidence.NewLocal(t.TempDir(), 1024)
	if err != nil {
		t.Fatal(err)
	}
	return NewPenaltyService(store, storage, authz.New(), fixedTime), store
}

func TestPenaltyCreationAndPardonRecalculateCount(t *testing.T) {
	service, store := penaltyFixture(t)
	created, err := service.Create(context.Background(), ownerUser(), PenaltyCommand{PoliceID: "P-001", ActionTypeIDs: []string{"PT001"}, Reason: "Misconduct", DurationType: "permanent", EvidenceName: "proof.png", Evidence: bytes.NewReader(testPNG)})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.PenaltyCount != 1 || !created.Penalty.IsPermanent() || len(store.Audits) != 1 {
		t.Fatalf("unexpected creation: %#v", created)
	}
	pardoned, err := service.Pardon(context.Background(), ownerUser(), created.Penalty.PenaltyID, "Review complete")
	if err != nil {
		t.Fatalf("Pardon() error = %v", err)
	}
	if pardoned.PenaltyCount != 0 || pardoned.Penalty.PardonedAt == nil || len(store.Audits) != 2 {
		t.Fatalf("unexpected pardon: %#v", pardoned)
	}
	_, err = service.Pardon(context.Background(), ownerUser(), created.Penalty.PenaltyID, "Again")
	if !errors.Is(err, apperror.ErrConflict) {
		t.Fatalf("second Pardon() error = %v", err)
	}
}

func TestPenaltyValidation(t *testing.T) {
	service, _ := penaltyFixture(t)
	_, err := service.Create(context.Background(), ownerUser(), PenaltyCommand{PoliceID: "P-001", DurationType: "months"})
	var validation *apperror.ValidationError
	if !errors.As(err, &validation) || len(validation.Details) < 3 {
		t.Fatalf("Create() error = %#v", err)
	}
}

func TestPenaltyExpirationRules(t *testing.T) {
	start := fixedTime().Add(-48 * time.Hour)
	expired := fixedTime().Add(-time.Second)
	future := fixedTime().Add(time.Hour)
	tests := []struct {
		name    string
		penalty model.Penalty
		active  bool
	}{
		{name: "permanent", penalty: model.Penalty{DurationType: model.DurationPermanent, StartsAt: &start}, active: true},
		{name: "expired", penalty: model.Penalty{DurationType: model.DurationMonths, StartsAt: &start, ExpiresAt: &expired}, active: false},
		{name: "not started", penalty: model.Penalty{DurationType: model.DurationPermanent, StartsAt: &future}, active: false},
		{name: "pardoned", penalty: model.Penalty{DurationType: model.DurationPermanent, StartsAt: &start, PardonedAt: &expired}, active: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.penalty.IsActive(fixedTime()); got != tt.active {
				t.Fatalf("IsActive() = %v, want %v", got, tt.active)
			}
		})
	}
}

func TestExpiredPenaltyIsRemovedByRecalculation(t *testing.T) {
	service, store := penaltyFixture(t)
	start := fixedTime().Add(-48 * time.Hour)
	expired := fixedTime().Add(-time.Second)
	store.Penalties["pen_expired"] = model.Penalty{PenaltyID: "pen_expired", MemberID: "mem_1", PoliceID: "P-001", DurationType: model.DurationMonths, StartsAt: &start, ExpiresAt: &expired}
	member := store.Members["mem_1"]
	member.PenaltyCount = 99
	store.Members[member.MemberID] = member
	result, _, err := service.Summary(context.Background(), ownerUser(), "P-001")
	if err != nil {
		t.Fatal(err)
	}
	if result.PenaltyCount != 0 {
		t.Fatalf("PenaltyCount = %d, want 0", result.PenaltyCount)
	}
}
