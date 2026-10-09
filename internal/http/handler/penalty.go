package handler

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oig-police/oig-web/internal/dto"
	appmiddleware "github.com/oig-police/oig-web/internal/http/middleware"
	"github.com/oig-police/oig-web/internal/http/response"
	"github.com/oig-police/oig-web/internal/model"
	"github.com/oig-police/oig-web/internal/service"
)

type PenaltyHandler struct {
	service      *service.PenaltyService
	maxBodyBytes int64
	clock        service.Clock
}

func NewPenaltyHandler(service *service.PenaltyService, maxEvidenceBytes int64, clock service.Clock) *PenaltyHandler {
	return &PenaltyHandler{service: service, maxBodyBytes: maxEvidenceBytes + (1 << 20), clock: clock}
}

// ListPenaltyTypes godoc
// @Summary List penalty types
// @Tags Penalty Types
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope{data=[]dto.PenaltyType}
// @Router /api/v1/penalty-types [get]
func (h *PenaltyHandler) ListTypes(c *gin.Context) {
	items, err := h.service.ListTypes(c.Request.Context(), appmiddleware.MustActor(c))
	if err != nil {
		response.FromError(c, err)
		return
	}
	mapped := make([]dto.PenaltyType, 0, len(items))
	for _, item := range items {
		mapped = append(mapped, dto.PenaltyTypeFromModel(item))
	}
	response.Success(c, http.StatusOK, "get penalty types successfully", mapped)
}

// CreatePenaltyType godoc
// @Summary Create a penalty type
// @Tags Penalty Types
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body dto.CreatePenaltyTypeRequest true "Penalty type"
// @Success 201 {object} response.Envelope{data=dto.PenaltyType}
// @Failure 403 {object} response.Envelope
// @Failure 409 {object} response.Envelope
// @Router /api/v1/penalty-types [post]
func (h *PenaltyHandler) CreateType(c *gin.Context) {
	var request dto.CreatePenaltyTypeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.FromError(c, validationError("body", "name is required"))
		return
	}
	item, err := h.service.CreateType(c.Request.Context(), appmiddleware.MustActor(c), request.Name, request.LegacyID)
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.Success(c, http.StatusCreated, "create penalty type successfully", dto.PenaltyTypeFromModel(item))
}

// DeletePenaltyType godoc
// @Summary Delete an unreferenced penalty type
// @Tags Penalty Types
// @Security BearerAuth
// @Param id path string true "Penalty type ID"
// @Success 200 {object} response.Envelope
// @Failure 409 {object} response.Envelope
// @Router /api/v1/penalty-types/{id} [delete]
func (h *PenaltyHandler) DeleteType(c *gin.Context) {
	if err := h.service.DeleteType(c.Request.Context(), appmiddleware.MustActor(c), c.Param("id")); err != nil {
		response.FromError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "delete penalty type successfully", nil)
}

// CreatePenalty godoc
// @Summary Create a disciplinary penalty with evidence
// @Description actionTypeIds accepts repeated form fields, a JSON string array, or comma-separated IDs/names/legacy PT identifiers. JPEG, PNG, and WebP evidence only.
// @Tags Penalties
// @Security BearerAuth
// @Accept multipart/form-data
// @Produce json
// @Param policeId formData string true "Police ID"
// @Param actionTypeIds formData []string true "One or more type identifiers"
// @Param reason formData string true "Reason"
// @Param durationType formData string true "permanent, months, or date_range"
// @Param months formData int false "Months for months duration"
// @Param startDate formData string false "YYYY-MM-DD; required for date_range"
// @Param endDate formData string false "Inclusive YYYY-MM-DD; required for date_range"
// @Param fineAmount formData int false "Fine in the smallest configured currency unit"
// @Param evidence formData file true "JPEG, PNG, or WebP evidence"
// @Success 201 {object} response.Envelope{data=dto.CreatePenaltyResult}
// @Failure 400 {object} response.Envelope
// @Failure 413 {object} response.Envelope
// @Router /api/v1/penalties [post]
func (h *PenaltyHandler) Create(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.maxBodyBytes)
	if err := c.Request.ParseMultipartForm(h.maxBodyBytes); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			response.Failure(c, http.StatusRequestEntityTooLarge, "evidence upload is too large", "PAYLOAD_TOO_LARGE", nil)
			return
		}
		response.FromError(c, validationError("evidence", "multipart request is invalid or too large"))
		return
	}
	header, err := c.FormFile("evidence")
	if err != nil {
		response.FromError(c, validationError("evidence", "image is required"))
		return
	}
	file, err := header.Open()
	if err != nil {
		response.FromError(c, validationError("evidence", "cannot read image"))
		return
	}
	defer file.Close()

	actions, err := multipartActions(c)
	if err != nil {
		response.FromError(c, err)
		return
	}
	var months *int
	if value := strings.TrimSpace(c.PostForm("months")); value != "" {
		parsed, parseErr := strconv.Atoi(value)
		if parseErr != nil {
			response.FromError(c, validationError("months", "must be an integer"))
			return
		}
		months = &parsed
	}
	var fineAmount *int64
	if value := strings.TrimSpace(c.PostForm("fineAmount")); value != "" {
		parsed, parseErr := strconv.ParseInt(value, 10, 64)
		if parseErr != nil {
			response.FromError(c, validationError("fineAmount", "must be an integer"))
			return
		}
		fineAmount = &parsed
	}
	created, err := h.service.Create(c.Request.Context(), appmiddleware.MustActor(c), service.PenaltyCommand{
		PoliceID: c.PostForm("policeId"), ActionTypeIDs: actions, Reason: c.PostForm("reason"), DurationType: c.PostForm("durationType"),
		Months: months, StartDate: c.PostForm("startDate"), EndDate: c.PostForm("endDate"), FineAmount: fineAmount,
		EvidenceName: header.Filename, Evidence: file,
	})
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.Success(c, http.StatusCreated, "create penalty successfully", dto.CreatePenaltyResult{PenaltyID: created.Penalty.PenaltyID, PenaltyCount: created.PenaltyCount, IsPermanent: created.Penalty.IsPermanent()})
}

// GetPenalty godoc
// @Summary Get penalty details
// @Tags Penalties
// @Security BearerAuth
// @Produce json
// @Param id path string true "Penalty ID"
// @Success 200 {object} response.Envelope{data=dto.Penalty}
// @Failure 404 {object} response.Envelope
// @Router /api/v1/penalties/{id} [get]
func (h *PenaltyHandler) Get(c *gin.Context) {
	penalty, err := h.service.Get(c.Request.Context(), appmiddleware.MustActor(c), c.Param("id"))
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "get penalty successfully", dto.PenaltyFromModel(penalty, h.clock()))
}

// PardonPenalty godoc
// @Summary Pardon a penalty without deleting its history
// @Tags Penalties
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Penalty ID"
// @Param request body dto.PardonPenaltyRequest true "Pardon reason"
// @Success 200 {object} response.Envelope{data=dto.PardonPenaltyResult}
// @Failure 409 {object} response.Envelope
// @Router /api/v1/penalties/{id}/pardon [post]
func (h *PenaltyHandler) Pardon(c *gin.Context) {
	var request dto.PardonPenaltyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.FromError(c, validationError("body", "reason is required"))
		return
	}
	result, err := h.service.Pardon(c.Request.Context(), appmiddleware.MustActor(c), c.Param("id"), request.Reason)
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "pardon penalty successfully", dto.PardonPenaltyResult{Penalty: dto.PenaltyFromModel(result.Penalty, h.clock()), PenaltyCount: result.PenaltyCount})
}

// MemberSummary godoc
// @Summary Get member disciplinary summary by Police ID
// @Tags Members
// @Security BearerAuth
// @Produce json
// @Param policeId path string true "Police ID"
// @Success 200 {object} response.Envelope{data=dto.MemberSummary}
// @Router /api/v1/members/by-police-id/{policeId}/summary [get]
func (h *PenaltyHandler) MemberSummary(c *gin.Context) {
	member, penalties, err := h.service.Summary(c.Request.Context(), appmiddleware.MustActor(c), c.Param("policeId"))
	if err != nil {
		response.FromError(c, err)
		return
	}
	mapped := mapPenalties(penalties, h.clock())
	permanent := false
	for _, penalty := range mapped {
		if penalty.IsActive && penalty.IsPermanent {
			permanent = true
			break
		}
	}
	response.Success(c, http.StatusOK, "get member summary successfully", dto.MemberSummary{Info: dto.MemberFromModel(member), Penalties: mapped, SeriousWarning: member.PenaltyCount >= 3 || permanent})
}

// MemberPenalties godoc
// @Summary List a member's complete penalty history
// @Tags Penalties
// @Security BearerAuth
// @Produce json
// @Param id path string true "Stable member ID"
// @Success 200 {object} response.Envelope{data=[]dto.Penalty}
// @Router /api/v1/members/{id}/penalties [get]
func (h *PenaltyHandler) MemberPenalties(c *gin.Context) {
	items, err := h.service.ListForMember(c.Request.Context(), appmiddleware.MustActor(c), c.Param("id"))
	if err != nil {
		response.FromError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "get member penalties successfully", mapPenalties(items, h.clock()))
}

// Evidence godoc
// @Summary Retrieve authorized penalty evidence
// @Tags Penalties
// @Security BearerAuth
// @Produce image/jpeg,image/png,image/webp
// @Param id path string true "Evidence ID"
// @Success 200 {file} binary
// @Failure 404 {object} response.Envelope
// @Router /api/v1/evidence/{id} [get]
func (h *PenaltyHandler) Evidence(c *gin.Context) {
	reader, contentType, filename, err := h.service.Evidence(c.Request.Context(), appmiddleware.MustActor(c), c.Param("id"))
	if err != nil {
		response.FromError(c, err)
		return
	}
	defer reader.Close()
	c.Header("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": filename}))
	c.Header("Content-Type", contentType)
	c.Header("Cache-Control", "private, no-store")
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, reader)
}

func multipartActions(c *gin.Context) ([]string, error) {
	values := append([]string{}, c.PostFormArray("actionTypeIds")...)
	values = append(values, c.PostFormArray("actionTypes")...)
	if one := strings.TrimSpace(c.PostForm("actionType")); one != "" {
		values = append(values, one)
	}
	if len(values) == 1 {
		var decoded []string
		if strings.HasPrefix(strings.TrimSpace(values[0]), "[") {
			if err := json.Unmarshal([]byte(values[0]), &decoded); err != nil {
				return nil, validationError("actionTypeIds", "JSON array is invalid")
			}
			values = decoded
		} else if strings.Contains(values[0], ",") {
			values = strings.Split(values[0], ",")
		}
	}
	return values, nil
}

func mapPenalties(items []model.Penalty, now time.Time) []dto.Penalty {
	result := make([]dto.Penalty, 0, len(items))
	for _, item := range items {
		result = append(result, dto.PenaltyFromModel(item, now))
	}
	return result
}
