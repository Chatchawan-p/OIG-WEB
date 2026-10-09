// Package authz provides the centralized OIG role policy.
package authz

import "github.com/oig-police/oig-web/internal/model"

type Action string

const (
	ReadMembers       Action = "members:read"
	WriteMembers      Action = "members:write"
	ReadPenalties     Action = "penalties:read"
	WritePenalties    Action = "penalties:write"
	ReadPenaltyTypes  Action = "penalty_types:read"
	WritePenaltyTypes Action = "penalty_types:write"
	ReadCatalog       Action = "catalog:read"
	WriteCatalog      Action = "catalog:write"
	ReadUsers         Action = "users:read"
	WriteUserRoles    Action = "users:roles:write"
	ReadAudit         Action = "audit:read"
	ReadMetadata      Action = "metadata:read"
	ReadEvidence      Action = "evidence:read"
)

type Policy struct {
	permissions map[model.Role]map[Action]struct{}
}

func New() *Policy {
	allOperational := []Action{ReadMembers, WriteMembers, ReadPenalties, WritePenalties, ReadPenaltyTypes, ReadCatalog, ReadUsers, ReadAudit, ReadMetadata, ReadEvidence}
	permissions := map[model.Role]map[Action]struct{}{
		model.RoleOwner:    {},
		model.RoleScreener: {},
		model.RoleAdmin:    {},
		model.RoleGuest:    {},
	}
	for _, action := range allOperational {
		permissions[model.RoleOwner][action] = struct{}{}
		permissions[model.RoleScreener][action] = struct{}{}
		permissions[model.RoleAdmin][action] = struct{}{}
	}
	for _, action := range []Action{WritePenaltyTypes, WriteCatalog, WriteUserRoles} {
		permissions[model.RoleOwner][action] = struct{}{}
	}
	permissions[model.RoleAdmin][WritePenaltyTypes] = struct{}{}
	permissions[model.RoleAdmin][WriteCatalog] = struct{}{}
	permissions[model.RoleScreener][WriteUserRoles] = struct{}{}
	return &Policy{permissions: permissions}
}

func (p *Policy) Can(role model.Role, action Action) bool {
	actions, ok := p.permissions[role]
	if !ok {
		return false
	}
	_, ok = actions[action]
	return ok
}

// CanChangeRole applies the stricter hierarchy used for role mutations.
// Owner is provisioned out of band and cannot be assigned through the API.
func (p *Policy) CanChangeRole(actor, target model.User, next model.Role) bool {
	if actor.UserID == target.UserID || next == model.RoleOwner || target.Role == model.RoleOwner {
		return false
	}
	switch actor.Role {
	case model.RoleOwner:
		return next == model.RoleScreener || next == model.RoleAdmin || next == model.RoleGuest
	case model.RoleScreener:
		return target.Role != model.RoleScreener && (next == model.RoleAdmin || next == model.RoleGuest)
	default:
		return false
	}
}
