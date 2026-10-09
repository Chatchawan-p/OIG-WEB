package service

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/oig-police/oig-web/internal/apperror"
	"github.com/oig-police/oig-web/internal/authz"
	"github.com/oig-police/oig-web/internal/evidence"
	"github.com/oig-police/oig-web/internal/model"
	"github.com/oig-police/oig-web/internal/platform/id"
	"github.com/oig-police/oig-web/internal/repository"
)

type PenaltyCommand struct {
	PoliceID      string
	ActionTypeIDs []string
	Reason        string
	DurationType  string
	Months        *int
	StartDate     string
	EndDate       string
	FineAmount    *int64
	EvidenceName  string
	Evidence      io.Reader
}

type PenaltyCreation struct {
	Penalty      model.Penalty
	PenaltyCount int
}

type PenaltyPardon struct {
	Penalty      model.Penalty
	PenaltyCount int
}

type PenaltyService struct {
	repo     penaltyStore
	evidence evidence.Storage
	policy   *authz.Policy
	now      Clock
	bangkok  *time.Location
}

type penaltyStore interface {
	repository.PenaltyRepository
	repository.MemberRepository
}

func NewPenaltyService(repo penaltyStore, evidenceStore evidence.Storage, policy *authz.Policy, now Clock) *PenaltyService {
	location, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		location = time.FixedZone("Asia/Bangkok", 7*60*60)
	}
	return &PenaltyService{repo: repo, evidence: evidenceStore, policy: policy, now: now, bangkok: location}
}

func (s *PenaltyService) ListTypes(ctx context.Context, actor model.User) ([]model.PenaltyType, error) {
	if !s.policy.Can(actor.Role, authz.ReadPenaltyTypes) {
		return nil, apperror.ErrForbidden
	}
	return s.repo.ListPenaltyTypes(ctx)
}

func (s *PenaltyService) CreateType(ctx context.Context, actor model.User, name, legacyID string) (model.PenaltyType, error) {
	if !s.policy.Can(actor.Role, authz.WritePenaltyTypes) {
		return model.PenaltyType{}, apperror.ErrForbidden
	}
	name = strings.TrimSpace(name)
	legacyID = strings.TrimSpace(legacyID)
	validation := &apperror.ValidationError{}
	if name == "" || len([]rune(name)) > 200 {
		validation.Add("name", "is required and must not exceed 200 characters")
	}
	if len([]rune(legacyID)) > 100 {
		validation.Add("legacyId", "must not exceed 100 characters")
	}
	if err := validation.OrNil(); err != nil {
		return model.PenaltyType{}, err
	}
	typeID, err := id.New("pty")
	if err != nil {
		return model.PenaltyType{}, err
	}
	now := s.now()
	penaltyType := model.PenaltyType{TypeID: typeID, LegacyID: legacyID, Name: name, CreatedAt: now, CreatedBy: actor.UserID}
	audit, err := newAudit(actor, "penalty_type.created", "penalty_type", typeID, "created penalty type "+name, now)
	if err != nil {
		return model.PenaltyType{}, err
	}
	if err := s.repo.CreatePenaltyTypeWithAudit(ctx, penaltyType, audit); err != nil {
		return model.PenaltyType{}, err
	}
	return penaltyType, nil
}

func (s *PenaltyService) DeleteType(ctx context.Context, actor model.User, typeID string) error {
	if !s.policy.Can(actor.Role, authz.WritePenaltyTypes) {
		return apperror.ErrForbidden
	}
	now := s.now()
	audit, err := newAudit(actor, "penalty_type.deleted", "penalty_type", typeID, "deleted unreferenced penalty type", now)
	if err != nil {
		return err
	}
	return s.repo.DeletePenaltyTypeWithAudit(ctx, typeID, audit)
}

func normalizeDuration(value string) model.DurationType {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "permanent", "ถาวร":
		return model.DurationPermanent
	case "months", "ระบุเดือน":
		return model.DurationMonths
	case "date_range", "ระบุวัน":
		return model.DurationDateRange
	default:
		return ""
	}
}

func (s *PenaltyService) parseDate(field, value string, required bool, validation *apperror.ValidationError) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		if required {
			validation.Add(field, "is required")
		}
		return nil
	}
	parsed, err := time.ParseInLocation("2006-01-02", value, s.bangkok)
	if err != nil {
		validation.Add(field, "must use YYYY-MM-DD")
		return nil
	}
	utc := parsed.UTC()
	return &utc
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func (s *PenaltyService) Create(ctx context.Context, actor model.User, command PenaltyCommand) (PenaltyCreation, error) {
	if !s.policy.Can(actor.Role, authz.WritePenalties) {
		return PenaltyCreation{}, apperror.ErrForbidden
	}
	validation := &apperror.ValidationError{}
	command.PoliceID = strings.TrimSpace(command.PoliceID)
	if command.PoliceID == "" {
		validation.Add("policeId", "is required")
	}
	command.ActionTypeIDs = uniqueStrings(command.ActionTypeIDs)
	if len(command.ActionTypeIDs) == 0 || len(command.ActionTypeIDs) > 20 {
		validation.Add("actionTypeIds", "must contain between 1 and 20 unique IDs")
	}
	command.Reason = strings.TrimSpace(command.Reason)
	if command.Reason == "" || len([]rune(command.Reason)) > 2000 {
		validation.Add("reason", "is required and must not exceed 2000 characters")
	}
	duration := normalizeDuration(command.DurationType)
	if duration == "" {
		validation.Add("durationType", "must be permanent, months, or date_range")
	}
	if command.FineAmount != nil && *command.FineAmount < 0 {
		validation.Add("fineAmount", "must not be negative")
	}
	if command.Evidence == nil {
		validation.Add("evidence", "image is required")
	}

	now := s.now()
	localNow := now.In(s.bangkok)
	localToday := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, s.bangkok)
	startsAt := s.parseDate("startDate", command.StartDate, duration == model.DurationDateRange, validation)
	if startsAt == nil && duration != model.DurationDateRange {
		utc := localToday.UTC()
		startsAt = &utc
	}
	var expiresAt *time.Time
	switch duration {
	case model.DurationPermanent:
		if command.Months != nil || strings.TrimSpace(command.EndDate) != "" {
			validation.Add("durationType", "permanent penalties cannot include months or endDate")
		}
	case model.DurationMonths:
		if command.Months == nil || *command.Months < 1 || *command.Months > 1200 {
			validation.Add("months", "must be between 1 and 1200")
		} else if startsAt != nil {
			localStart := startsAt.In(s.bangkok)
			expires := localStart.AddDate(0, *command.Months, 0).UTC()
			expiresAt = &expires
		}
	case model.DurationDateRange:
		end := s.parseDate("endDate", command.EndDate, true, validation)
		if end != nil {
			// Legacy date input is treated as inclusive in Asia/Bangkok; store the
			// exclusive boundary at midnight on the following local day.
			exclusive := end.In(s.bangkok).AddDate(0, 0, 1).UTC()
			expiresAt = &exclusive
			if startsAt != nil && !expiresAt.After(*startsAt) {
				validation.Add("endDate", "must not be before startDate")
			}
		}
	}
	if err := validation.OrNil(); err != nil {
		return PenaltyCreation{}, err
	}

	member, err := s.repo.FindMemberByPoliceID(ctx, command.PoliceID)
	if err != nil {
		return PenaltyCreation{}, err
	}
	types, err := s.repo.FindPenaltyTypesByIdentifiers(ctx, command.ActionTypeIDs)
	if err != nil {
		return PenaltyCreation{}, err
	}
	if len(types) != len(command.ActionTypeIDs) {
		return PenaltyCreation{}, &apperror.ValidationError{Details: []apperror.FieldError{{Field: "actionTypeIds", Message: "contains an unknown penalty type"}}}
	}
	resolved := make(map[string]model.PenaltyType, len(types)*3)
	for _, penaltyType := range types {
		resolved[penaltyType.TypeID] = penaltyType
		resolved[penaltyType.Name] = penaltyType
		if penaltyType.LegacyID != "" {
			resolved[penaltyType.LegacyID] = penaltyType
		}
	}
	resolvedIDs := make([]string, 0, len(command.ActionTypeIDs))
	orderedNames := make([]string, 0, len(command.ActionTypeIDs))
	seenResolved := make(map[string]struct{}, len(command.ActionTypeIDs))
	for _, identifier := range command.ActionTypeIDs {
		penaltyType, ok := resolved[identifier]
		if !ok {
			return PenaltyCreation{}, &apperror.ValidationError{Details: []apperror.FieldError{{Field: "actionTypeIds", Message: "contains an unknown penalty type"}}}
		}
		if _, duplicate := seenResolved[penaltyType.TypeID]; duplicate {
			continue
		}
		seenResolved[penaltyType.TypeID] = struct{}{}
		resolvedIDs = append(resolvedIDs, penaltyType.TypeID)
		orderedNames = append(orderedNames, penaltyType.Name)
	}

	stored, err := s.evidence.Save(ctx, command.EvidenceName, command.Evidence)
	if err != nil {
		return PenaltyCreation{}, err
	}
	cleanupEvidence := true
	defer func() {
		if cleanupEvidence {
			_ = s.evidence.Delete(context.Background(), stored.Path)
		}
	}()
	penaltyID, err := id.New("pen")
	if err != nil {
		return PenaltyCreation{}, err
	}
	penalty := model.Penalty{
		PenaltyID: penaltyID, MemberID: member.MemberID, PoliceID: member.PoliceID,
		PenaltyTypeIDs: resolvedIDs, PenaltyTypeNames: orderedNames, Reason: command.Reason,
		DurationType: duration, DurationMonths: command.Months, StartsAt: startsAt, ExpiresAt: expiresAt,
		EvidenceID: stored.ID, EvidencePath: stored.Path, EvidenceType: stored.ContentType, EvidenceName: stored.OriginalName,
		FineAmount: command.FineAmount, CreatedBy: actor.UserID, CreatedAt: now,
	}
	audit, err := newAudit(actor, "penalty.created", "penalty", penaltyID, fmt.Sprintf("created penalty for police ID %s", member.PoliceID), now)
	if err != nil {
		return PenaltyCreation{}, err
	}
	count, err := s.repo.CreatePenaltyWithAudit(ctx, penalty, audit, now)
	if err != nil {
		return PenaltyCreation{}, err
	}
	cleanupEvidence = false
	return PenaltyCreation{Penalty: penalty, PenaltyCount: count}, nil
}

func (s *PenaltyService) Get(ctx context.Context, actor model.User, penaltyID string) (model.Penalty, error) {
	if !s.policy.Can(actor.Role, authz.ReadPenalties) {
		return model.Penalty{}, apperror.ErrForbidden
	}
	return s.repo.FindPenaltyByID(ctx, penaltyID)
}

func (s *PenaltyService) ListForMember(ctx context.Context, actor model.User, memberID string) ([]model.Penalty, error) {
	if !s.policy.Can(actor.Role, authz.ReadPenalties) {
		return nil, apperror.ErrForbidden
	}
	return s.repo.ListMemberPenalties(ctx, memberID)
}

func (s *PenaltyService) Pardon(ctx context.Context, actor model.User, penaltyID, reason string) (PenaltyPardon, error) {
	if !s.policy.Can(actor.Role, authz.WritePenalties) {
		return PenaltyPardon{}, apperror.ErrForbidden
	}
	reason = strings.TrimSpace(reason)
	if reason == "" || len([]rune(reason)) > 1000 {
		return PenaltyPardon{}, &apperror.ValidationError{Details: []apperror.FieldError{{Field: "reason", Message: "is required and must not exceed 1000 characters"}}}
	}
	now := s.now()
	audit, err := newAudit(actor, "penalty.pardoned", "penalty", penaltyID, "pardoned penalty: "+reason, now)
	if err != nil {
		return PenaltyPardon{}, err
	}
	penalty, count, err := s.repo.PardonPenaltyWithAudit(ctx, penaltyID, actor.UserID, reason, now, audit)
	return PenaltyPardon{Penalty: penalty, PenaltyCount: count}, err
}

func (s *PenaltyService) Summary(ctx context.Context, actor model.User, policeID string) (model.PoliceMember, []model.Penalty, error) {
	if !s.policy.Can(actor.Role, authz.ReadPenalties) || !s.policy.Can(actor.Role, authz.ReadMembers) {
		return model.PoliceMember{}, nil, apperror.ErrForbidden
	}
	member, err := s.repo.FindMemberByPoliceID(ctx, strings.TrimSpace(policeID))
	if err != nil {
		return model.PoliceMember{}, nil, err
	}
	count, err := s.repo.RecalculateMemberPenaltyCount(ctx, member.MemberID, s.now())
	if err != nil {
		return model.PoliceMember{}, nil, err
	}
	member.PenaltyCount = count
	penalties, err := s.repo.ListMemberPenalties(ctx, member.MemberID)
	return member, penalties, err
}

func (s *PenaltyService) Evidence(ctx context.Context, actor model.User, evidenceID string) (io.ReadCloser, string, string, error) {
	if !s.policy.Can(actor.Role, authz.ReadEvidence) {
		return nil, "", "", apperror.ErrForbidden
	}
	penalty, err := s.repo.FindPenaltyByEvidenceID(ctx, evidenceID)
	if err != nil {
		return nil, "", "", err
	}
	reader, err := s.evidence.Open(ctx, penalty.EvidencePath)
	if err != nil {
		return nil, "", "", err
	}
	return reader, penalty.EvidenceType, penalty.EvidenceName, nil
}

func (s *PenaltyService) ActiveCount(penalties []model.Penalty, at time.Time) int {
	count := 0
	for _, penalty := range penalties {
		if penalty.IsActive(at) {
			count++
		}
	}
	return count
}
