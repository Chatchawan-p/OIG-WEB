package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/oig-police/oig-web/internal/dto"
	appmiddleware "github.com/oig-police/oig-web/internal/http/middleware"
	"github.com/oig-police/oig-web/internal/http/response"
	"github.com/oig-police/oig-web/internal/model"
	"github.com/oig-police/oig-web/internal/repository"
	"github.com/oig-police/oig-web/internal/service"
)

type AdminHandler struct{ service *service.AdminService }

func NewAdminHandler(service *service.AdminService) *AdminHandler {
	return &AdminHandler{service: service}
}

// ListPenaltyRules godoc
// @Summary List penalty catalog rules
// @Tags Penalty Catalog
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope{data=[]dto.PenaltyRule}
// @Router /api/v1/penalty-rules [get]
func (h *AdminHandler) ListRules(c *gin.Context) {
	items, err := h.service.ListRules(c.Request.Context(), appmiddleware.MustActor(c))
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "get penalty rules successfully", mapRules(items))
}

// CreatePenaltyRule godoc
// @Summary Create a penalty catalog rule (Owner/Admin)
// @Tags Penalty Catalog
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body dto.PenaltyRuleRequest true "Rule"
// @Success 201 {object} response.Envelope{data=dto.PenaltyRule}
// @Failure 403 {object} response.Envelope
// @Router /api/v1/penalty-rules [post]
func (h *AdminHandler) CreateRule(c *gin.Context) {
	var request dto.PenaltyRuleRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.FromError(c, validationError("body", "name and category are required"))
		return
	}
	rule, err := h.service.CreateRule(c.Request.Context(), appmiddleware.MustActor(c), ruleCommand(request))
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.Success(c, http.StatusCreated, "create penalty rule successfully", dto.RuleFromModel(rule))
}

// UpdatePenaltyRule godoc
// @Summary Update a penalty catalog rule (Owner/Admin)
// @Tags Penalty Catalog
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Rule ID"
// @Param request body dto.PenaltyRuleRequest true "Rule"
// @Success 200 {object} response.Envelope{data=dto.PenaltyRule}
// @Router /api/v1/penalty-rules/{id} [patch]
func (h *AdminHandler) UpdateRule(c *gin.Context) {
	var request dto.PenaltyRuleRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.FromError(c, validationError("body", "name and category are required"))
		return
	}
	rule, err := h.service.UpdateRule(c.Request.Context(), appmiddleware.MustActor(c), c.Param("id"), ruleCommand(request))
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "update penalty rule successfully", dto.RuleFromModel(rule))
}

// DeletePenaltyRule godoc
// @Summary Delete a penalty catalog rule (Owner/Admin)
// @Tags Penalty Catalog
// @Security BearerAuth
// @Param id path string true "Rule ID"
// @Success 200 {object} response.Envelope
// @Router /api/v1/penalty-rules/{id} [delete]
func (h *AdminHandler) DeleteRule(c *gin.Context) {
	if err := h.service.DeleteRule(c.Request.Context(), appmiddleware.MustActor(c), c.Param("id")); err != nil {
		response.FromError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "delete penalty rule successfully", nil)
}

// ListUsers godoc
// @Summary List internal users
// @Tags Users
// @Security BearerAuth
// @Produce json
// @Param page query int false "Page" default(1)
// @Param limit query int false "Limit" default(20) maximum(100)
// @Success 200 {object} response.Envelope{data=dto.UserList}
// @Router /api/v1/users [get]
func (h *AdminHandler) ListUsers(c *gin.Context) {
	page, err := pageFromQuery(c, 20, 100)
	if err != nil {
		response.FromError(c, err)
		return
	}
	items, total, err := h.service.ListUsers(c.Request.Context(), appmiddleware.MustActor(c), page)
	if err != nil {
		response.FromError(c, err)
		return
	}
	mapped := make([]dto.User, 0, len(items))
	for _, item := range items {
		mapped = append(mapped, dto.UserFromModel(item))
	}
	response.Success(c, http.StatusOK, "get users successfully", dto.UserList{Items: mapped, Pagination: dto.Pagination{Page: page.Number, Limit: page.Limit, Total: total}})
}

// UpdateUserRole godoc
// @Summary Change a user's role using the server-side hierarchy policy
// @Tags Users
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "User ID"
// @Param request body dto.UpdateRoleRequest true "New role"
// @Success 200 {object} response.Envelope{data=dto.User}
// @Failure 403 {object} response.Envelope
// @Router /api/v1/users/{id}/role [patch]
func (h *AdminHandler) UpdateUserRole(c *gin.Context) {
	var request dto.UpdateRoleRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.FromError(c, validationError("body", "role is required"))
		return
	}
	user, err := h.service.UpdateUserRole(c.Request.Context(), appmiddleware.MustActor(c), c.Param("id"), request.Role)
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "update user role successfully", dto.UserFromModel(user))
}

// ListAuditLogs godoc
// @Summary List immutable audit logs newest first
// @Tags Audit
// @Security BearerAuth
// @Produce json
// @Param page query int false "Page" default(1)
// @Param limit query int false "Limit" default(50) maximum(200)
// @Success 200 {object} response.Envelope{data=dto.AuditLogList}
// @Router /api/v1/audit-logs [get]
func (h *AdminHandler) ListAuditLogs(c *gin.Context) {
	page, err := pageFromQuery(c, 50, 200)
	if err != nil {
		response.FromError(c, err)
		return
	}
	items, total, err := h.service.ListAuditLogs(c.Request.Context(), appmiddleware.MustActor(c), repository.AuditFilter{Page: page})
	if err != nil {
		response.FromError(c, err)
		return
	}
	mapped := make([]dto.AuditLog, 0, len(items))
	for _, item := range items {
		mapped = append(mapped, dto.AuditFromModel(item))
	}
	response.Success(c, http.StatusOK, "get audit logs successfully", dto.AuditLogList{Items: mapped, Pagination: dto.Pagination{Page: page.Number, Limit: page.Limit, Total: total}})
}

// Departments godoc
// @Summary List distinct stored department options
// @Tags Metadata
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope{data=dto.Departments}
// @Router /api/v1/metadata/departments [get]
func (h *AdminHandler) Departments(c *gin.Context) {
	items, err := h.service.Departments(c.Request.Context(), appmiddleware.MustActor(c))
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "get departments successfully", dto.Departments{Items: items})
}

func ruleCommand(request dto.PenaltyRuleRequest) service.RuleCommand {
	return service.RuleCommand{Name: request.Name, Category: request.Category, Fine: request.Fine, Jail: request.Jail, Description: request.Description}
}

func mapRules(items []model.PenaltyRule) []dto.PenaltyRule {
	result := make([]dto.PenaltyRule, 0, len(items))
	for _, item := range items {
		result = append(result, dto.RuleFromModel(item))
	}
	return result
}
