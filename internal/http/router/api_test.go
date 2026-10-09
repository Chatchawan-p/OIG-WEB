package router

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/oig-police/oig-web/internal/authz"
	"github.com/oig-police/oig-web/internal/config"
	"github.com/oig-police/oig-web/internal/evidence"
	"github.com/oig-police/oig-web/internal/model"
	"github.com/oig-police/oig-web/internal/repository/memory"
	"github.com/oig-police/oig-web/internal/service"
	"golang.org/x/oauth2"
)

var apiTestNow = time.Date(2026, 10, 9, 9, 0, 0, 0, time.UTC)

func apiClock() time.Time { return apiTestNow }

func buildAPIRouter(t *testing.T, role model.Role) (*gin.Engine, string) {
	t.Helper()
	store := memory.New()
	user := model.User{UserID: "usr_test", DiscordID: "discord_test", DisplayName: "Tester", Role: role}
	store.SeedUsers(user)
	store.SeedMembers(model.PoliceMember{MemberID: "mem_1", PoliceID: "P-001", Generation: 12, FullName: "Test Officer", Department: "OIG", EmploymentStatus: "รับราชการ"})
	policy := authz.New()
	evidenceStore, err := evidence.NewLocal(t.TempDir(), 1024)
	if err != nil {
		t.Fatal(err)
	}
	secret := "01234567890123456789012345678901"
	authService := service.NewAuthService(store, oauth2.Config{ClientID: "client", RedirectURL: "http://localhost:18080/api/v1/auth/discord/callback", Endpoint: oauth2.Endpoint{AuthURL: "https://discord.example/authorize", TokenURL: "https://discord.example/token"}}, "https://discord.example/users/@me", secret, 15*time.Minute, apiClock)
	memberService := service.NewMemberService(store, policy, apiClock)
	penaltyService := service.NewPenaltyService(store, evidenceStore, policy, apiClock)
	adminService := service.NewAdminService(store, policy, apiClock)
	cfg := config.Config{AppEnv: "test", CORSAllowedOrigins: []string{"http://localhost:3000"}, JWTExpiration: 15 * time.Minute, EvidenceMaxBytes: 1024}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := New(cfg, log, checkerStub{}, Dependencies{Auth: authService, Members: memberService, Penalties: penaltyService, Admin: adminService, Policy: policy})
	claims := service.TokenClaims{Role: role, TokenVersion: 0, RegisteredClaims: jwt.RegisteredClaims{Issuer: "oig-api", Subject: user.UserID, Audience: jwt.ClaimStrings{"oig-frontend"}, IssuedAt: jwt.NewNumericDate(apiTestNow), ExpiresAt: jwt.NewNumericDate(apiTestNow.Add(15 * time.Minute))}}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	return router, token
}

func TestMemberListResponseCompatibility(t *testing.T) {
	router, token := buildAPIRouter(t, model.RoleAdmin)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/members?query=Test&genRangeFilter=10-20", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	data, ok := payload["data"].(map[string]any)
	if !ok || data["items"] == nil || data["groupedByGeneration"] == nil || data["pagination"] == nil {
		t.Fatalf("response contract missing fields: %s", recorder.Body.String())
	}
}

func TestAuthenticationAndRBACResponses(t *testing.T) {
	router, token := buildAPIRouter(t, model.RoleGuest)
	tests := []struct {
		name   string
		token  string
		status int
	}{
		{name: "missing token", status: http.StatusUnauthorized},
		{name: "guest forbidden", token: token, status: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/members", nil)
			if tt.token != "" {
				request.Header.Set("Authorization", "Bearer "+tt.token)
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != tt.status || !strings.Contains(recorder.Body.String(), `"error"`) {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestDiscordLoginMissingReturnTo(t *testing.T) {
	router, _ := buildAPIRouter(t, model.RoleAdmin)
	endpoints := []string{
		"/api/v1/auth/discord/login",
		"/api/v1/auth/discord/login?return_to=",
		"/api/v1/auth/discord/login?return_to=%20",
	}

	for _, endpoint := range endpoints {
		request := httptest.NewRequest(http.MethodGet, endpoint, nil)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusFound {
			t.Fatalf("endpoint %s expected status 302, got %d (body: %s)", endpoint, recorder.Code, recorder.Body.String())
		}
		loc := recorder.Header().Get("Location")
		if !strings.Contains(loc, "discord") || !strings.Contains(loc, "state=") {
			t.Fatalf("unexpected Location header: %q", loc)
		}

		cookies := recorder.Result().Cookies()
		var stateCookie, returnToCookie *http.Cookie
		for _, c := range cookies {
			if c.Name == "oig_oauth_state" {
				stateCookie = c
			}
			if c.Name == "oig_return_to" {
				returnToCookie = c
			}
		}
		if stateCookie == nil || stateCookie.Value == "" {
			t.Fatalf("expected oig_oauth_state cookie to be set")
		}
		if returnToCookie != nil && returnToCookie.Value != "" {
			t.Fatalf("expected oig_return_to cookie to NOT be set, got %v", returnToCookie)
		}
	}
}

func TestDiscordLoginRedirectURIConfiguration(t *testing.T) {
	router, _ := buildAPIRouter(t, model.RoleAdmin)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/discord/login", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", recorder.Code)
	}

	loc := recorder.Header().Get("Location")
	if strings.Contains(loc, "script.google.com") || strings.Contains(loc, "macros") {
		t.Fatalf("Location contains legacy Google Apps Script URL: %s", loc)
	}

	parsedURL, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("failed to parse redirect url: %v", err)
	}

	redirectURI := parsedURL.Query().Get("redirect_uri")
	expectedRedirectURI := "http://localhost:18080/api/v1/auth/discord/callback"
	if redirectURI != expectedRedirectURI {
		t.Fatalf("expected redirect_uri = %q, got %q (full URL: %s)", expectedRedirectURI, redirectURI, loc)
	}

	// Verify that the query string contains properly encoded redirect_uri
	if !strings.Contains(loc, "redirect_uri="+url.QueryEscape(expectedRedirectURI)) {
		t.Fatalf("expected encoded redirect_uri in Location header: %s", loc)
	}
}

func TestDiscordLoginValidReturnTo(t *testing.T) {
	router, _ := buildAPIRouter(t, model.RoleAdmin)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/discord/login?return_to=http://localhost:3000/dashboard", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", recorder.Code)
	}

	cookies := recorder.Result().Cookies()
	var stateCookie, returnToCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "oig_oauth_state" {
			stateCookie = c
		}
		if c.Name == "oig_return_to" {
			returnToCookie = c
		}
	}
	if stateCookie == nil || stateCookie.Value == "" {
		t.Fatalf("expected oig_oauth_state cookie to be set")
	}
	if returnToCookie == nil {
		t.Fatal("expected oig_return_to cookie to be set")
	}
	unescapedReturnTo, _ := url.QueryUnescape(returnToCookie.Value)
	if unescapedReturnTo != "http://localhost:3000/dashboard" {
		t.Fatalf("expected oig_return_to cookie with destination http://localhost:3000/dashboard, got %q", unescapedReturnTo)
	}
}

func TestDiscordLoginInvalidExternalReturnTo(t *testing.T) {
	router, _ := buildAPIRouter(t, model.RoleAdmin)
	badURLs := []string{
		"https://evil.example.com",
		"http://attacker.com/oauth/steal",
		"http://localhost:18080/api/v1/auth/discord/callback",
		"javascript:alert(1)",
	}

	for _, badURL := range badURLs {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/discord/login?return_to="+url.QueryEscape(badURL), nil)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("url %q expected status 400, got %d (body: %s)", badURL, recorder.Code, recorder.Body.String())
		}
		if !strings.Contains(recorder.Body.String(), "VALIDATION_ERROR") || !strings.Contains(recorder.Body.String(), "return_to") {
			t.Fatalf("expected validation error for return_to, got %s", recorder.Body.String())
		}
	}
}

func TestOAuthStateGeneration(t *testing.T) {
	router, _ := buildAPIRouter(t, model.RoleAdmin)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/discord/login", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusFound {
		t.Fatalf("expected status 302, got %d", recorder.Code)
	}

	loc := recorder.Header().Get("Location")
	parsedURL, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("failed to parse redirect url: %v", err)
	}
	queryState := parsedURL.Query().Get("state")
	if queryState == "" || !strings.HasPrefix(queryState, "state_") {
		t.Fatalf("expected secure state query param, got %q", queryState)
	}

	cookies := recorder.Result().Cookies()
	var stateCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "oig_oauth_state" {
			stateCookie = c
			break
		}
	}
	if stateCookie == nil || stateCookie.Value != queryState {
		t.Fatalf("cookie state %v does not match query state %q", stateCookie, queryState)
	}
}

func TestOAuthStateMismatch(t *testing.T) {
	router, _ := buildAPIRouter(t, model.RoleAdmin)

	// Case 1: missing cookie
	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/auth/discord/callback?code=some_code&state=original_state", nil)
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when cookie is missing, got %d", rec1.Code)
	}

	// Case 2: mismatched cookie
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/auth/discord/callback?code=some_code&state=attack_state", nil)
	req2.AddCookie(&http.Cookie{Name: "oig_oauth_state", Value: "legit_state"})
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 on state mismatch, got %d", rec2.Code)
	}
}

func TestOAuthCallbackMissingCode(t *testing.T) {
	router, _ := buildAPIRouter(t, model.RoleAdmin)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/discord/callback?state=legit_state", nil)
	req.AddCookie(&http.Cookie{Name: "oig_oauth_state", Value: "legit_state"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when code is missing, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "VALIDATION_ERROR") || !strings.Contains(rec.Body.String(), "code") {
		t.Fatalf("expected VALIDATION_ERROR for code, got %s", rec.Body.String())
	}
}

func TestJWTValidationBearerAndCookie(t *testing.T) {
	router, token := buildAPIRouter(t, model.RoleAdmin)

	// 1. Authorization: Bearer
	reqBearer := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	reqBearer.Header.Set("Authorization", "Bearer "+token)
	recBearer := httptest.NewRecorder()
	router.ServeHTTP(recBearer, reqBearer)
	if recBearer.Code != http.StatusOK {
		t.Fatalf("expected 200 with Bearer token, got %d", recBearer.Code)
	}
	if !strings.Contains(recBearer.Body.String(), "usr_test") {
		t.Fatalf("expected response to contain user details, got %s", recBearer.Body.String())
	}

	// 2. Cookie authentication
	reqCookie := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	reqCookie.AddCookie(&http.Cookie{Name: "oig_access_token", Value: token})
	recCookie := httptest.NewRecorder()
	router.ServeHTTP(recCookie, reqCookie)
	if recCookie.Code != http.StatusOK {
		t.Fatalf("expected 200 with cookie authentication, got %d", recCookie.Code)
	}
}

func TestJWTValidationExpired(t *testing.T) {
	router, _ := buildAPIRouter(t, model.RoleAdmin)
	secret := "01234567890123456789012345678901"

	// Create expired claims (expired 1 hour ago)
	claims := service.TokenClaims{
		Role:         model.RoleAdmin,
		TokenVersion: 0,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "oig-api",
			Subject:   "usr_test",
			Audience:  jwt.ClaimStrings{"oig-frontend"},
			IssuedAt:  jwt.NewNumericDate(apiTestNow.Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(apiTestNow.Add(-1 * time.Hour)),
		},
	}
	expiredToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+expiredToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for expired JWT, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "UNAUTHORIZED") {
		t.Fatalf("expected UNAUTHORIZED code, got %s", rec.Body.String())
	}
}

func TestRoleAuthorizationHierarchy(t *testing.T) {
	routerAdmin, tokenAdmin := buildAPIRouter(t, model.RoleAdmin)
	routerOwner, tokenOwner := buildAPIRouter(t, model.RoleOwner)

	// Admin trying to write user roles -> forbidden
	reqAdmin := httptest.NewRequest(http.MethodPatch, "/api/v1/users/usr_target/role", strings.NewReader(`{"role":"Guest"}`))
	reqAdmin.Header.Set("Authorization", "Bearer "+tokenAdmin)
	reqAdmin.Header.Set("Content-Type", "application/json")
	recAdmin := httptest.NewRecorder()
	routerAdmin.ServeHTTP(recAdmin, reqAdmin)
	if recAdmin.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for Admin role change, got %d (body: %s)", recAdmin.Code, recAdmin.Body.String())
	}

	// Owner reading users -> allowed
	reqOwner := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	reqOwner.Header.Set("Authorization", "Bearer "+tokenOwner)
	recOwner := httptest.NewRecorder()
	routerOwner.ServeHTTP(recOwner, reqOwner)
	if recOwner.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for Owner reading users, got %d (body: %s)", recOwner.Code, recOwner.Body.String())
	}
}
