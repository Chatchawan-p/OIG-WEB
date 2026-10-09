package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/oig-police/oig-web/internal/apperror"
	"github.com/oig-police/oig-web/internal/dto"
	appmiddleware "github.com/oig-police/oig-web/internal/http/middleware"
	"github.com/oig-police/oig-web/internal/http/response"
	"github.com/oig-police/oig-web/internal/platform/id"
	"github.com/oig-police/oig-web/internal/service"
)

const (
	oauthStateCookie = "oig_oauth_state"
	returnToCookie   = "oig_return_to"
)

type AuthHandler struct {
	service        *service.AuthService
	allowedOrigins []string
	secureCookies  bool
}

func NewAuthHandler(service *service.AuthService, allowedOrigins []string, secureCookies bool) *AuthHandler {
	return &AuthHandler{service: service, allowedOrigins: allowedOrigins, secureCookies: secureCookies}
}

// DiscordLogin godoc
// @Summary Start Discord OAuth2 login
// @Description Initiates Discord OAuth2 authentication. Sets a short-lived state cookie and redirects (HTTP 302) to Discord. Optional return_to must use an allowed frontend origin.
// @Tags Authentication
// @Param return_to query string false "Optional frontend destination URL after successful login (e.g. http://localhost:3000). Must match an allowed frontend origin. Do NOT use DISCORD_REDIRECT_URI here." example(http://localhost:3000)
// @Success 302
// @Failure 400 {object} response.Envelope
// @Router /api/v1/auth/discord/login [get]
func (h *AuthHandler) DiscordLogin(c *gin.Context) {
	state, err := id.New("state")
	if err != nil {
		response.FromError(c, err)
		return
	}
	if returnTo := strings.TrimSpace(c.Query("return_to")); returnTo != "" {
		value, ok := allowedReturnURL(returnTo, h.allowedOrigins)
		if !ok {
			response.FromError(c, validationError("return_to", "must use an allowed frontend origin"))
			return
		}
		h.setCookie(c, returnToCookie, value, 300, "/api/v1/auth/discord/callback")
	}
	h.setCookie(c, oauthStateCookie, state, 300, "/api/v1/auth/discord/callback")
	c.Redirect(http.StatusFound, h.service.AuthorizationURL(state))
}

// DiscordCallback godoc
// @Summary Complete Discord OAuth2 login
// @Description Validates OAuth state, exchanges the code server-side, resolves a pre-provisioned user, and issues a JWT. If return_to was set, redirects (HTTP 303) to that destination; otherwise returns JSON.
// @Tags Authentication
// @Produce json
// @Param code query string true "Discord authorization code"
// @Param state query string true "OAuth state"
// @Success 200 {object} response.Envelope{data=dto.AuthToken}
// @Failure 400 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Failure 403 {object} response.Envelope
// @Router /api/v1/auth/discord/callback [get]
func (h *AuthHandler) DiscordCallback(c *gin.Context) {
	expectedState, err := c.Cookie(oauthStateCookie)
	if err != nil || expectedState == "" || c.Query("state") == "" || expectedState != c.Query("state") {
		response.FromError(c, apperror.ErrUnauthorized)
		return
	}
	h.clearCookie(c, oauthStateCookie, "/api/v1/auth/discord/callback")
	result, err := h.service.Callback(c.Request.Context(), c.Query("code"))
	if err != nil {
		response.FromError(c, err)
		return
	}
	h.setCookie(c, appmiddleware.AccessCookieName, result.AccessToken, int(result.ExpiresIn), "/api/v1")
	if returnTo, cookieErr := c.Cookie(returnToCookie); cookieErr == nil {
		h.clearCookie(c, returnToCookie, "/api/v1/auth/discord/callback")
		if safe, ok := allowedReturnURL(returnTo, h.allowedOrigins); ok {
			c.Redirect(http.StatusSeeOther, safe)
			return
		}
	}
	response.Success(c, http.StatusOK, "login completed successfully", dto.AuthToken{AccessToken: result.AccessToken, TokenType: "Bearer", ExpiresIn: result.ExpiresIn, User: dto.AuthUserFromModel(result.User)})
}

// Me godoc
// @Summary Get current user
// @Tags Authentication
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope{data=dto.AuthUser}
// @Failure 401 {object} response.Envelope
// @Router /api/v1/auth/me [get]
func (h *AuthHandler) Me(c *gin.Context) {
	actor := appmiddleware.MustActor(c)
	response.Success(c, http.StatusOK, "get current user successfully", dto.AuthUserFromModel(actor))
}

// Logout godoc
// @Summary Revoke current user's active tokens
// @Tags Authentication
// @Security BearerAuth
// @Produce json
// @Success 200 {object} response.Envelope
// @Failure 401 {object} response.Envelope
// @Router /api/v1/auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	actor := appmiddleware.MustActor(c)
	if err := h.service.Logout(c.Request.Context(), actor); err != nil {
		response.FromError(c, err)
		return
	}
	h.clearCookie(c, appmiddleware.AccessCookieName, "/api/v1")
	response.Success(c, http.StatusOK, "logout completed successfully", nil)
}

func (h *AuthHandler) setCookie(c *gin.Context, name, value string, maxAge int, path string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(name, value, maxAge, path, "", h.secureCookies, true)
}

func (h *AuthHandler) clearCookie(c *gin.Context, name, path string) {
	h.setCookie(c, name, "", -1, path)
}
