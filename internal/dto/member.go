package dto

import "time"

type CreateMemberRequest struct {
	PoliceID         string `json:"policeId" binding:"required" example:"OIG-001"`
	Generation       int    `json:"generation" binding:"required,min=1" example:"12"`
	Rank             string `json:"rank" example:"Pol. Col."`
	MedicalPrefix    string `json:"medicalPrefix" example:"Dr."`
	FullName         string `json:"fullName" binding:"required" example:"Somchai Jaidee"`
	EmploymentStatus string `json:"employmentStatus" example:"รับราชการ"`
	Department       string `json:"department" example:"Internal Affairs"`
	Unit             string `json:"unit" example:"Investigation"`
}

type BulkCreateMembersRequest struct {
	Members []CreateMemberRequest `json:"members" binding:"required,min=1,max=100"`
}

type UpdateMemberRequest struct {
	Generation    *int    `json:"generation,omitempty" binding:"omitempty,min=1"`
	Rank          *string `json:"rank,omitempty"`
	MedicalPrefix *string `json:"medicalPrefix,omitempty"`
	FullName      *string `json:"fullName,omitempty"`
	Department    *string `json:"department,omitempty"`
	Unit          *string `json:"unit,omitempty"`
}

type UpdateMemberStatusRequest struct {
	EmploymentStatus string `json:"employmentStatus" binding:"required" example:"ปลดราชการ"`
}

type Member struct {
	ID               string    `json:"id"`
	PoliceID         string    `json:"policeId"`
	Generation       int       `json:"generation"`
	Rank             string    `json:"rank,omitempty"`
	MedicalPrefix    string    `json:"medicalPrefix,omitempty"`
	FullName         string    `json:"fullName"`
	PenaltyCount     int       `json:"penaltyCount"`
	EmploymentStatus string    `json:"employmentStatus"`
	Department       string    `json:"department,omitempty"`
	Unit             string    `json:"unit,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type MemberList struct {
	Items               []Member            `json:"items"`
	GroupedByGeneration map[string][]Member `json:"groupedByGeneration"`
	Pagination          Pagination          `json:"pagination"`
}

type NameHistory struct {
	ID         string    `json:"id"`
	PoliceID   string    `json:"policeId"`
	OldName    string    `json:"oldName"`
	NewName    string    `json:"newName"`
	Generation int       `json:"generation"`
	ChangedAt  time.Time `json:"changedAt"`
	ChangedBy  string    `json:"changedBy"`
}

type MemberDetail struct {
	Member      Member        `json:"member"`
	NameHistory []NameHistory `json:"nameHistory"`
}
