//go:build integration

package mongo

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/oig-police/oig-web/internal/apperror"
	database "github.com/oig-police/oig-web/internal/database/mongodb"
	"github.com/oig-police/oig-web/internal/model"
)

func TestStoreMemberLifecycleIntegration(t *testing.T) {
	uri := os.Getenv("TEST_MONGO_URI")
	if uri == "" {
		t.Skip("TEST_MONGO_URI is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	databaseName := "oig_integration_" + time.Now().UTC().Format("20060102_150405_000000000")
	db, err := database.Connect(ctx, uri, databaseName)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Disconnect(context.Background())
	defer db.Collection("police_members").Database().Drop(context.Background())
	if err := db.EnsureIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	store := New(db)
	now := time.Now().UTC()
	member := model.PoliceMember{MemberID: "mem_integration", PoliceID: "P-INTEGRATION", Generation: 1, FullName: "Integration Officer", EmploymentStatus: "รับราชการ", CreatedAt: now, UpdatedAt: now}
	audit := model.AuditLog{AuditID: "aud_integration", ActorID: "usr_test", Action: "member.created", ResourceType: "police_member", ResourceID: member.MemberID, CreatedAt: now}
	if err := store.CreateMemberWithAudit(ctx, member, audit); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.FindMemberByPoliceID(ctx, member.PoliceID)
	if err != nil || loaded.MemberID != member.MemberID {
		t.Fatalf("FindMemberByPoliceID() = %#v, %v", loaded, err)
	}
	member.MemberID = "mem_duplicate"
	audit.AuditID = "aud_duplicate"
	err = store.CreateMemberWithAudit(ctx, member, audit)
	if !errors.Is(err, apperror.ErrConflict) {
		t.Fatalf("duplicate error = %v", err)
	}
}
