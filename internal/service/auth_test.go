package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/oig-police/oig-web/internal/apperror"
	"github.com/oig-police/oig-web/internal/model"
	"github.com/oig-police/oig-web/internal/repository/memory"
	"golang.org/x/oauth2"
)

func TestJWTAuthenticationAndLogoutRevocation(t *testing.T) {
	store := memory.New()
	user := model.User{UserID: "usr_1", DiscordID: "discord_1", Role: model.RoleAdmin, TokenVersion: 2}
	store.SeedUsers(user)
	service := NewAuthService(store, oauth2.Config{}, "https://example.invalid", "01234567890123456789012345678901", 15*time.Minute, fixedTime)
	issued, err := service.issue(user)
	if err != nil {
		t.Fatal(err)
	}
	authenticated, err := service.Authenticate(context.Background(), issued.AccessToken)
	if err != nil || authenticated.UserID != user.UserID {
		t.Fatalf("Authenticate() = %#v, %v", authenticated, err)
	}
	if err := service.Logout(context.Background(), user); err != nil {
		t.Fatal(err)
	}
	_, err = service.Authenticate(context.Background(), issued.AccessToken)
	if !errors.Is(err, apperror.ErrUnauthorized) {
		t.Fatalf("revoked Authenticate() error = %v", err)
	}
}

func TestJWTRejectsTampering(t *testing.T) {
	store := memory.New()
	user := model.User{UserID: "usr_1", Role: model.RoleAdmin}
	store.SeedUsers(user)
	service := NewAuthService(store, oauth2.Config{}, "https://example.invalid", "01234567890123456789012345678901", time.Minute, fixedTime)
	issued, _ := service.issue(user)
	_, err := service.Authenticate(context.Background(), issued.AccessToken+"tampered")
	if !errors.Is(err, apperror.ErrUnauthorized) {
		t.Fatalf("Authenticate() error = %v", err)
	}
}

func TestJWTRejectsExpiredToken(t *testing.T) {
	store := memory.New()
	user := model.User{UserID: "usr_1", Role: model.RoleAdmin}
	store.SeedUsers(user)
	currentTime := fixedTime()
	clock := func() time.Time { return currentTime }

	service := NewAuthService(store, oauth2.Config{}, "https://example.invalid", "01234567890123456789012345678901", 10*time.Minute, clock)
	issued, err := service.issue(user)
	if err != nil {
		t.Fatal(err)
	}

	// Advance time past expiry + leeway
	currentTime = currentTime.Add(15 * time.Minute)

	_, err = service.Authenticate(context.Background(), issued.AccessToken)
	if !errors.Is(err, apperror.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized for expired token, got %v", err)
	}
}

func TestCallbackMissingAuthorizationCode(t *testing.T) {
	store := memory.New()
	service := NewAuthService(store, oauth2.Config{}, "https://example.invalid", "01234567890123456789012345678901", time.Minute, fixedTime)

	_, err := service.Callback(context.Background(), "")
	var valErr *apperror.ValidationError
	if !errors.As(err, &valErr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	if len(valErr.Details) != 1 || valErr.Details[0].Field != "code" {
		t.Fatalf("unexpected validation details: %#v", valErr.Details)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestCallbackUnauthorizedDiscordUser(t *testing.T) {
	mockClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/token":
				body := `{"access_token":"mock_token","token_type":"Bearer","expires_in":3600}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			case "/users/@me":
				body := `{"id":"unknown_discord_id","username":"intruder"}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			default:
				return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}, nil
			}
		}),
	}

	store := memory.New() // store has no users
	oauthConfig := oauth2.Config{
		ClientID:     "client",
		ClientSecret: "secret",
		Endpoint:     oauth2.Endpoint{TokenURL: "https://discord.example/token"},
	}
	service := NewAuthService(store, oauthConfig, "https://discord.example/users/@me", "01234567890123456789012345678901", 15*time.Minute, fixedTime)

	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, mockClient)
	_, err := service.Callback(ctx, "auth_code")
	if !errors.Is(err, apperror.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for unprovisioned discord user, got %v", err)
	}
}

func TestCallbackSuccessAndJWTIssue(t *testing.T) {
	mockClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/token":
				body := `{"access_token":"mock_token","token_type":"Bearer","expires_in":3600}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			case "/users/@me":
				body := `{"id":"discord_valid","username":"officer","avatar":"avatar_123"}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			default:
				return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}, nil
			}
		}),
	}

	store := memory.New()
	user := model.User{UserID: "usr_legit", DiscordID: "discord_valid", DisplayName: "Officer Dave", Role: model.RoleAdmin, TokenVersion: 1}
	store.SeedUsers(user)

	oauthConfig := oauth2.Config{
		ClientID:     "client",
		ClientSecret: "secret",
		Endpoint:     oauth2.Endpoint{TokenURL: "https://discord.example/token"},
	}
	service := NewAuthService(store, oauthConfig, "https://discord.example/users/@me", "01234567890123456789012345678901", 15*time.Minute, fixedTime)

	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, mockClient)
	result, err := service.Callback(ctx, "valid_code")
	if err != nil {
		t.Fatalf("Callback() error = %v", err)
	}
	if result.AccessToken == "" || result.User.UserID != user.UserID {
		t.Fatalf("Callback() unexpected result: %#v", result)
	}

	// Verify that the issued JWT validates successfully
	authed, err := service.Authenticate(context.Background(), result.AccessToken)
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if authed.UserID != user.UserID || authed.Role != model.RoleAdmin {
		t.Fatalf("Authenticate() unexpected user: %#v", authed)
	}
}
