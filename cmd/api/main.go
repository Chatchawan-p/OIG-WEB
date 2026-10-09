// Package main starts the OIG Police Management API.
//
// @title OIG Police Management API
// @version 1.0.0
// @description Production-oriented REST API for the OIG Police Management System.
// @BasePath /
// @schemes http https
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Enter "Bearer {token}".
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/oig-police/oig-web/docs"
	"github.com/oig-police/oig-web/internal/authz"
	"github.com/oig-police/oig-web/internal/config"
	"github.com/oig-police/oig-web/internal/database/mongodb"
	"github.com/oig-police/oig-web/internal/evidence"
	httprouter "github.com/oig-police/oig-web/internal/http/router"
	"github.com/oig-police/oig-web/internal/platform/logger"
	repositorymongo "github.com/oig-police/oig-web/internal/repository/mongo"
	"github.com/oig-police/oig-web/internal/service"
	"golang.org/x/oauth2"
)

func main() {
	bootstrap := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	slog.SetDefault(bootstrap)
	if err := run(); err != nil {
		slog.Error("application stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	_ = config.LoadEnvFiles(".env.local", ".env")
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	log := logger.New(cfg.AppEnv)
	slog.SetDefault(log)

	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelStartup()

	db, err := mongodb.Connect(startupCtx, cfg.MongoURI, cfg.MongoDatabase)
	if err != nil {
		return fmt.Errorf("mongodb startup: %w", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := db.Disconnect(ctx); err != nil {
			log.Error("mongodb disconnect failed", "error", err)
		}
	}()

	if err := db.EnsureIndexes(startupCtx); err != nil {
		return fmt.Errorf("mongodb index creation: %w", err)
	}

	evidenceStore, err := evidence.NewLocal(cfg.EvidenceStoragePath, cfg.EvidenceMaxBytes)
	if err != nil {
		return fmt.Errorf("evidence storage startup: %w", err)
	}
	store := repositorymongo.New(db)
	policy := authz.New()
	oauthConfig := oauth2.Config{
		ClientID: cfg.DiscordClientID, ClientSecret: cfg.DiscordClientSecret, RedirectURL: cfg.DiscordRedirectURI,
		Scopes:   []string{"identify"},
		Endpoint: oauth2.Endpoint{AuthURL: "https://discord.com/oauth2/authorize", TokenURL: "https://discord.com/api/oauth2/token"},
	}
	authService := service.NewAuthService(store, oauthConfig, "https://discord.com/api/users/@me", cfg.JWTSecret, cfg.JWTExpiration, service.UTCNow)
	memberService := service.NewMemberService(store, policy, service.UTCNow)
	penaltyService := service.NewPenaltyService(store, evidenceStore, policy, service.UTCNow)
	adminService := service.NewAdminService(store, policy, service.UTCNow)
	engine := httprouter.New(cfg, log, db, httprouter.Dependencies{Auth: authService, Members: memberService, Penalties: penaltyService, Admin: adminService, Policy: policy})
	server := &http.Server{
		Addr:              ":" + cfg.AppPort,
		Handler:           engine,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Info("http server starting", "address", server.Addr, "environment", cfg.AppEnv)
		serverErr <- server.ListenAndServe()
	}()

	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-shutdownSignal:
		log.Info("shutdown signal received", "signal", sig.String())
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http server: %w", err)
		}
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	log.Info("http server stopped")
	return nil
}
