// Package main provides a safe CLI tool to provision and correct the initial administrator in local development.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/oig-police/oig-web/internal/config"
	"github.com/oig-police/oig-web/internal/database/mongodb"
	"github.com/oig-police/oig-web/internal/model"
	repositorymongo "github.com/oig-police/oig-web/internal/repository/mongo"
	"github.com/oig-police/oig-web/internal/service"
)

func main() {
	var (
		inspectFlag      bool
		correctDiscordID string
		targetUserID     string
		discordID        string
		displayName      string
		roleStr          string
		confirm          bool
		envFile          string
	)

	flag.BoolVar(&inspectFlag, "inspect", false, "Inspect current administrative users and audit history")
	flag.StringVar(&correctDiscordID, "correct-discord-id", "", "Update an existing Owner's Discord User ID (requires -confirm)")
	flag.StringVar(&targetUserID, "user-id", "", "Target User ID (e.g., usr_40e09880c4230d5b7cde65a065c50db3; optional if only 1 Owner exists)")
	flag.StringVar(&discordID, "discord-id", "", "Verified Discord User ID for initial bootstrap (snowflake, 17-20 digits)")
	flag.StringVar(&displayName, "name", "", "Display name of the user for initial bootstrap")
	flag.StringVar(&roleStr, "role", "Owner", "Initial role for bootstrap: Owner or Admin (default: Owner)")
	flag.BoolVar(&confirm, "confirm", false, "Explicit confirmation to provision or correct the administrator [REQUIRED for mutations]")
	flag.StringVar(&envFile, "env-file", ".env.local,.env", "Comma-separated env files to load in order")
	flag.Parse()

	envPaths := strings.Split(envFile, ",")
	_ = config.LoadEnvFiles(envPaths...)
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	// Strictly enforce non-production check before doing anything
	if cfg.AppEnv != "development" && cfg.AppEnv != "test" {
		fmt.Fprintf(os.Stderr, "Security error: bootstrap command is strictly forbidden in %q environment.\n", cfg.AppEnv)
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	logger.Info("connecting to mongodb...", "database", cfg.MongoDatabase)
	db, err := mongodb.Connect(ctx, cfg.MongoURI, cfg.MongoDatabase)
	if err != nil {
		fmt.Fprintf(os.Stderr, "MongoDB connection failed: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		discCtx, discCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer discCancel()
		_ = db.Disconnect(discCtx)
	}()

	if err := db.EnsureIndexes(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Ensuring indexes failed: %v\n", err)
		os.Exit(1)
	}

	store := repositorymongo.New(db)
	bootstrapService := service.NewBootstrapService(store, store, cfg.AppEnv, service.UTCNow)

	// Mode 1: Inspect
	if inspectFlag {
		users, audits, inspectErr := bootstrapService.InspectAdministrators(ctx)
		if inspectErr != nil {
			fmt.Fprintf(os.Stderr, "Inspection failed: %v\n", inspectErr)
			os.Exit(1)
		}
		fmt.Println()
		fmt.Println("==================================================")
		fmt.Println("CURRENT DATABASE USERS")
		fmt.Println("==================================================")
		if len(users) == 0 {
			fmt.Println("  (No users found in database)")
		} else {
			for i, u := range users {
				fmt.Printf("[%d] User ID:      %s\n", i+1, u.UserID)
				fmt.Printf("    Discord ID:   %s\n", u.DiscordID)
				fmt.Printf("    Display Name: %s\n", u.DisplayName)
				fmt.Printf("    Role:         %s\n", u.Role)
				fmt.Printf("    Token Ver:    %d\n", u.TokenVersion)
				fmt.Printf("    Created At:   %s\n", u.CreatedAt.Format(time.RFC3339))
				fmt.Printf("    Updated At:   %s\n", u.UpdatedAt.Format(time.RFC3339))
				fmt.Println()
			}
		}

		fmt.Println("==================================================")
		fmt.Println("RECENT AUDIT LOGS")
		fmt.Println("==================================================")
		if len(audits) == 0 {
			fmt.Println("  (No audit records found)")
		} else {
			for i, a := range audits {
				fmt.Printf("[%d] Audit ID:    %s\n", i+1, a.AuditID)
				fmt.Printf("    Actor ID:    %s\n", a.ActorID)
				fmt.Printf("    Action:      %s\n", a.Action)
				fmt.Printf("    Resource:    %s (%s)\n", a.ResourceType, a.ResourceID)
				fmt.Printf("    Description: %s\n", a.Description)
				fmt.Printf("    Created At:  %s\n", a.CreatedAt.Format(time.RFC3339))
				fmt.Println()
			}
		}
		fmt.Println("==================================================")
		return
	}

	// Mode 2: Correction of Owner Discord ID
	if correctDiscordID != "" {
		if !confirm {
			fmt.Fprintf(os.Stderr, "Error: owner identity correction requires explicit confirmation flag (-confirm).\n\n")
			fmt.Fprintf(os.Stderr, "Example:\n")
			fmt.Fprintf(os.Stderr, "  go run ./cmd/bootstrap-admin -correct-discord-id <NEW_DISCORD_ID> -confirm\n\n")
			os.Exit(1)
		}

		cmd := service.CorrectOwnerDiscordIDCommand{
			UserID:       targetUserID,
			NewDiscordID: correctDiscordID,
			Confirm:      confirm,
		}

		updatedUser, updateErr := bootstrapService.CorrectOwnerDiscordID(ctx, cmd)
		if updateErr != nil {
			fmt.Fprintf(os.Stderr, "\nCorrection failed: %v\n", updateErr)
			os.Exit(1)
		}

		fmt.Println()
		fmt.Println("==================================================")
		fmt.Println("Owner Discord Identity Corrected Successfully")
		fmt.Println("==================================================")
		fmt.Printf("User ID:      %s (PRESERVED)\n", updatedUser.UserID)
		fmt.Printf("Discord ID:   %s (UPDATED)\n", updatedUser.DiscordID)
		fmt.Printf("Display Name: %s (PRESERVED)\n", updatedUser.DisplayName)
		fmt.Printf("Role:         %s (PRESERVED)\n", updatedUser.Role)
		fmt.Printf("Token Ver:    %d (INCREMENTED)\n", updatedUser.TokenVersion)
		fmt.Printf("Updated At:   %s\n", updatedUser.UpdatedAt.Format(time.RFC3339))
		fmt.Println("==================================================")
		fmt.Println()
		fmt.Println("You can now authenticate via Discord OAuth2 at:")
		fmt.Printf("  http://localhost:%s/api/v1/auth/discord/login\n\n", cfg.AppPort)
		return
	}

	// Mode 3: Initial Bootstrap
	if discordID == "" || displayName == "" || !confirm {
		fmt.Fprintf(os.Stderr, "Error: missing required arguments.\n\n")
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  Inspect:  go run ./cmd/bootstrap-admin -inspect\n")
		fmt.Fprintf(os.Stderr, "  Correct:  go run ./cmd/bootstrap-admin -correct-discord-id <DISCORD_ID> -confirm\n")
		fmt.Fprintf(os.Stderr, "  Create:   go run ./cmd/bootstrap-admin -discord-id <DISCORD_ID> -name \"MyName\" -confirm\n\n")
		os.Exit(1)
	}

	role := model.Role(roleStr)
	cmd := service.BootstrapCommand{
		DiscordID:   discordID,
		DisplayName: displayName,
		Role:        role,
		Confirm:     confirm,
	}

	user, err := bootstrapService.BootstrapFirstAdmin(ctx, cmd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nBootstrap failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("==================================================")
	fmt.Println("Initial Administrator Provisioned Successfully")
	fmt.Println("==================================================")
	fmt.Printf("User ID:      %s\n", user.UserID)
	fmt.Printf("Discord ID:   %s\n", user.DiscordID)
	fmt.Printf("Display Name: %s\n", user.DisplayName)
	fmt.Printf("Role:         %s\n", user.Role)
	fmt.Printf("Created At:   %s\n", user.CreatedAt.Format(time.RFC3339))
	fmt.Println("==================================================")
	fmt.Println()
	fmt.Println("You can now authenticate via Discord OAuth2 at:")
	fmt.Printf("  http://localhost:%s/api/v1/auth/discord/login\n\n", cfg.AppPort)
}
