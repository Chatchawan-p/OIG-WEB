// Package model defines MongoDB persistence documents. API DTOs must not expose ObjectIDs.
package model

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Role string

const (
	RoleOwner    Role = "Owner"
	RoleScreener Role = "Screener"
	RoleAdmin    Role = "Admin"
	RoleGuest    Role = "Guest"
)

type User struct {
	ID           primitive.ObjectID `bson:"_id,omitempty"`
	UserID       string             `bson:"user_id"`
	DiscordID    string             `bson:"discord_id"`
	DisplayName  string             `bson:"display_name"`
	AvatarURL    string             `bson:"avatar_url,omitempty"`
	Role         Role               `bson:"role"`
	TokenVersion int64              `bson:"token_version"`
	CreatedAt    time.Time          `bson:"created_at"`
	UpdatedAt    time.Time          `bson:"updated_at"`
}

type PoliceMember struct {
	ID               primitive.ObjectID `bson:"_id,omitempty"`
	MemberID         string             `bson:"member_id"`
	PoliceID         string             `bson:"police_id"`
	Generation       int                `bson:"generation"`
	Rank             string             `bson:"rank,omitempty"`
	MedicalPrefix    string             `bson:"medical_prefix,omitempty"`
	FullName         string             `bson:"full_name"`
	PenaltyCount     int                `bson:"penalty_count"`
	EmploymentStatus string             `bson:"employment_status"`
	Department       string             `bson:"department,omitempty"`
	Unit             string             `bson:"unit,omitempty"`
	CreatedAt        time.Time          `bson:"created_at"`
	UpdatedAt        time.Time          `bson:"updated_at"`
	DeletedAt        *time.Time         `bson:"deleted_at,omitempty"`
}

type NameHistory struct {
	ID         primitive.ObjectID `bson:"_id,omitempty"`
	HistoryID  string             `bson:"history_id"`
	MemberID   string             `bson:"member_id"`
	PoliceID   string             `bson:"police_id"`
	OldName    string             `bson:"old_name"`
	NewName    string             `bson:"new_name"`
	Generation int                `bson:"generation"`
	ChangedAt  time.Time          `bson:"changed_at"`
	ChangedBy  string             `bson:"changed_by"`
}

type DurationType string

const (
	DurationPermanent DurationType = "permanent"
	DurationMonths    DurationType = "months"
	DurationDateRange DurationType = "date_range"
)

type Penalty struct {
	ID               primitive.ObjectID `bson:"_id,omitempty"`
	PenaltyID        string             `bson:"penalty_id"`
	MemberID         string             `bson:"member_id"`
	PoliceID         string             `bson:"police_id"`
	PenaltyTypeIDs   []string           `bson:"penalty_type_ids"`
	PenaltyTypeNames []string           `bson:"penalty_type_names"`
	Reason           string             `bson:"reason"`
	DurationType     DurationType       `bson:"duration_type"`
	DurationMonths   *int               `bson:"duration_months,omitempty"`
	StartsAt         *time.Time         `bson:"starts_at,omitempty"`
	ExpiresAt        *time.Time         `bson:"expires_at,omitempty"`
	EvidenceID       string             `bson:"evidence_id,omitempty"`
	EvidencePath     string             `bson:"evidence_path,omitempty"`
	EvidenceType     string             `bson:"evidence_type,omitempty"`
	EvidenceName     string             `bson:"evidence_name,omitempty"`
	FineAmount       *int64             `bson:"fine_amount,omitempty"`
	CreatedBy        string             `bson:"created_by"`
	CreatedAt        time.Time          `bson:"created_at"`
	PardonedAt       *time.Time         `bson:"pardoned_at,omitempty"`
	PardonedBy       string             `bson:"pardoned_by,omitempty"`
	PardonReason     string             `bson:"pardon_reason,omitempty"`
}

func (p Penalty) IsPermanent() bool { return p.DurationType == DurationPermanent }

func (p Penalty) IsActive(at time.Time) bool {
	if p.PardonedAt != nil {
		return false
	}
	if p.StartsAt != nil && at.Before(*p.StartsAt) {
		return false
	}
	if p.IsPermanent() {
		return true
	}
	return p.ExpiresAt != nil && at.Before(*p.ExpiresAt)
}

type PenaltyType struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	TypeID    string             `bson:"type_id"`
	LegacyID  string             `bson:"legacy_id,omitempty"`
	Name      string             `bson:"name"`
	CreatedAt time.Time          `bson:"created_at"`
	CreatedBy string             `bson:"created_by"`
}

type PenaltyRule struct {
	ID          primitive.ObjectID `bson:"_id,omitempty"`
	RuleID      string             `bson:"rule_id"`
	Name        string             `bson:"name"`
	Category    string             `bson:"category"`
	Fine        string             `bson:"fine"`
	Jail        string             `bson:"jail"`
	Description string             `bson:"description,omitempty"`
	CreatedAt   time.Time          `bson:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at"`
}

type AuditLog struct {
	ID           primitive.ObjectID `bson:"_id,omitempty"`
	AuditID      string             `bson:"audit_id"`
	ActorID      string             `bson:"actor_id"`
	Action       string             `bson:"action"`
	ResourceType string             `bson:"resource_type"`
	ResourceID   string             `bson:"resource_id"`
	Description  string             `bson:"description"`
	CreatedAt    time.Time          `bson:"created_at"`
}
