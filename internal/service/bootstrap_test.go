package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/oig-police/oig-web/internal/apperror"
	"github.com/oig-police/oig-web/internal/model"
	"github.com/oig-police/oig-web/internal/repository/memory"
)

func TestBootstrapFirstAdmin_Success(t *testing.T) {
	store := memory.New()
	svc := NewBootstrapService(store, "development", fixedTime)

	cmd := BootstrapCommand{
		DiscordID:   "123456789012345678",
		DisplayName: "Initial Owner",
		Role:        model.RoleOwner,
		Confirm:     true,
	}

	user, err := svc.BootstrapFirstAdmin(context.Background(), cmd)
	if err != nil {
		t.Fatalf("BootstrapFirstAdmin() unexpected error = %v", err)
	}

	if !strings.HasPrefix(user.UserID, "usr_") {
		t.Errorf("expected UserID to have prefix 'usr_', got %q", user.UserID)
	}
	if user.DiscordID != cmd.DiscordID {
		t.Errorf("expected DiscordID %q, got %q", cmd.DiscordID, user.DiscordID)
	}
	if user.DisplayName != cmd.DisplayName {
		t.Errorf("expected DisplayName %q, got %q", cmd.DisplayName, user.DisplayName)
	}
	if user.Role != model.RoleOwner {
		t.Errorf("expected Role %q, got %q", model.RoleOwner, user.Role)
	}
	if user.TokenVersion != 1 {
		t.Errorf("expected TokenVersion 1, got %d", user.TokenVersion)
	}
	if user.CreatedAt != fixedTime() || user.UpdatedAt != fixedTime() {
		t.Errorf("expected timestamps %v, got created=%v updated=%v", fixedTime(), user.CreatedAt, user.UpdatedAt)
	}

	// Verify persistence in store
	saved, err := store.FindUserByID(context.Background(), user.UserID)
	if err != nil {
		t.Fatalf("user not found in store: %v", err)
	}
	if saved.DiscordID != cmd.DiscordID {
		t.Errorf("saved DiscordID mismatch: %q", saved.DiscordID)
	}

	// Verify audit log
	if len(store.Audits) != 1 {
		t.Fatalf("expected 1 audit log, got %d", len(store.Audits))
	}
	audit := store.Audits[0]
	if audit.ActorID != user.UserID {
		t.Errorf("expected audit ActorID %q, got %q", user.UserID, audit.ActorID)
	}
	if audit.Action != "user.bootstrap_admin" {
		t.Errorf("expected audit Action 'user.bootstrap_admin', got %q", audit.Action)
	}
	if audit.ResourceID != user.UserID {
		t.Errorf("expected audit ResourceID %q, got %q", user.UserID, audit.ResourceID)
	}
	if !strings.Contains(audit.Description, cmd.DiscordID) {
		t.Errorf("expected audit Description to contain DiscordID, got %q", audit.Description)
	}
}

func TestBootstrapFirstAdmin_DefaultRoleIsOwner(t *testing.T) {
	store := memory.New()
	svc := NewBootstrapService(store, "development", fixedTime)

	cmd := BootstrapCommand{
		DiscordID:   "123456789012345678",
		DisplayName: "Root Admin",
		Role:        "", // empty should default to Owner
		Confirm:     true,
	}

	user, err := svc.BootstrapFirstAdmin(context.Background(), cmd)
	if err != nil {
		t.Fatalf("BootstrapFirstAdmin() unexpected error = %v", err)
	}
	if user.Role != model.RoleOwner {
		t.Errorf("expected default Role %q, got %q", model.RoleOwner, user.Role)
	}
}

func TestBootstrapFirstAdmin_EnvironmentRestrictions(t *testing.T) {
	disallowedEnvs := []string{"production", "staging", "prod", "uat", ""}

	for _, env := range disallowedEnvs {
		t.Run("env_"+env, func(t *testing.T) {
			store := memory.New()
			svc := NewBootstrapService(store, env, fixedTime)

			cmd := BootstrapCommand{
				DiscordID:   "123456789012345678",
				DisplayName: "Admin",
				Role:        model.RoleOwner,
				Confirm:     true,
			}

			_, err := svc.BootstrapFirstAdmin(context.Background(), cmd)
			if err == nil {
				t.Fatalf("expected error for environment %q, got nil", env)
			}
			if !errors.Is(err, apperror.ErrForbidden) {
				t.Errorf("expected ErrForbidden for environment %q, got %v", env, err)
			}
			if len(store.Users) != 0 {
				t.Errorf("expected 0 users in store, got %d", len(store.Users))
			}
			if len(store.Audits) != 0 {
				t.Errorf("expected 0 audits in store, got %d", len(store.Audits))
			}
		})
	}
}

func TestBootstrapFirstAdmin_ExplicitConfirmationRequired(t *testing.T) {
	store := memory.New()
	svc := NewBootstrapService(store, "development", fixedTime)

	cmd := BootstrapCommand{
		DiscordID:   "123456789012345678",
		DisplayName: "Admin",
		Role:        model.RoleOwner,
		Confirm:     false, // Unconfirmed
	}

	_, err := svc.BootstrapFirstAdmin(context.Background(), cmd)
	if err == nil {
		t.Fatal("expected error when confirm is false, got nil")
	}
	if !errors.Is(err, apperror.ErrInvalid) {
		t.Errorf("expected ErrInvalid, got %v", err)
	}
	if len(store.Users) != 0 {
		t.Errorf("expected 0 users created, got %d", len(store.Users))
	}
}

func TestBootstrapFirstAdmin_Validation(t *testing.T) {
	testCases := []struct {
		name        string
		discordID   string
		displayName string
		role        model.Role
		field       string
	}{
		{"empty discord id", "", "Admin", model.RoleOwner, "discord_id"},
		{"alphanumeric discord id", "abc123456789012345", "Admin", model.RoleOwner, "discord_id"},
		{"too short discord id", "1234567890123456", "Admin", model.RoleOwner, "discord_id"},     // 16 digits
		{"too long discord id", "123456789012345678901", "Admin", model.RoleOwner, "discord_id"}, // 21 digits
		{"empty display name", "123456789012345678", "", model.RoleOwner, "display_name"},
		{"whitespace display name", "123456789012345678", "   ", model.RoleOwner, "display_name"},
		{"disallowed role Screener", "123456789012345678", "Admin", model.RoleScreener, "role"},
		{"disallowed role Guest", "123456789012345678", "Admin", model.RoleGuest, "role"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			store := memory.New()
			svc := NewBootstrapService(store, "development", fixedTime)

			cmd := BootstrapCommand{
				DiscordID:   tc.discordID,
				DisplayName: tc.displayName,
				Role:        tc.role,
				Confirm:     true,
			}

			_, err := svc.BootstrapFirstAdmin(context.Background(), cmd)
			if err == nil {
				t.Fatalf("expected validation error for %s, got nil", tc.name)
			}
			var valErr *apperror.ValidationError
			if !errors.As(err, &valErr) {
				t.Fatalf("expected ValidationError, got %v", err)
			}
			found := false
			for _, detail := range valErr.Details {
				if detail.Field == tc.field {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected validation detail for field %q, got %#v", tc.field, valErr.Details)
			}
		})
	}
}

func TestBootstrapFirstAdmin_PreventsExecutionWhenAdminAlreadyExists(t *testing.T) {
	testCases := []struct {
		name         string
		existingRole model.Role
	}{
		{"existing owner", model.RoleOwner},
		{"existing admin", model.RoleAdmin},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			store := memory.New()
			store.SeedUsers(model.User{
				UserID:      "usr_existing",
				DiscordID:   "999999999999999999",
				DisplayName: "Existing Privileged",
				Role:        tc.existingRole,
			})

			svc := NewBootstrapService(store, "development", fixedTime)
			cmd := BootstrapCommand{
				DiscordID:   "123456789012345678",
				DisplayName: "New Admin",
				Role:        model.RoleOwner,
				Confirm:     true,
			}

			_, err := svc.BootstrapFirstAdmin(context.Background(), cmd)
			if err == nil {
				t.Fatalf("expected error when %s exists, got nil", tc.name)
			}
			if !errors.Is(err, apperror.ErrConflict) {
				t.Errorf("expected ErrConflict, got %v", err)
			}
			if !strings.Contains(err.Error(), "already exists") {
				t.Errorf("expected error message to mention 'already exists', got %q", err.Error())
			}
		})
	}
}

func TestBootstrapFirstAdmin_PreventsDuplicateDiscordID(t *testing.T) {
	store := memory.New()
	// Pre-seed a non-privileged user (e.g. Guest) with the target Discord ID
	store.SeedUsers(model.User{
		UserID:      "usr_guest",
		DiscordID:   "123456789012345678",
		DisplayName: "Guest User",
		Role:        model.RoleGuest,
	})

	svc := NewBootstrapService(store, "development", fixedTime)
	cmd := BootstrapCommand{
		DiscordID:   "123456789012345678",
		DisplayName: "New Owner",
		Role:        model.RoleOwner,
		Confirm:     true,
	}

	_, err := svc.BootstrapFirstAdmin(context.Background(), cmd)
	if err == nil {
		t.Fatal("expected error for duplicate Discord ID, got nil")
	}
	if !errors.Is(err, apperror.ErrConflict) {
		t.Errorf("expected ErrConflict, got %v", err)
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected error to mention 'already exists', got %q", err.Error())
	}
}
