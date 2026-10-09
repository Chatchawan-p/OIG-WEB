// Package repository defines persistence boundaries used by services.
package repository

import (
	"context"
	"time"

	"github.com/oig-police/oig-web/internal/model"
)

type Page struct {
	Number int
	Limit  int
}

type MemberFilter struct {
	Query            string
	EmploymentStatus string
	Discipline       string
	Department       string
	GenerationMin    *int
	GenerationMax    *int
	Page             Page
}

type AuditFilter struct{ Page Page }

type UserRepository interface {
	FindUserByID(context.Context, string) (model.User, error)
	FindUserByDiscordID(context.Context, string) (model.User, error)
	ListUsers(context.Context, Page) ([]model.User, int64, error)
	UpdateUserRoleWithAudit(context.Context, string, model.Role, time.Time, model.AuditLog) (model.User, error)
	UpdateUserDiscordIDWithAudit(context.Context, string, string, time.Time, model.AuditLog) (model.User, error)
	IncrementTokenVersion(context.Context, string, time.Time) error
	CreateUserWithAudit(context.Context, model.User, model.AuditLog) error
	CountUsersByRoles(context.Context, ...model.Role) (int64, error)
}

type MemberRepository interface {
	ListMembers(context.Context, MemberFilter) ([]model.PoliceMember, int64, error)
	FindMemberByID(context.Context, string) (model.PoliceMember, error)
	FindMemberByPoliceID(context.Context, string) (model.PoliceMember, error)
	CreateMemberWithAudit(context.Context, model.PoliceMember, model.AuditLog) error
	BulkCreateMembersWithAudit(context.Context, []model.PoliceMember, []model.AuditLog) error
	UpdateMemberWithHistoryAndAudit(context.Context, model.PoliceMember, *model.NameHistory, model.AuditLog) error
	UpdateMemberStatusWithAudit(context.Context, string, string, time.Time, model.AuditLog) (model.PoliceMember, error)
	SoftDeleteMemberWithAudit(context.Context, string, time.Time, model.AuditLog) error
	ListNameHistory(context.Context, string) ([]model.NameHistory, error)
}

type PenaltyRepository interface {
	ListPenaltyTypes(context.Context) ([]model.PenaltyType, error)
	FindPenaltyTypesByIdentifiers(context.Context, []string) ([]model.PenaltyType, error)
	CreatePenaltyTypeWithAudit(context.Context, model.PenaltyType, model.AuditLog) error
	DeletePenaltyTypeWithAudit(context.Context, string, model.AuditLog) error
	CreatePenaltyWithAudit(context.Context, model.Penalty, model.AuditLog, time.Time) (int, error)
	FindPenaltyByID(context.Context, string) (model.Penalty, error)
	FindPenaltyByEvidenceID(context.Context, string) (model.Penalty, error)
	ListMemberPenalties(context.Context, string) ([]model.Penalty, error)
	PardonPenaltyWithAudit(context.Context, string, string, string, time.Time, model.AuditLog) (model.Penalty, int, error)
	RecalculateMemberPenaltyCount(context.Context, string, time.Time) (int, error)
	RecalculateAllPenaltyCounts(context.Context, time.Time) error
}

type CatalogRepository interface {
	ListPenaltyRules(context.Context) ([]model.PenaltyRule, error)
	CreatePenaltyRuleWithAudit(context.Context, model.PenaltyRule, model.AuditLog) error
	UpdatePenaltyRuleWithAudit(context.Context, model.PenaltyRule, model.AuditLog) (model.PenaltyRule, error)
	DeletePenaltyRuleWithAudit(context.Context, string, model.AuditLog) error
}

type AuditRepository interface {
	ListAuditLogs(context.Context, AuditFilter) ([]model.AuditLog, int64, error)
}

type MetadataRepository interface {
	ListDepartments(context.Context) ([]string, error)
}

type Store interface {
	UserRepository
	MemberRepository
	PenaltyRepository
	CatalogRepository
	AuditRepository
	MetadataRepository
}
