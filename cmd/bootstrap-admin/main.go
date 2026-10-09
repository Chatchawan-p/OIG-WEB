// Package main provides a safe CLI tool to provision the initial administrator in local development.
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
		discordID   string
		displayName string
		roleStr     string
		confirm     bool
		envFile     string
	)

	flag.StringVar(&discordID, "discord-id", "", "Verified Discord User ID (snowflake, 17-20 digits) [REQUIRED]")
	flag.StringVar(&displayName, "name", "", "Display name of the user [REQUIRED]")
	flag.StringVar(&roleStr, "role", "Owner", "Initial role: Owner or Admin (default: Owner)")
	flag.BoolVar(&confirm, "confirm", false, "Explicit confirmation to provision the initial administrator [REQUIRED]")
	flag.StringVar(&envFile, "env-file", ".env.local,.env", "Comma-separated env files to load in order")
	flag.Parse()

	if discordID == "" || displayName == "" || !confirm {
		fmt.Fprintf(os.Stderr, "Error: missing required arguments.\n\n")
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  go run ./cmd/bootstrap-admin -discord-id 123456789012345678 -name \"MyName\" -confirm\n\n")
		os.Exit(1)
	}

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
	bootstrapService := service.NewBootstrapService(store, cfg.AppEnv, service.UTCNow)

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
