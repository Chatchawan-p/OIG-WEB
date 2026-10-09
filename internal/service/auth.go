package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/oig-police/oig-web/internal/apperror"
	"github.com/oig-police/oig-web/internal/model"
	"github.com/oig-police/oig-web/internal/platform/id"
	"github.com/oig-police/oig-web/internal/repository"
	"golang.org/x/oauth2"
)

const (
	jwtIssuer   = "oig-api"
	jwtAudience = "oig-frontend"
)

type TokenClaims struct {
	Role         model.Role `json:"role"`
	TokenVersion int64      `json:"tokenVersion"`
	jwt.RegisteredClaims
}

type DiscordIdentity struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
}

type AuthResult struct {
	AccessToken string
	ExpiresIn   int64
	User        model.User
}

type AuthService struct {
	users          repository.UserRepository
	oauth          oauth2.Config
	discordUserURL string
	jwtSecret      []byte
	tokenTTL       time.Duration
	now            Clock
}

func NewAuthService(users repository.UserRepository, oauthConfig oauth2.Config, discordUserURL, jwtSecret string, tokenTTL time.Duration, now Clock) *AuthService {
	return &AuthService{users: users, oauth: oauthConfig, discordUserURL: discordUserURL, jwtSecret: []byte(jwtSecret), tokenTTL: tokenTTL, now: now}
}

func (s *AuthService) AuthorizationURL(state string) string {
	return s.oauth.AuthCodeURL(state, oauth2.AccessTypeOnline)
}

func (s *AuthService) Callback(ctx context.Context, code string) (AuthResult, error) {
	if strings.TrimSpace(code) == "" {
		return AuthResult{}, &apperror.ValidationError{Details: []apperror.FieldError{{Field: "code", Message: "is required"}}}
	}
	token, err := s.oauth.Exchange(ctx, code)
	if err != nil {
		return AuthResult{}, fmt.Errorf("discord authorization code exchange: %v: %w", err, apperror.ErrUnauthorized)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.discordUserURL, nil)
	if err != nil {
		return AuthResult{}, err
	}
	request.Header.Set("Authorization", "Bearer "+token.AccessToken)
	response, err := s.oauth.Client(ctx, token).Do(request)
	if err != nil {
		return AuthResult{}, fmt.Errorf("fetch discord identity: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return AuthResult{}, apperror.ErrUnauthorized
	}
	var identity DiscordIdentity
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&identity); err != nil {
		return AuthResult{}, fmt.Errorf("decode discord identity: %w", err)
	}
	if identity.ID == "" {
		return AuthResult{}, apperror.ErrUnauthorized
	}
	user, err := s.users.FindUserByDiscordID(ctx, identity.ID)
	if errors.Is(err, apperror.ErrNotFound) {
		return AuthResult{}, apperror.ErrForbidden
	}
	if err != nil {
		return AuthResult{}, err
	}
	return s.issue(user)
}

func (s *AuthService) issue(user model.User) (AuthResult, error) {
	now := s.now()
	jti, err := id.New("jti")
	if err != nil {
		return AuthResult{}, err
	}
	claims := TokenClaims{
		Role: user.Role, TokenVersion: user.TokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: jwtIssuer, Subject: user.UserID, Audience: jwt.ClaimStrings{jwtAudience},
			ExpiresAt: jwt.NewNumericDate(now.Add(s.tokenTTL)), IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now.Add(-5 * time.Second)), ID: jti,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return AuthResult{}, fmt.Errorf("sign access token: %w", err)
	}
	return AuthResult{AccessToken: signed, ExpiresIn: int64(s.tokenTTL.Seconds()), User: user}, nil
}

func (s *AuthService) Authenticate(ctx context.Context, rawToken string) (model.User, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return model.User{}, apperror.ErrUnauthorized
	}
	claims := &TokenClaims{}
	parsed, err := jwt.ParseWithClaims(rawToken, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, apperror.ErrUnauthorized
		}
		return s.jwtSecret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer(jwtIssuer), jwt.WithAudience(jwtAudience), jwt.WithLeeway(30*time.Second), jwt.WithTimeFunc(s.now))
	if err != nil || !parsed.Valid || claims.Subject == "" {
		return model.User{}, apperror.ErrUnauthorized
	}
	user, err := s.users.FindUserByID(ctx, claims.Subject)
	if err != nil {
		return model.User{}, apperror.ErrUnauthorized
	}
	if user.TokenVersion != claims.TokenVersion || user.Role != claims.Role {
		return model.User{}, apperror.ErrUnauthorized
	}
	return user, nil
}

func (s *AuthService) Logout(ctx context.Context, user model.User) error {
	return s.users.IncrementTokenVersion(ctx, user.UserID, s.now())
}
