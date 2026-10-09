package dto

import "time"

type PenaltyType struct {
	ID       string `json:"id"`
	LegacyID string `json:"legacyId,omitempty"`
	Name     string `json:"name"`
}

type CreatePenaltyTypeRequest struct {
	Name     string `json:"name" binding:"required" example:"Written warning"`
	LegacyID string `json:"legacyId,omitempty" example:"PT001"`
}

type Penalty struct {
	ID            string     `json:"id"`
	MemberID      string     `json:"memberId"`
	PoliceID      string     `json:"policeId"`
	ActionTypeIDs []string   `json:"actionTypeIds"`
	ActionTypes   []string   `json:"actionTypes"`
	Reason        string     `json:"reason"`
	DurationType  string     `json:"durationType" enums:"permanent,months,date_range"`
	Months        *int       `json:"months,omitempty"`
	StartDate     *time.Time `json:"startDate,omitempty"`
	ExpiryDate    *time.Time `json:"expiryDate,omitempty"`
	FineAmount    *int64     `json:"fineAmount,omitempty"`
	EvidenceID    string     `json:"evidenceId,omitempty"`
	EvidenceURL   string     `json:"evidenceUrl,omitempty"`
	CreatedBy     string     `json:"createdBy"`
	CreatedAt     time.Time  `json:"createdAt"`
	PardonedAt    *time.Time `json:"pardonedAt,omitempty"`
	PardonedBy    string     `json:"pardonedBy,omitempty"`
	PardonReason  string     `json:"pardonReason,omitempty"`
	IsActive      bool       `json:"isActive"`
	IsPermanent   bool       `json:"isPermanent"`
}

type CreatePenaltyResult struct {
	PenaltyID    string `json:"penaltyId"`
	PenaltyCount int    `json:"penaltyCount"`
	IsPermanent  bool   `json:"isPermanent"`
}

type PardonPenaltyRequest struct {
	Reason string `json:"reason" binding:"required" example:"Pardoned after review"`
}

type PardonPenaltyResult struct {
	Penalty      Penalty `json:"penalty"`
	PenaltyCount int     `json:"penaltyCount"`
}

type MemberSummary struct {
	Info           Member    `json:"info"`
	Penalties      []Penalty `json:"penalties"`
	SeriousWarning bool      `json:"seriousWarning"`
}
