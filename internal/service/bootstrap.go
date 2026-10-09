package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/oig-police/oig-web/internal/apperror"
	"github.com/oig-police/oig-web/internal/model"
	"github.com/oig-police/oig-web/internal/platform/id"
	"github.com/oig-police/oig-web/internal/repository"
)

var discordSnowflakePattern = regexp.MustCompile(`^[0-9]{17,20}$`)

// BootstrapCommand captures the operator input required to provision the initial administrator.
type BootstrapCommand struct {
	DiscordID   string
	DisplayName string
	Role        model.Role
	Confirm     bool
}

// CorrectOwnerDiscordIDCommand captures the operator input required to correct an Owner's Discord identity.
type CorrectOwnerDiscordIDCommand struct {
	UserID       string
	NewDiscordID string
	Confirm      bool
}

// BootstrapService provides safe, environment-restricted provisioning and correction of the initial administrative user.
type BootstrapService struct {
	users  repository.UserRepository
	audits repository.AuditRepository
	appEnv string
	now    Clock
}

// NewBootstrapService creates a new BootstrapService.
func NewBootstrapService(users repository.UserRepository, audits repository.AuditRepository, appEnv string, now Clock) *BootstrapService {
	return &BootstrapService{
		users:  users,
		audits: audits,
		appEnv: strings.ToLower(strings.TrimSpace(appEnv)),
		now:    now,
	}
}

// BootstrapFirstAdmin provisions the initial Owner or Admin when no administrators exist.
// It fails closed if executed in production, if unconfirmed, if an administrator already exists,
// or if the Discord ID is duplicate or invalid.
func (s *BootstrapService) BootstrapFirstAdmin(ctx context.Context, cmd BootstrapCommand) (model.User, error) {
	// 1. Environment restriction: strictly development or test
	if s.appEnv != "development" && s.appEnv != "test" && s.appEnv != "local" {
		return model.User{}, fmt.Errorf("bootstrap is only permitted in development and test environments (current: %s): %w", s.appEnv, apperror.ErrForbidden)
	}

	// 2. Explicit confirmation check
	if !cmd.Confirm {
		return model.User{}, fmt.Errorf("bootstrap requires explicit administrator confirmation flag (-confirm): %w", apperror.ErrInvalid)
	}

	// 3. Validation: Discord ID
	discordID := strings.TrimSpace(cmd.DiscordID)
	if !discordSnowflakePattern.MatchString(discordID) {
		return model.User{}, &apperror.ValidationError{
			Details: []apperror.FieldError{
				{Field: "discord_id", Message: "must be a valid Discord snowflake ID (17-20 digits)"},
			},
		}
	}

	// 4. Validation: Display Name
	displayName := strings.TrimSpace(cmd.DisplayName)
	if displayName == "" || len([]rune(displayName)) > 100 {
		return model.User{}, &apperror.ValidationError{
			Details: []apperror.FieldError{
				{Field: "display_name", Message: "must be between 1 and 100 characters"},
			},
		}
	}

	// 5. Validation: Role (defaults to Owner, must be Owner or Admin)
	role := cmd.Role
	if role == "" {
		role = model.RoleOwner
	}
	if role != model.RoleOwner && role != model.RoleAdmin {
		return model.User{}, &apperror.ValidationError{
			Details: []apperror.FieldError{
				{Field: "role", Message: "must be Owner or Admin for initial bootstrap"},
			},
		}
	}

	// 6. Prevent execution when an administrator already exists
	adminCount, err := s.users.CountUsersByRoles(ctx, model.RoleOwner, model.RoleAdmin)
	if err != nil {
		return model.User{}, fmt.Errorf("check existing administrators: %w", err)
	}
	if adminCount > 0 {
		return model.User{}, fmt.Errorf("an administrator or owner already exists (%d found); initial bootstrap aborted: %w", adminCount, apperror.ErrConflict)
	}

	// 7. Prevent duplicate accounts: check if Discord ID is already registered
	if _, err := s.users.FindUserByDiscordID(ctx, discordID); err == nil {
		return model.User{}, fmt.Errorf("user with discord ID %s already exists: %w", discordID, apperror.ErrConflict)
	} else if !errors.Is(err, apperror.ErrNotFound) {
		return model.User{}, fmt.Errorf("check existing user: %w", err)
	}

	// 8. Generate stable resource IDs
	userID, err := id.New("usr")
	if err != nil {
		return model.User{}, fmt.Errorf("generate user id: %w", err)
	}
	auditID, err := id.New("aud")
	if err != nil {
		return model.User{}, fmt.Errorf("generate audit id: %w", err)
	}

	now := s.now().UTC()
	user := model.User{
		UserID:       userID,
		DiscordID:    discordID,
		DisplayName:  displayName,
		Role:         role,
		TokenVersion: 1,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	audit := model.AuditLog{
		AuditID:      auditID,
		ActorID:      userID,
		Action:       "user.bootstrap_admin",
		ResourceType: "user",
		ResourceID:   userID,
		Description:  fmt.Sprintf("Initial %s bootstrapped via CLI for Discord ID %s (%s)", role, discordID, displayName),
		CreatedAt:    now,
	}

	if err := s.users.CreateUserWithAudit(ctx, user, audit); err != nil {
		return model.User{}, fmt.Errorf("persist bootstrap user: %w", err)
	}

	return user, nil
}

// CorrectOwnerDiscordID safely updates the Discord ID of an existing Owner.
// It preserves the user_id, display_name, and role (strictly RoleOwner), prevents duplicate Discord IDs,
// records an immutable audit log, and is restricted to local/test environments.
func (s *BootstrapService) CorrectOwnerDiscordID(ctx context.Context, cmd CorrectOwnerDiscordIDCommand) (model.User, error) {
	// 1. Environment restriction: strictly development or test
	if s.appEnv != "development" && s.appEnv != "test" && s.appEnv != "local" {
		return model.User{}, fmt.Errorf("owner identity correction is only permitted in development and test environments (current: %s): %w", s.appEnv, apperror.ErrForbidden)
	}

	// 2. Explicit confirmation check
	if !cmd.Confirm {
		return model.User{}, fmt.Errorf("owner identity correction requires explicit confirmation flag (-confirm): %w", apperror.ErrInvalid)
	}

	// 3. Validation: New Discord ID
	newDiscordID := strings.TrimSpace(cmd.NewDiscordID)
	if !discordSnowflakePattern.MatchString(newDiscordID) {
		return model.User{}, &apperror.ValidationError{
			Details: []apperror.FieldError{
				{Field: "discord_id", Message: "must be a valid Discord snowflake ID (17-20 digits)"},
			},
		}
	}

	// 4. Find the target Owner user
	targetUserID := strings.TrimSpace(cmd.UserID)
	var target model.User
	var err error

	if targetUserID != "" {
		target, err = s.users.FindUserByID(ctx, targetUserID)
		if err != nil {
			return model.User{}, fmt.Errorf("find owner user %s: %w", targetUserID, err)
		}
	} else {
		// If user_id is omitted, verify there is exactly one Owner in the database
		users, _, listErr := s.users.ListUsers(ctx, repository.Page{Number: 1, Limit: 100})
		if listErr != nil {
			return model.User{}, fmt.Errorf("list users: %w", listErr)
		}
		var owners []model.User
		for _, u := range users {
			if u.Role == model.RoleOwner {
				owners = append(owners, u)
			}
		}
		if len(owners) == 0 {
			return model.User{}, fmt.Errorf("no owner found to correct: %w", apperror.ErrNotFound)
		}
		if len(owners) > 1 {
			return model.User{}, fmt.Errorf("multiple owners found; please specify -user-id explicitly: %w", apperror.ErrConflict)
		}
		target = owners[0]
	}

	// 5. Enforce role preservation and strict Owner requirement
	if target.Role != model.RoleOwner {
		return model.User{}, fmt.Errorf("user %s is not an Owner (role: %s); correction only applies to Owner: %w", target.UserID, target.Role, apperror.ErrForbidden)
	}

	// 6. No-op check
	if target.DiscordID == newDiscordID {
		return model.User{}, fmt.Errorf("discord ID is already %s: %w", newDiscordID, apperror.ErrConflict)
	}

	// 7. Duplicate prevention: ensure new Discord ID is not used by anyone else
	if existing, err := s.users.FindUserByDiscordID(ctx, newDiscordID); err == nil {
		return model.User{}, fmt.Errorf("discord ID %s is already in use by user %s: %w", newDiscordID, existing.UserID, apperror.ErrConflict)
	} else if !errors.Is(err, apperror.ErrNotFound) {
		return model.User{}, fmt.Errorf("check discord ID conflict: %w", err)
	}

	// 8. Generate audit record
	auditID, err := id.New("aud")
	if err != nil {
		return model.User{}, fmt.Errorf("generate audit id: %w", err)
	}
	now := s.now().UTC()

	audit := model.AuditLog{
		AuditID:      auditID,
		ActorID:      target.UserID,
		Action:       "user.discord_identity_corrected",
		ResourceType: "user",
		ResourceID:   target.UserID,
		Description:  fmt.Sprintf("Owner Discord ID corrected from %s to %s for user %s", target.DiscordID, newDiscordID, target.UserID),
		CreatedAt:    now,
	}

	// 9. Atomically update Discord ID and persist audit
	updatedUser, err := s.users.UpdateUserDiscordIDWithAudit(ctx, target.UserID, newDiscordID, now, audit)
	if err != nil {
		return model.User{}, fmt.Errorf("update owner discord ID: %w", err)
	}

	return updatedUser, nil
}

// InspectAdministrators returns all users and recent audit records for inspection.
func (s *BootstrapService) InspectAdministrators(ctx context.Context) ([]model.User, []model.AuditLog, error) {
	users, _, err := s.users.ListUsers(ctx, repository.Page{Number: 1, Limit: 100})
	if err != nil {
		return nil, nil, fmt.Errorf("list users: %w", err)
	}
	var audits []model.AuditLog
	if s.audits != nil {
		audits, _, err = s.audits.ListAuditLogs(ctx, repository.AuditFilter{Page: repository.Page{Number: 1, Limit: 100}})
		if err != nil {
			return nil, nil, fmt.Errorf("list audits: %w", err)
		}
	}
	return users, audits, nil
}
