package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/oig-police/oig-web/internal/model"
	"github.com/oig-police/oig-web/internal/platform/id"
)

type Clock func() time.Time

func UTCNow() time.Time { return time.Now().UTC() }

func newAudit(actor model.User, action, resourceType, resourceID, description string, now time.Time) (model.AuditLog, error) {
	auditID, err := id.New("aud")
	if err != nil {
		return model.AuditLog{}, err
	}
	return model.AuditLog{
		AuditID: auditID, ActorID: actor.UserID, Action: action, ResourceType: resourceType,
		ResourceID: resourceID, Description: strings.TrimSpace(description), CreatedAt: now,
	}, nil
}

func trimLimited(value string, max int) (string, error) {
	value = strings.TrimSpace(value)
	if len([]rune(value)) > max {
		return "", fmt.Errorf("must not exceed %d characters", max)
	}
	return value, nil
}
