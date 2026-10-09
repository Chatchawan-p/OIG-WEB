// Package mongo implements repositories with the official MongoDB driver.
package mongo

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/oig-police/oig-web/internal/apperror"
	database "github.com/oig-police/oig-web/internal/database/mongodb"
	"github.com/oig-police/oig-web/internal/model"
	"github.com/oig-police/oig-web/internal/repository"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Store struct{ db *database.Database }

func New(db *database.Database) *Store { return &Store{db: db} }

func (s *Store) users() *mongo.Collection        { return s.db.Collection("users") }
func (s *Store) members() *mongo.Collection      { return s.db.Collection("police_members") }
func (s *Store) history() *mongo.Collection      { return s.db.Collection("name_history") }
func (s *Store) penalties() *mongo.Collection    { return s.db.Collection("penalties") }
func (s *Store) penaltyTypes() *mongo.Collection { return s.db.Collection("penalty_types") }
func (s *Store) catalog() *mongo.Collection      { return s.db.Collection("penalty_catalog") }
func (s *Store) audits() *mongo.Collection       { return s.db.Collection("audit_logs") }

func mapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, mongo.ErrNoDocuments) {
		return apperror.ErrNotFound
	}
	if mongo.IsDuplicateKeyError(err) {
		return apperror.ErrConflict
	}
	return err
}

func (s *Store) FindUserByID(ctx context.Context, userID string) (model.User, error) {
	var user model.User
	err := s.users().FindOne(ctx, bson.M{"user_id": userID}).Decode(&user)
	return user, mapError(err)
}

func (s *Store) FindUserByDiscordID(ctx context.Context, discordID string) (model.User, error) {
	var user model.User
	err := s.users().FindOne(ctx, bson.M{"discord_id": discordID}).Decode(&user)
	return user, mapError(err)
}

func (s *Store) ListUsers(ctx context.Context, page repository.Page) ([]model.User, int64, error) {
	total, err := s.users().CountDocuments(ctx, bson.M{})
	if err != nil {
		return nil, 0, err
	}
	cursor, err := s.users().Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "role", Value: 1}, {Key: "display_name", Value: 1}, {Key: "user_id", Value: 1}}).SetSkip(int64((page.Number-1)*page.Limit)).SetLimit(int64(page.Limit)))
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)
	var users []model.User
	if err := cursor.All(ctx, &users); err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

func (s *Store) UpdateUserRoleWithAudit(ctx context.Context, userID string, role model.Role, now time.Time, audit model.AuditLog) (model.User, error) {
	var updated model.User
	err := s.db.WithTransaction(ctx, func(tx context.Context) error {
		err := s.users().FindOneAndUpdate(tx, bson.M{"user_id": userID}, bson.M{"$set": bson.M{"role": role, "updated_at": now}, "$inc": bson.M{"token_version": 1}}, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&updated)
		if err != nil {
			return mapError(err)
		}
		_, err = s.audits().InsertOne(tx, audit)
		return mapError(err)
	})
	return updated, err
}

func (s *Store) UpdateUserDiscordIDWithAudit(ctx context.Context, userID, newDiscordID string, now time.Time, audit model.AuditLog) (model.User, error) {
	var updated model.User
	err := s.db.WithTransaction(ctx, func(tx context.Context) error {
		err := s.users().FindOneAndUpdate(tx, bson.M{"user_id": userID}, bson.M{"$set": bson.M{"discord_id": newDiscordID, "updated_at": now}, "$inc": bson.M{"token_version": 1}}, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&updated)
		if err != nil {
			return mapError(err)
		}
		_, err = s.audits().InsertOne(tx, audit)
		return mapError(err)
	})
	return updated, err
}

func (s *Store) IncrementTokenVersion(ctx context.Context, userID string, now time.Time) error {
	result, err := s.users().UpdateOne(ctx, bson.M{"user_id": userID}, bson.M{"$inc": bson.M{"token_version": 1}, "$set": bson.M{"updated_at": now}})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return apperror.ErrNotFound
	}
	return nil
}

func (s *Store) CreateUserWithAudit(ctx context.Context, user model.User, audit model.AuditLog) error {
	return s.db.WithTransaction(ctx, func(tx context.Context) error {
		if _, err := s.users().InsertOne(tx, user); err != nil {
			return mapError(err)
		}
		_, err := s.audits().InsertOne(tx, audit)
		return mapError(err)
	})
}

func (s *Store) CountUsersByRoles(ctx context.Context, roles ...model.Role) (int64, error) {
	filter := bson.M{}
	if len(roles) > 0 {
		filter["role"] = bson.M{"$in": roles}
	}
	count, err := s.users().CountDocuments(ctx, filter)
	if err != nil {
		return 0, mapError(err)
	}
	return count, nil
}

func memberQuery(filter repository.MemberFilter) bson.M {
	query := bson.M{"deleted_at": nil}
	if filter.Query != "" {
		pattern := primitiveRegex(filter.Query)
		query["$or"] = bson.A{
			bson.M{"police_id": pattern}, bson.M{"full_name": pattern}, bson.M{"rank": pattern},
			bson.M{"medical_prefix": pattern}, bson.M{"department": pattern}, bson.M{"unit": pattern},
		}
	}
	if filter.EmploymentStatus != "" {
		query["employment_status"] = filter.EmploymentStatus
	}
	if filter.Department != "" {
		query["department"] = filter.Department
	}
	if filter.Discipline != "" {
		switch filter.Discipline {
		case "0", "1", "2":
			query["penalty_count"] = map[string]int{"0": 0, "1": 1, "2": 2}[filter.Discipline]
		case "3":
			query["penalty_count"] = bson.M{"$gte": 3}
		}
	}
	if filter.GenerationMin != nil || filter.GenerationMax != nil {
		rangeQuery := bson.M{}
		if filter.GenerationMin != nil {
			rangeQuery["$gte"] = *filter.GenerationMin
		}
		if filter.GenerationMax != nil {
			rangeQuery["$lte"] = *filter.GenerationMax
		}
		query["generation"] = rangeQuery
	}
	return query
}

func primitiveRegex(value string) primitive.Regex {
	return primitive.Regex{Pattern: regexp.QuoteMeta(strings.TrimSpace(value)), Options: "i"}
}

func (s *Store) ListMembers(ctx context.Context, filter repository.MemberFilter) ([]model.PoliceMember, int64, error) {
	query := memberQuery(filter)
	total, err := s.members().CountDocuments(ctx, query)
	if err != nil {
		return nil, 0, err
	}
	findOptions := options.Find().SetSort(bson.D{{Key: "generation", Value: 1}, {Key: "police_id", Value: 1}, {Key: "member_id", Value: 1}}).SetSkip(int64((filter.Page.Number - 1) * filter.Page.Limit)).SetLimit(int64(filter.Page.Limit))
	cursor, err := s.members().Find(ctx, query, findOptions)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)
	var members []model.PoliceMember
	if err := cursor.All(ctx, &members); err != nil {
		return nil, 0, err
	}
	return members, total, nil
}

func (s *Store) FindMemberByID(ctx context.Context, memberID string) (model.PoliceMember, error) {
	var member model.PoliceMember
	err := s.members().FindOne(ctx, bson.M{"member_id": memberID, "deleted_at": nil}).Decode(&member)
	return member, mapError(err)
}

func (s *Store) FindMemberByPoliceID(ctx context.Context, policeID string) (model.PoliceMember, error) {
	var member model.PoliceMember
	err := s.members().FindOne(ctx, bson.M{"police_id": policeID, "deleted_at": nil}).Decode(&member)
	return member, mapError(err)
}

func (s *Store) CreateMemberWithAudit(ctx context.Context, member model.PoliceMember, audit model.AuditLog) error {
	return s.db.WithTransaction(ctx, func(tx context.Context) error {
		if _, err := s.members().InsertOne(tx, member); err != nil {
			return mapError(err)
		}
		_, err := s.audits().InsertOne(tx, audit)
		return mapError(err)
	})
}

func (s *Store) BulkCreateMembersWithAudit(ctx context.Context, members []model.PoliceMember, audits []model.AuditLog) error {
	return s.db.WithTransaction(ctx, func(tx context.Context) error {
		memberDocs := make([]any, len(members))
		for i := range members {
			memberDocs[i] = members[i]
		}
		if _, err := s.members().InsertMany(tx, memberDocs, options.InsertMany().SetOrdered(true)); err != nil {
			return mapError(err)
		}
		auditDocs := make([]any, len(audits))
		for i := range audits {
			auditDocs[i] = audits[i]
		}
		_, err := s.audits().InsertMany(tx, auditDocs, options.InsertMany().SetOrdered(true))
		return mapError(err)
	})
}

func (s *Store) UpdateMemberWithHistoryAndAudit(ctx context.Context, member model.PoliceMember, history *model.NameHistory, audit model.AuditLog) error {
	return s.db.WithTransaction(ctx, func(tx context.Context) error {
		result, err := s.members().UpdateOne(tx, bson.M{"member_id": member.MemberID, "deleted_at": nil}, bson.M{"$set": bson.M{
			"generation": member.Generation, "rank": member.Rank, "medical_prefix": member.MedicalPrefix,
			"full_name": member.FullName, "department": member.Department, "unit": member.Unit, "updated_at": member.UpdatedAt,
		}})
		if err != nil {
			return mapError(err)
		}
		if result.MatchedCount == 0 {
			return apperror.ErrNotFound
		}
		if history != nil {
			if _, err := s.history().InsertOne(tx, *history); err != nil {
				return mapError(err)
			}
		}
		_, err = s.audits().InsertOne(tx, audit)
		return mapError(err)
	})
}

func (s *Store) UpdateMemberStatusWithAudit(ctx context.Context, memberID, status string, now time.Time, audit model.AuditLog) (model.PoliceMember, error) {
	var updated model.PoliceMember
	err := s.db.WithTransaction(ctx, func(tx context.Context) error {
		err := s.members().FindOneAndUpdate(tx, bson.M{"member_id": memberID, "deleted_at": nil}, bson.M{"$set": bson.M{"employment_status": status, "updated_at": now}}, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&updated)
		if err != nil {
			return mapError(err)
		}
		_, err = s.audits().InsertOne(tx, audit)
		return mapError(err)
	})
	return updated, err
}

func (s *Store) SoftDeleteMemberWithAudit(ctx context.Context, memberID string, now time.Time, audit model.AuditLog) error {
	return s.db.WithTransaction(ctx, func(tx context.Context) error {
		result, err := s.members().UpdateOne(tx, bson.M{"member_id": memberID, "deleted_at": nil}, bson.M{"$set": bson.M{"deleted_at": now, "updated_at": now}})
		if err != nil {
			return err
		}
		if result.MatchedCount == 0 {
			return apperror.ErrNotFound
		}
		_, err = s.audits().InsertOne(tx, audit)
		return mapError(err)
	})
}

func (s *Store) ListNameHistory(ctx context.Context, memberID string) ([]model.NameHistory, error) {
	cursor, err := s.history().Find(ctx, bson.M{"member_id": memberID}, options.Find().SetSort(bson.D{{Key: "changed_at", Value: -1}, {Key: "history_id", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var history []model.NameHistory
	if err := cursor.All(ctx, &history); err != nil {
		return nil, err
	}
	return history, nil
}

func (s *Store) ListPenaltyTypes(ctx context.Context) ([]model.PenaltyType, error) {
	cursor, err := s.penaltyTypes().Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "name", Value: 1}, {Key: "type_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var types []model.PenaltyType
	if err := cursor.All(ctx, &types); err != nil {
		return nil, err
	}
	return types, nil
}

func (s *Store) FindPenaltyTypesByIdentifiers(ctx context.Context, identifiers []string) ([]model.PenaltyType, error) {
	cursor, err := s.penaltyTypes().Find(ctx, bson.M{"$or": bson.A{
		bson.M{"type_id": bson.M{"$in": identifiers}}, bson.M{"legacy_id": bson.M{"$in": identifiers}}, bson.M{"name": bson.M{"$in": identifiers}},
	}})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var types []model.PenaltyType
	if err := cursor.All(ctx, &types); err != nil {
		return nil, err
	}
	return types, nil
}

func (s *Store) CreatePenaltyTypeWithAudit(ctx context.Context, penaltyType model.PenaltyType, audit model.AuditLog) error {
	return s.db.WithTransaction(ctx, func(tx context.Context) error {
		if _, err := s.penaltyTypes().InsertOne(tx, penaltyType); err != nil {
			return mapError(err)
		}
		_, err := s.audits().InsertOne(tx, audit)
		return mapError(err)
	})
}

func (s *Store) DeletePenaltyTypeWithAudit(ctx context.Context, typeID string, audit model.AuditLog) error {
	return s.db.WithTransaction(ctx, func(tx context.Context) error {
		used, err := s.penalties().CountDocuments(tx, bson.M{"penalty_type_ids": typeID})
		if err != nil {
			return err
		}
		if used > 0 {
			return apperror.ErrConflict
		}
		result, err := s.penaltyTypes().DeleteOne(tx, bson.M{"type_id": typeID})
		if err != nil {
			return err
		}
		if result.DeletedCount == 0 {
			return apperror.ErrNotFound
		}
		_, err = s.audits().InsertOne(tx, audit)
		return mapError(err)
	})
}

func activePenaltyQuery(memberID string, now time.Time) bson.M {
	return bson.M{
		"member_id":   memberID,
		"pardoned_at": nil,
		"$and": bson.A{
			bson.M{"$or": bson.A{bson.M{"starts_at": nil}, bson.M{"starts_at": bson.M{"$lte": now}}}},
			bson.M{"$or": bson.A{bson.M{"duration_type": model.DurationPermanent}, bson.M{"expires_at": bson.M{"$gt": now}}}},
		},
	}
}

func (s *Store) CreatePenaltyWithAudit(ctx context.Context, penalty model.Penalty, audit model.AuditLog, now time.Time) (int, error) {
	count := 0
	err := s.db.WithTransaction(ctx, func(tx context.Context) error {
		if _, err := s.penalties().InsertOne(tx, penalty); err != nil {
			return mapError(err)
		}
		active, err := s.penalties().CountDocuments(tx, activePenaltyQuery(penalty.MemberID, now))
		if err != nil {
			return err
		}
		count = int(active)
		result, err := s.members().UpdateOne(tx, bson.M{"member_id": penalty.MemberID, "deleted_at": nil}, bson.M{"$set": bson.M{"penalty_count": count, "updated_at": now}})
		if err != nil {
			return err
		}
		if result.MatchedCount == 0 {
			return apperror.ErrNotFound
		}
		_, err = s.audits().InsertOne(tx, audit)
		return mapError(err)
	})
	return count, err
}

func (s *Store) FindPenaltyByID(ctx context.Context, penaltyID string) (model.Penalty, error) {
	var penalty model.Penalty
	err := s.penalties().FindOne(ctx, bson.M{"penalty_id": penaltyID}).Decode(&penalty)
	return penalty, mapError(err)
}

func (s *Store) FindPenaltyByEvidenceID(ctx context.Context, evidenceID string) (model.Penalty, error) {
	var penalty model.Penalty
	err := s.penalties().FindOne(ctx, bson.M{"evidence_id": evidenceID}).Decode(&penalty)
	return penalty, mapError(err)
}

func (s *Store) ListMemberPenalties(ctx context.Context, memberID string) ([]model.Penalty, error) {
	cursor, err := s.penalties().Find(ctx, bson.M{"member_id": memberID}, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}, {Key: "penalty_id", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var penalties []model.Penalty
	if err := cursor.All(ctx, &penalties); err != nil {
		return nil, err
	}
	return penalties, nil
}

func (s *Store) PardonPenaltyWithAudit(ctx context.Context, penaltyID, actorID, reason string, now time.Time, audit model.AuditLog) (model.Penalty, int, error) {
	var penalty model.Penalty
	count := 0
	err := s.db.WithTransaction(ctx, func(tx context.Context) error {
		err := s.penalties().FindOneAndUpdate(tx, bson.M{"penalty_id": penaltyID, "pardoned_at": nil}, bson.M{"$set": bson.M{"pardoned_at": now, "pardoned_by": actorID, "pardon_reason": reason}}, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&penalty)
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				var existing model.Penalty
				if lookupErr := s.penalties().FindOne(tx, bson.M{"penalty_id": penaltyID}).Decode(&existing); lookupErr == nil {
					return apperror.ErrConflict
				}
			}
			return mapError(err)
		}
		active, err := s.penalties().CountDocuments(tx, activePenaltyQuery(penalty.MemberID, now))
		if err != nil {
			return err
		}
		count = int(active)
		if _, err := s.members().UpdateOne(tx, bson.M{"member_id": penalty.MemberID}, bson.M{"$set": bson.M{"penalty_count": count, "updated_at": now}}); err != nil {
			return err
		}
		_, err = s.audits().InsertOne(tx, audit)
		return mapError(err)
	})
	return penalty, count, err
}

func (s *Store) RecalculateMemberPenaltyCount(ctx context.Context, memberID string, now time.Time) (int, error) {
	active, err := s.penalties().CountDocuments(ctx, activePenaltyQuery(memberID, now))
	if err != nil {
		return 0, err
	}
	result, err := s.members().UpdateOne(ctx, bson.M{"member_id": memberID, "deleted_at": nil}, bson.M{"$set": bson.M{"penalty_count": active}})
	if err != nil {
		return 0, err
	}
	if result.MatchedCount == 0 {
		return 0, apperror.ErrNotFound
	}
	return int(active), nil
}

func (s *Store) RecalculateAllPenaltyCounts(ctx context.Context, now time.Time) error {
	if _, err := s.members().UpdateMany(ctx, bson.M{"deleted_at": nil}, bson.M{"$set": bson.M{"penalty_count": 0}}); err != nil {
		return err
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"pardoned_at": nil, "$and": bson.A{
			bson.M{"$or": bson.A{bson.M{"starts_at": nil}, bson.M{"starts_at": bson.M{"$lte": now}}}},
			bson.M{"$or": bson.A{bson.M{"duration_type": model.DurationPermanent}, bson.M{"expires_at": bson.M{"$gt": now}}}},
		}}}},
		{{Key: "$group", Value: bson.M{"_id": "$member_id", "count": bson.M{"$sum": 1}}}},
	}
	cursor, err := s.penalties().Aggregate(ctx, pipeline)
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)
	type activeCount struct {
		MemberID string `bson:"_id"`
		Count    int    `bson:"count"`
	}
	var counts []activeCount
	if err := cursor.All(ctx, &counts); err != nil {
		return err
	}
	if len(counts) == 0 {
		return nil
	}
	writes := make([]mongo.WriteModel, 0, len(counts))
	for _, item := range counts {
		writes = append(writes, mongo.NewUpdateOneModel().SetFilter(bson.M{"member_id": item.MemberID, "deleted_at": nil}).SetUpdate(bson.M{"$set": bson.M{"penalty_count": item.Count}}))
	}
	_, err = s.members().BulkWrite(ctx, writes, options.BulkWrite().SetOrdered(false))
	return err
}

func (s *Store) ListPenaltyRules(ctx context.Context) ([]model.PenaltyRule, error) {
	cursor, err := s.catalog().Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "category", Value: 1}, {Key: "name", Value: 1}, {Key: "rule_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var rules []model.PenaltyRule
	if err := cursor.All(ctx, &rules); err != nil {
		return nil, err
	}
	return rules, nil
}

func (s *Store) CreatePenaltyRuleWithAudit(ctx context.Context, rule model.PenaltyRule, audit model.AuditLog) error {
	return s.db.WithTransaction(ctx, func(tx context.Context) error {
		if _, err := s.catalog().InsertOne(tx, rule); err != nil {
			return mapError(err)
		}
		_, err := s.audits().InsertOne(tx, audit)
		return mapError(err)
	})
}

func (s *Store) UpdatePenaltyRuleWithAudit(ctx context.Context, rule model.PenaltyRule, audit model.AuditLog) (model.PenaltyRule, error) {
	var updated model.PenaltyRule
	err := s.db.WithTransaction(ctx, func(tx context.Context) error {
		err := s.catalog().FindOneAndUpdate(tx, bson.M{"rule_id": rule.RuleID}, bson.M{"$set": bson.M{"name": rule.Name, "category": rule.Category, "fine": rule.Fine, "jail": rule.Jail, "description": rule.Description, "updated_at": rule.UpdatedAt}}, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&updated)
		if err != nil {
			return mapError(err)
		}
		_, err = s.audits().InsertOne(tx, audit)
		return mapError(err)
	})
	return updated, err
}

func (s *Store) DeletePenaltyRuleWithAudit(ctx context.Context, ruleID string, audit model.AuditLog) error {
	return s.db.WithTransaction(ctx, func(tx context.Context) error {
		result, err := s.catalog().DeleteOne(tx, bson.M{"rule_id": ruleID})
		if err != nil {
			return err
		}
		if result.DeletedCount == 0 {
			return apperror.ErrNotFound
		}
		_, err = s.audits().InsertOne(tx, audit)
		return mapError(err)
	})
}

func (s *Store) ListAuditLogs(ctx context.Context, filter repository.AuditFilter) ([]model.AuditLog, int64, error) {
	total, err := s.audits().CountDocuments(ctx, bson.M{})
	if err != nil {
		return nil, 0, err
	}
	cursor, err := s.audits().Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}, {Key: "audit_id", Value: -1}}).SetSkip(int64((filter.Page.Number-1)*filter.Page.Limit)).SetLimit(int64(filter.Page.Limit)))
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)
	var logs []model.AuditLog
	if err := cursor.All(ctx, &logs); err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

func (s *Store) ListDepartments(ctx context.Context) ([]string, error) {
	values, err := s.members().Distinct(ctx, "department", bson.M{"deleted_at": nil, "department": bson.M{"$nin": bson.A{"", nil}}})
	if err != nil {
		return nil, err
	}
	departments := make([]string, 0, len(values))
	for _, value := range values {
		if department, ok := value.(string); ok && strings.TrimSpace(department) != "" {
			departments = append(departments, department)
		}
	}
	sort.Strings(departments)
	return departments, nil
}

var _ repository.Store = (*Store)(nil)
