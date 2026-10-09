package dto

import (
	"time"

	"github.com/oig-police/oig-web/internal/model"
)

func MemberFromModel(item model.PoliceMember) Member {
	return Member{ID: item.MemberID, PoliceID: item.PoliceID, Generation: item.Generation, Rank: item.Rank, MedicalPrefix: item.MedicalPrefix, FullName: item.FullName, PenaltyCount: item.PenaltyCount, EmploymentStatus: item.EmploymentStatus, Department: item.Department, Unit: item.Unit, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt}
}

func NameHistoryFromModel(item model.NameHistory) NameHistory {
	return NameHistory{ID: item.HistoryID, PoliceID: item.PoliceID, OldName: item.OldName, NewName: item.NewName, Generation: item.Generation, ChangedAt: item.ChangedAt, ChangedBy: item.ChangedBy}
}

func PenaltyFromModel(item model.Penalty, now time.Time) Penalty {
	evidenceURL := ""
	if item.EvidenceID != "" {
		evidenceURL = "/api/v1/evidence/" + item.EvidenceID
	}
	return Penalty{ID: item.PenaltyID, MemberID: item.MemberID, PoliceID: item.PoliceID, ActionTypeIDs: item.PenaltyTypeIDs, ActionTypes: item.PenaltyTypeNames, Reason: item.Reason, DurationType: string(item.DurationType), Months: item.DurationMonths, StartDate: item.StartsAt, ExpiryDate: item.ExpiresAt, FineAmount: item.FineAmount, EvidenceID: item.EvidenceID, EvidenceURL: evidenceURL, CreatedBy: item.CreatedBy, CreatedAt: item.CreatedAt, PardonedAt: item.PardonedAt, PardonedBy: item.PardonedBy, PardonReason: item.PardonReason, IsActive: item.IsActive(now), IsPermanent: item.IsPermanent()}
}

func PenaltyTypeFromModel(item model.PenaltyType) PenaltyType {
	return PenaltyType{ID: item.TypeID, LegacyID: item.LegacyID, Name: item.Name}
}

func RuleFromModel(item model.PenaltyRule) PenaltyRule {
	return PenaltyRule{ID: item.RuleID, Name: item.Name, Category: item.Category, Fine: item.Fine, Jail: item.Jail, Description: item.Description, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt}
}

func UserFromModel(item model.User) User {
	return User{ID: item.UserID, DiscordID: item.DiscordID, Name: item.DisplayName, Avatar: item.AvatarURL, Role: string(item.Role), CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt}
}

func AuthUserFromModel(item model.User) AuthUser {
	return AuthUser{ID: item.UserID, DiscordID: item.DiscordID, Name: item.DisplayName, Avatar: item.AvatarURL, Role: string(item.Role), IsAuthorized: item.Role != model.RoleGuest}
}

func AuditFromModel(item model.AuditLog) AuditLog {
	return AuditLog{ID: item.AuditID, ActorID: item.ActorID, Action: item.Action, ResourceType: item.ResourceType, ResourceID: item.ResourceID, Description: item.Description, CreatedAt: item.CreatedAt}
}
