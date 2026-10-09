package dto

import "time"

type PenaltyRuleRequest struct {
	Name        string `json:"name" binding:"required"`
	Category    string `json:"category" binding:"required"`
	Fine        string `json:"fine"`
	Jail        string `json:"jail"`
	Description string `json:"description"`
}

type PenaltyRule struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Category    string    `json:"category"`
	Fine        string    `json:"fine"`
	Jail        string    `json:"jail"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type User struct {
	ID        string    `json:"id"`
	DiscordID string    `json:"discordId"`
	Name      string    `json:"name"`
	Avatar    string    `json:"avatar,omitempty"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type UpdateRoleRequest struct {
	Role string `json:"role" binding:"required" enums:"Screener,Admin,Guest"`
}

type UserList struct {
	Items      []User     `json:"items"`
	Pagination Pagination `json:"pagination"`
}

type AuditLog struct {
	ID           string    `json:"id"`
	ActorID      string    `json:"actorId"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resourceType"`
	ResourceID   string    `json:"resourceId"`
	Description  string    `json:"description"`
	CreatedAt    time.Time `json:"createdAt"`
}

type AuditLogList struct {
	Items      []AuditLog `json:"items"`
	Pagination Pagination `json:"pagination"`
}

type Departments struct {
	Items []string `json:"items"`
}
