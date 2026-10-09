package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/oig-police/oig-web/internal/apperror"
	"github.com/oig-police/oig-web/internal/authz"
	"github.com/oig-police/oig-web/internal/http/response"
	"github.com/oig-police/oig-web/internal/model"
)

const (
	actorContextKey  = "authenticated_actor"
	AccessCookieName = "oig_access_token"
)

type Authenticator interface {
	Authenticate(context.Context, string) (model.User, error)
}

func Authentication(authenticator Authenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := ""
		header := strings.TrimSpace(c.GetHeader("Authorization"))
		if header != "" {
			parts := strings.SplitN(header, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				response.Failure(c, http.StatusUnauthorized, "invalid authorization header", "UNAUTHORIZED", nil)
				c.Abort()
				return
			}
			raw = strings.TrimSpace(parts[1])
		} else if cookie, err := c.Cookie(AccessCookieName); err == nil {
			raw = cookie
		}
		actor, err := authenticator.Authenticate(c.Request.Context(), raw)
		if err != nil {
			response.Failure(c, http.StatusUnauthorized, "authentication required", "UNAUTHORIZED", nil)
			c.Abort()
			return
		}
		c.Set(actorContextKey, actor)
		c.Next()
	}
}

func Require(policy *authz.Policy, action authz.Action) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := Actor(c)
		if !ok {
			response.Failure(c, http.StatusUnauthorized, "authentication required", "UNAUTHORIZED", nil)
			c.Abort()
			return
		}
		if !policy.Can(actor.Role, action) {
			response.Failure(c, http.StatusForbidden, "permission denied", "FORBIDDEN", nil)
			c.Abort()
			return
		}
		c.Next()
	}
}

func Actor(c *gin.Context) (model.User, bool) {
	value, ok := c.Get(actorContextKey)
	if !ok {
		return model.User{}, false
	}
	actor, ok := value.(model.User)
	return actor, ok
}

func MustActor(c *gin.Context) model.User {
	actor, ok := Actor(c)
	if !ok {
		panic(apperror.ErrUnauthorized)
	}
	return actor
}
