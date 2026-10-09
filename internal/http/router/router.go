// Package router wires middleware and route handlers.
package router

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/oig-police/oig-web/internal/authz"
	"github.com/oig-police/oig-web/internal/config"
	"github.com/oig-police/oig-web/internal/http/handler"
	appmiddleware "github.com/oig-police/oig-web/internal/http/middleware"
	"github.com/oig-police/oig-web/internal/http/response"
	"github.com/oig-police/oig-web/internal/service"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

type Dependencies struct {
	Auth      *service.AuthService
	Members   *service.MemberService
	Penalties *service.PenaltyService
	Admin     *service.AdminService
	Policy    *authz.Policy
}

// New constructs the HTTP router and optionally registers the business API.
func New(cfg config.Config, log *slog.Logger, checker handler.HealthChecker, dependencies ...Dependencies) *gin.Engine {
	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	engine := gin.New()
	_ = engine.SetTrustedProxies(nil)
	engine.Use(
		appmiddleware.RequestID(),
		appmiddleware.SecurityHeaders(),
		appmiddleware.CORS(cfg.CORSAllowedOrigins),
		appmiddleware.Logging(log),
		appmiddleware.Recovery(log),
	)

	health := handler.NewHealthHandler(checker)
	engine.GET("/health", health.Health)
	engine.GET("/ready", health.Ready)

	if len(dependencies) > 0 && dependencies[0].Auth != nil {
		registerAPI(engine, cfg, dependencies[0])
	}

	if cfg.SwaggerEnabled {
		engine.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	}

	engine.NoRoute(func(c *gin.Context) {
		response.Failure(c, http.StatusNotFound, "route not found", "NOT_FOUND", nil)
	})
	return engine
}

func registerAPI(engine *gin.Engine, cfg config.Config, dependencies Dependencies) {
	authHandler := handler.NewAuthHandler(dependencies.Auth, cfg.CORSAllowedOrigins, cfg.AppEnv == "production" || cfg.AppEnv == "staging")
	memberHandler := handler.NewMemberHandler(dependencies.Members)
	penaltyHandler := handler.NewPenaltyHandler(dependencies.Penalties, cfg.EvidenceMaxBytes, service.UTCNow)
	adminHandler := handler.NewAdminHandler(dependencies.Admin)

	api := engine.Group("/api/v1")
	api.GET("/auth/discord/login", authHandler.DiscordLogin)
	api.GET("/auth/discord/callback", authHandler.DiscordCallback)

	protected := api.Group("")
	protected.Use(appmiddleware.Authentication(dependencies.Auth))
	protected.GET("/auth/me", authHandler.Me)
	protected.POST("/auth/logout", authHandler.Logout)

	protected.GET("/members", appmiddleware.Require(dependencies.Policy, authz.ReadMembers), memberHandler.List)
	protected.POST("/members", appmiddleware.Require(dependencies.Policy, authz.WriteMembers), memberHandler.Create)
	protected.POST("/members/bulk", appmiddleware.Require(dependencies.Policy, authz.WriteMembers), memberHandler.BulkCreate)
	protected.GET("/members/by-police-id/:policeId/summary", appmiddleware.Require(dependencies.Policy, authz.ReadMembers), penaltyHandler.MemberSummary)
	protected.GET("/members/:id", appmiddleware.Require(dependencies.Policy, authz.ReadMembers), memberHandler.Get)
	protected.PATCH("/members/:id", appmiddleware.Require(dependencies.Policy, authz.WriteMembers), memberHandler.Update)
	protected.PATCH("/members/:id/status", appmiddleware.Require(dependencies.Policy, authz.WriteMembers), memberHandler.UpdateStatus)
	protected.DELETE("/members/:id", appmiddleware.Require(dependencies.Policy, authz.WriteMembers), memberHandler.Delete)
	protected.GET("/members/:id/penalties", appmiddleware.Require(dependencies.Policy, authz.ReadPenalties), penaltyHandler.MemberPenalties)

	protected.GET("/penalty-types", appmiddleware.Require(dependencies.Policy, authz.ReadPenaltyTypes), penaltyHandler.ListTypes)
	protected.POST("/penalty-types", appmiddleware.Require(dependencies.Policy, authz.WritePenaltyTypes), penaltyHandler.CreateType)
	protected.DELETE("/penalty-types/:id", appmiddleware.Require(dependencies.Policy, authz.WritePenaltyTypes), penaltyHandler.DeleteType)
	protected.POST("/penalties", appmiddleware.Require(dependencies.Policy, authz.WritePenalties), penaltyHandler.Create)
	protected.GET("/penalties/:id", appmiddleware.Require(dependencies.Policy, authz.ReadPenalties), penaltyHandler.Get)
	protected.POST("/penalties/:id/pardon", appmiddleware.Require(dependencies.Policy, authz.WritePenalties), penaltyHandler.Pardon)
	protected.GET("/evidence/:id", appmiddleware.Require(dependencies.Policy, authz.ReadEvidence), penaltyHandler.Evidence)

	protected.GET("/penalty-rules", appmiddleware.Require(dependencies.Policy, authz.ReadCatalog), adminHandler.ListRules)
	protected.POST("/penalty-rules", appmiddleware.Require(dependencies.Policy, authz.WriteCatalog), adminHandler.CreateRule)
	protected.PATCH("/penalty-rules/:id", appmiddleware.Require(dependencies.Policy, authz.WriteCatalog), adminHandler.UpdateRule)
	protected.DELETE("/penalty-rules/:id", appmiddleware.Require(dependencies.Policy, authz.WriteCatalog), adminHandler.DeleteRule)
	protected.GET("/users", appmiddleware.Require(dependencies.Policy, authz.ReadUsers), adminHandler.ListUsers)
	protected.PATCH("/users/:id/role", appmiddleware.Require(dependencies.Policy, authz.WriteUserRoles), adminHandler.UpdateUserRole)
	protected.GET("/audit-logs", appmiddleware.Require(dependencies.Policy, authz.ReadAudit), adminHandler.ListAuditLogs)
	protected.GET("/metadata/departments", appmiddleware.Require(dependencies.Policy, authz.ReadMetadata), adminHandler.Departments)
}
