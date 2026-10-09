package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/oig-police/oig-web/internal/dto"
	appmiddleware "github.com/oig-police/oig-web/internal/http/middleware"
	"github.com/oig-police/oig-web/internal/http/response"
	"github.com/oig-police/oig-web/internal/model"
	"github.com/oig-police/oig-web/internal/repository"
	"github.com/oig-police/oig-web/internal/service"
)

type MemberHandler struct{ service *service.MemberService }

func NewMemberHandler(service *service.MemberService) *MemberHandler {
	return &MemberHandler{service: service}
}

// ListMembers godoc
// @Summary List and filter police members
// @Description Returns deterministic paginated items and the same page grouped by generation for frontend rendering.
// @Tags Members
// @Security BearerAuth
// @Produce json
// @Param query query string false "Search Police ID, name, rank, department, or unit"
// @Param empFilter query string false "Employment status"
// @Param discFilter query string false "Active penalty count: 0, 1, 2, or 3 for 3+"
// @Param deptFilter query string false "Department"
// @Param genRangeFilter query string false "Generation or inclusive min-max"
// @Param page query int false "Page" default(1)
// @Param limit query int false "Limit" default(100) maximum(500)
// @Success 200 {object} response.Envelope{data=dto.MemberList}
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Router /api/v1/members [get]
func (h *MemberHandler) List(c *gin.Context) {
	page, err := pageFromQuery(c, 100, 500)
	if err != nil {
		response.FromError(c, err)
		return
	}
	minimum, maximum, err := parseGenerationRange(c.Query("genRangeFilter"))
	if err != nil {
		response.FromError(c, err)
		return
	}
	discipline := c.Query("discFilter")
	if discipline != "" && discipline != "0" && discipline != "1" && discipline != "2" && discipline != "3" {
		response.FromError(c, validationError("discFilter", "must be 0, 1, 2, or 3"))
		return
	}
	filter := repository.MemberFilter{Query: c.Query("query"), EmploymentStatus: c.Query("empFilter"), Discipline: discipline, Department: c.Query("deptFilter"), GenerationMin: minimum, GenerationMax: maximum, Page: page}
	items, total, err := h.service.List(c.Request.Context(), appmiddleware.MustActor(c), filter)
	if err != nil {
		response.FromError(c, err)
		return
	}
	mapped := mapMembers(items)
	groups := make(map[string][]dto.Member)
	for _, member := range mapped {
		key := strconv.Itoa(member.Generation)
		groups[key] = append(groups[key], member)
	}
	response.Success(c, http.StatusOK, "get members successfully", dto.MemberList{Items: mapped, GroupedByGeneration: groups, Pagination: dto.Pagination{Page: page.Number, Limit: page.Limit, Total: total}})
}

// GetMember godoc
// @Summary Get member profile
// @Tags Members
// @Security BearerAuth
// @Produce json
// @Param id path string true "Stable member ID"
// @Success 200 {object} response.Envelope{data=dto.MemberDetail}
// @Failure 404 {object} response.Envelope
// @Router /api/v1/members/{id} [get]
func (h *MemberHandler) Get(c *gin.Context) {
	member, history, err := h.service.Get(c.Request.Context(), appmiddleware.MustActor(c), c.Param("id"))
	if err != nil {
		response.FromError(c, err)
		return
	}
	mappedHistory := make([]dto.NameHistory, 0, len(history))
	for _, entry := range history {
		mappedHistory = append(mappedHistory, dto.NameHistoryFromModel(entry))
	}
	response.Success(c, http.StatusOK, "get member successfully", dto.MemberDetail{Member: dto.MemberFromModel(member), NameHistory: mappedHistory})
}

// CreateMember godoc
// @Summary Create a police member
// @Tags Members
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body dto.CreateMemberRequest true "Member"
// @Success 201 {object} response.Envelope{data=dto.Member}
// @Failure 400 {object} response.Envelope
// @Failure 409 {object} response.Envelope
// @Router /api/v1/members [post]
func (h *MemberHandler) Create(c *gin.Context) {
	var request dto.CreateMemberRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.FromError(c, validationError("body", "must be a valid member object"))
		return
	}
	member, err := h.service.Create(c.Request.Context(), appmiddleware.MustActor(c), memberCommand(request))
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.Success(c, http.StatusCreated, "create member successfully", dto.MemberFromModel(member))
}

// BulkCreateMembers godoc
// @Summary Atomically create up to 100 members
// @Tags Members
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body dto.BulkCreateMembersRequest true "Members"
// @Success 201 {object} response.Envelope{data=[]dto.Member}
// @Failure 400 {object} response.Envelope
// @Failure 409 {object} response.Envelope
// @Router /api/v1/members/bulk [post]
func (h *MemberHandler) BulkCreate(c *gin.Context) {
	var request dto.BulkCreateMembersRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.FromError(c, validationError("body", "must contain a members array with 1 to 100 items"))
		return
	}
	commands := make([]service.MemberCommand, 0, len(request.Members))
	for _, member := range request.Members {
		commands = append(commands, memberCommand(member))
	}
	members, err := h.service.BulkCreate(c.Request.Context(), appmiddleware.MustActor(c), commands)
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.Success(c, http.StatusCreated, "bulk create members successfully", mapMembers(members))
}

// UpdateMember godoc
// @Summary Update a member and automatically record name history
// @Tags Members
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Stable member ID"
// @Param request body dto.UpdateMemberRequest true "Fields to update"
// @Success 200 {object} response.Envelope{data=dto.Member}
// @Failure 400 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Router /api/v1/members/{id} [patch]
func (h *MemberHandler) Update(c *gin.Context) {
	var request dto.UpdateMemberRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.FromError(c, validationError("body", "must be a valid member patch"))
		return
	}
	if request.Generation == nil && request.Rank == nil && request.MedicalPrefix == nil && request.FullName == nil && request.Department == nil && request.Unit == nil {
		response.FromError(c, validationError("body", "must include at least one editable field"))
		return
	}
	member, err := h.service.Update(c.Request.Context(), appmiddleware.MustActor(c), c.Param("id"), service.MemberPatch{Generation: request.Generation, Rank: request.Rank, MedicalPrefix: request.MedicalPrefix, FullName: request.FullName, Department: request.Department, Unit: request.Unit})
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "update member successfully", dto.MemberFromModel(member))
}

// UpdateMemberStatus godoc
// @Summary Update employment status
// @Tags Members
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Stable member ID"
// @Param request body dto.UpdateMemberStatusRequest true "Status"
// @Success 200 {object} response.Envelope{data=dto.Member}
// @Router /api/v1/members/{id}/status [patch]
func (h *MemberHandler) UpdateStatus(c *gin.Context) {
	var request dto.UpdateMemberStatusRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.FromError(c, validationError("body", "employmentStatus is required"))
		return
	}
	member, err := h.service.UpdateStatus(c.Request.Context(), appmiddleware.MustActor(c), c.Param("id"), request.EmploymentStatus)
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "update member status successfully", dto.MemberFromModel(member))
}

// DeleteMember godoc
// @Summary Soft-delete a member
// @Tags Members
// @Security BearerAuth
// @Param id path string true "Stable member ID"
// @Success 200 {object} response.Envelope
// @Failure 404 {object} response.Envelope
// @Router /api/v1/members/{id} [delete]
func (h *MemberHandler) Delete(c *gin.Context) {
	if err := h.service.Delete(c.Request.Context(), appmiddleware.MustActor(c), c.Param("id")); err != nil {
		response.FromError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "delete member successfully", nil)
}

func memberCommand(request dto.CreateMemberRequest) service.MemberCommand {
	return service.MemberCommand{PoliceID: request.PoliceID, Generation: request.Generation, Rank: request.Rank, MedicalPrefix: request.MedicalPrefix, FullName: request.FullName, EmploymentStatus: request.EmploymentStatus, Department: request.Department, Unit: request.Unit}
}

func mapMembers(items []model.PoliceMember) []dto.Member {
	result := make([]dto.Member, 0, len(items))
	for _, item := range items {
		result = append(result, dto.MemberFromModel(item))
	}
	return result
}
