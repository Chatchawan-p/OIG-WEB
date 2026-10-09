// Package memory provides an isolated repository implementation for unit and HTTP tests.
package memory

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/oig-police/oig-web/internal/apperror"
	"github.com/oig-police/oig-web/internal/model"
	"github.com/oig-police/oig-web/internal/repository"
)

type Store struct {
	mu        sync.RWMutex
	Users     map[string]model.User
	Members   map[string]model.PoliceMember
	Penalties map[string]model.Penalty
	Types     map[string]model.PenaltyType
	Rules     map[string]model.PenaltyRule
	History   []model.NameHistory
	Audits    []model.AuditLog
}

func New() *Store {
	return &Store{Users: map[string]model.User{}, Members: map[string]model.PoliceMember{}, Penalties: map[string]model.Penalty{}, Types: map[string]model.PenaltyType{}, Rules: map[string]model.PenaltyRule{}}
}

func (s *Store) SeedUsers(items ...model.User) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range items {
		s.Users[item.UserID] = item
	}
}

func (s *Store) SeedMembers(items ...model.PoliceMember) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range items {
		s.Members[item.MemberID] = item
	}
}

func (s *Store) SeedPenaltyTypes(items ...model.PenaltyType) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range items {
		s.Types[item.TypeID] = item
	}
}

func (s *Store) FindUserByID(_ context.Context, userID string) (model.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	user, ok := s.Users[userID]
	if !ok {
		return model.User{}, apperror.ErrNotFound
	}
	return user, nil
}

func (s *Store) FindUserByDiscordID(_ context.Context, discordID string) (model.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, user := range s.Users {
		if user.DiscordID == discordID {
			return user, nil
		}
	}
	return model.User{}, apperror.ErrNotFound
}

func (s *Store) ListUsers(_ context.Context, page repository.Page) ([]model.User, int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]model.User, 0, len(s.Users))
	for _, user := range s.Users {
		items = append(items, user)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Role != items[j].Role {
			return items[i].Role < items[j].Role
		}
		if items[i].DisplayName != items[j].DisplayName {
			return items[i].DisplayName < items[j].DisplayName
		}
		return items[i].UserID < items[j].UserID
	})
	return paginate(items, page), int64(len(items)), nil
}

func (s *Store) UpdateUserRoleWithAudit(_ context.Context, userID string, role model.Role, now time.Time, audit model.AuditLog) (model.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.Users[userID]
	if !ok {
		return model.User{}, apperror.ErrNotFound
	}
	user.Role, user.UpdatedAt, user.TokenVersion = role, now, user.TokenVersion+1
	s.Users[userID] = user
	s.Audits = append(s.Audits, audit)
	return user, nil
}

func (s *Store) IncrementTokenVersion(_ context.Context, userID string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.Users[userID]
	if !ok {
		return apperror.ErrNotFound
	}
	user.TokenVersion++
	user.UpdatedAt = now
	s.Users[userID] = user
	return nil
}

func (s *Store) CreateUserWithAudit(_ context.Context, user model.User, audit model.AuditLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.Users {
		if existing.UserID == user.UserID || existing.DiscordID == user.DiscordID {
			return apperror.ErrConflict
		}
	}
	s.Users[user.UserID] = user
	s.Audits = append(s.Audits, audit)
	return nil
}

func (s *Store) CountUsersByRoles(_ context.Context, roles ...model.Role) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(roles) == 0 {
		return int64(len(s.Users)), nil
	}
	roleSet := make(map[model.Role]bool, len(roles))
	for _, r := range roles {
		roleSet[r] = true
	}
	var count int64
	for _, user := range s.Users {
		if roleSet[user.Role] {
			count++
		}
	}
	return count, nil
}

func (s *Store) ListMembers(_ context.Context, filter repository.MemberFilter) ([]model.PoliceMember, int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	query := strings.ToLower(strings.TrimSpace(filter.Query))
	items := make([]model.PoliceMember, 0)
	for _, member := range s.Members {
		if member.DeletedAt != nil || (filter.EmploymentStatus != "" && member.EmploymentStatus != filter.EmploymentStatus) || (filter.Department != "" && member.Department != filter.Department) {
			continue
		}
		if filter.GenerationMin != nil && member.Generation < *filter.GenerationMin || filter.GenerationMax != nil && member.Generation > *filter.GenerationMax {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(strings.Join([]string{member.PoliceID, member.FullName, member.Rank, member.MedicalPrefix, member.Department, member.Unit}, " ")), query) {
			continue
		}
		if filter.Discipline != "" {
			matches := filter.Discipline == "0" && member.PenaltyCount == 0 || filter.Discipline == "1" && member.PenaltyCount == 1 || filter.Discipline == "2" && member.PenaltyCount == 2 || filter.Discipline == "3" && member.PenaltyCount >= 3
			if !matches {
				continue
			}
		}
		items = append(items, member)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Generation != items[j].Generation {
			return items[i].Generation < items[j].Generation
		}
		if items[i].PoliceID != items[j].PoliceID {
			return items[i].PoliceID < items[j].PoliceID
		}
		return items[i].MemberID < items[j].MemberID
	})
	return paginate(items, filter.Page), int64(len(items)), nil
}

func (s *Store) FindMemberByID(_ context.Context, memberID string) (model.PoliceMember, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	member, ok := s.Members[memberID]
	if !ok || member.DeletedAt != nil {
		return model.PoliceMember{}, apperror.ErrNotFound
	}
	return member, nil
}

func (s *Store) FindMemberByPoliceID(_ context.Context, policeID string) (model.PoliceMember, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, member := range s.Members {
		if member.PoliceID == policeID && member.DeletedAt == nil {
			return member, nil
		}
	}
	return model.PoliceMember{}, apperror.ErrNotFound
}

func (s *Store) policeIDExists(policeID, exceptMemberID string) bool {
	for _, member := range s.Members {
		if member.PoliceID == policeID && member.MemberID != exceptMemberID {
			return true
		}
	}
	return false
}

func (s *Store) CreateMemberWithAudit(_ context.Context, member model.PoliceMember, audit model.AuditLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.Members[member.MemberID]; exists || s.policeIDExists(member.PoliceID, "") {
		return apperror.ErrConflict
	}
	s.Members[member.MemberID] = member
	s.Audits = append(s.Audits, audit)
	return nil
}

func (s *Store) BulkCreateMembersWithAudit(_ context.Context, members []model.PoliceMember, audits []model.AuditLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := map[string]struct{}{}
	for _, member := range members {
		if _, exists := s.Members[member.MemberID]; exists || s.policeIDExists(member.PoliceID, "") {
			return apperror.ErrConflict
		}
		if _, duplicate := seen[member.PoliceID]; duplicate {
			return apperror.ErrConflict
		}
		seen[member.PoliceID] = struct{}{}
	}
	for _, member := range members {
		s.Members[member.MemberID] = member
	}
	s.Audits = append(s.Audits, audits...)
	return nil
}

func (s *Store) UpdateMemberWithHistoryAndAudit(_ context.Context, member model.PoliceMember, history *model.NameHistory, audit model.AuditLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.Members[member.MemberID]; !ok {
		return apperror.ErrNotFound
	}
	s.Members[member.MemberID] = member
	if history != nil {
		s.History = append(s.History, *history)
	}
	s.Audits = append(s.Audits, audit)
	return nil
}

func (s *Store) UpdateMemberStatusWithAudit(_ context.Context, memberID, status string, now time.Time, audit model.AuditLog) (model.PoliceMember, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	member, ok := s.Members[memberID]
	if !ok || member.DeletedAt != nil {
		return model.PoliceMember{}, apperror.ErrNotFound
	}
	member.EmploymentStatus, member.UpdatedAt = status, now
	s.Members[memberID] = member
	s.Audits = append(s.Audits, audit)
	return member, nil
}

func (s *Store) SoftDeleteMemberWithAudit(_ context.Context, memberID string, now time.Time, audit model.AuditLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	member, ok := s.Members[memberID]
	if !ok || member.DeletedAt != nil {
		return apperror.ErrNotFound
	}
	member.DeletedAt, member.UpdatedAt = &now, now
	s.Members[memberID] = member
	s.Audits = append(s.Audits, audit)
	return nil
}

func (s *Store) ListNameHistory(_ context.Context, memberID string) ([]model.NameHistory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]model.NameHistory, 0)
	for _, item := range s.History {
		if item.MemberID == memberID {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ChangedAt.After(items[j].ChangedAt) })
	return items, nil
}

func (s *Store) ListPenaltyTypes(_ context.Context) ([]model.PenaltyType, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]model.PenaltyType, 0, len(s.Types))
	for _, item := range s.Types {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}

func (s *Store) FindPenaltyTypesByIdentifiers(_ context.Context, identifiers []string) ([]model.PenaltyType, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := map[string]struct{}{}
	items := make([]model.PenaltyType, 0)
	for _, identifier := range identifiers {
		for _, item := range s.Types {
			if item.TypeID == identifier || item.LegacyID == identifier || item.Name == identifier {
				if _, ok := seen[item.TypeID]; !ok {
					seen[item.TypeID] = struct{}{}
					items = append(items, item)
				}
			}
		}
	}
	return items, nil
}

func (s *Store) CreatePenaltyTypeWithAudit(_ context.Context, item model.PenaltyType, audit model.AuditLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.Types {
		if existing.TypeID == item.TypeID || existing.Name == item.Name {
			return apperror.ErrConflict
		}
	}
	s.Types[item.TypeID] = item
	s.Audits = append(s.Audits, audit)
	return nil
}

func (s *Store) DeletePenaltyTypeWithAudit(_ context.Context, typeID string, audit model.AuditLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.Types[typeID]; !ok {
		return apperror.ErrNotFound
	}
	for _, penalty := range s.Penalties {
		for _, referenced := range penalty.PenaltyTypeIDs {
			if referenced == typeID {
				return apperror.ErrConflict
			}
		}
	}
	delete(s.Types, typeID)
	s.Audits = append(s.Audits, audit)
	return nil
}

func (s *Store) activeCount(memberID string, now time.Time) int {
	count := 0
	for _, penalty := range s.Penalties {
		if penalty.MemberID == memberID && penalty.IsActive(now) {
			count++
		}
	}
	return count
}

func (s *Store) CreatePenaltyWithAudit(_ context.Context, penalty model.Penalty, audit model.AuditLog, now time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.Penalties[penalty.PenaltyID]; ok {
		return 0, apperror.ErrConflict
	}
	member, ok := s.Members[penalty.MemberID]
	if !ok || member.DeletedAt != nil {
		return 0, apperror.ErrNotFound
	}
	s.Penalties[penalty.PenaltyID] = penalty
	count := s.activeCount(penalty.MemberID, now)
	member.PenaltyCount, member.UpdatedAt = count, now
	s.Members[member.MemberID] = member
	s.Audits = append(s.Audits, audit)
	return count, nil
}

func (s *Store) FindPenaltyByID(_ context.Context, penaltyID string) (model.Penalty, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	penalty, ok := s.Penalties[penaltyID]
	if !ok {
		return model.Penalty{}, apperror.ErrNotFound
	}
	return penalty, nil
}

func (s *Store) FindPenaltyByEvidenceID(_ context.Context, evidenceID string) (model.Penalty, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, penalty := range s.Penalties {
		if penalty.EvidenceID == evidenceID {
			return penalty, nil
		}
	}
	return model.Penalty{}, apperror.ErrNotFound
}

func (s *Store) ListMemberPenalties(_ context.Context, memberID string) ([]model.Penalty, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]model.Penalty, 0)
	for _, penalty := range s.Penalties {
		if penalty.MemberID == memberID {
			items = append(items, penalty)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, nil
}

func (s *Store) PardonPenaltyWithAudit(_ context.Context, penaltyID, actorID, reason string, now time.Time, audit model.AuditLog) (model.Penalty, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	penalty, ok := s.Penalties[penaltyID]
	if !ok {
		return model.Penalty{}, 0, apperror.ErrNotFound
	}
	if penalty.PardonedAt != nil {
		return model.Penalty{}, 0, apperror.ErrConflict
	}
	penalty.PardonedAt, penalty.PardonedBy, penalty.PardonReason = &now, actorID, reason
	s.Penalties[penaltyID] = penalty
	count := s.activeCount(penalty.MemberID, now)
	member := s.Members[penalty.MemberID]
	member.PenaltyCount, member.UpdatedAt = count, now
	s.Members[member.MemberID] = member
	s.Audits = append(s.Audits, audit)
	return penalty, count, nil
}

func (s *Store) RecalculateMemberPenaltyCount(_ context.Context, memberID string, now time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	member, ok := s.Members[memberID]
	if !ok || member.DeletedAt != nil {
		return 0, apperror.ErrNotFound
	}
	count := s.activeCount(memberID, now)
	member.PenaltyCount = count
	s.Members[memberID] = member
	return count, nil
}

func (s *Store) RecalculateAllPenaltyCounts(_ context.Context, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, member := range s.Members {
		if member.DeletedAt == nil {
			member.PenaltyCount = s.activeCount(id, now)
			s.Members[id] = member
		}
	}
	return nil
}

func (s *Store) ListPenaltyRules(_ context.Context) ([]model.PenaltyRule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]model.PenaltyRule, 0, len(s.Rules))
	for _, item := range s.Rules {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Category != items[j].Category {
			return items[i].Category < items[j].Category
		}
		return items[i].Name < items[j].Name
	})
	return items, nil
}

func (s *Store) CreatePenaltyRuleWithAudit(_ context.Context, rule model.PenaltyRule, audit model.AuditLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.Rules[rule.RuleID]; ok {
		return apperror.ErrConflict
	}
	s.Rules[rule.RuleID] = rule
	s.Audits = append(s.Audits, audit)
	return nil
}

func (s *Store) UpdatePenaltyRuleWithAudit(_ context.Context, rule model.PenaltyRule, audit model.AuditLog) (model.PenaltyRule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.Rules[rule.RuleID]
	if !ok {
		return model.PenaltyRule{}, apperror.ErrNotFound
	}
	rule.CreatedAt = existing.CreatedAt
	s.Rules[rule.RuleID] = rule
	s.Audits = append(s.Audits, audit)
	return rule, nil
}

func (s *Store) DeletePenaltyRuleWithAudit(_ context.Context, ruleID string, audit model.AuditLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.Rules[ruleID]; !ok {
		return apperror.ErrNotFound
	}
	delete(s.Rules, ruleID)
	s.Audits = append(s.Audits, audit)
	return nil
}

func (s *Store) ListAuditLogs(_ context.Context, filter repository.AuditFilter) ([]model.AuditLog, int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := append([]model.AuditLog(nil), s.Audits...)
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return paginate(items, filter.Page), int64(len(items)), nil
}

func (s *Store) ListDepartments(_ context.Context) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := map[string]struct{}{}
	for _, member := range s.Members {
		if member.DeletedAt == nil && strings.TrimSpace(member.Department) != "" {
			seen[member.Department] = struct{}{}
		}
	}
	items := make([]string, 0, len(seen))
	for item := range seen {
		items = append(items, item)
	}
	sort.Strings(items)
	return items, nil
}

func paginate[T any](items []T, page repository.Page) []T {
	start := (page.Number - 1) * page.Limit
	if start < 0 || start >= len(items) {
		return []T{}
	}
	end := start + page.Limit
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

var _ repository.Store = (*Store)(nil)
