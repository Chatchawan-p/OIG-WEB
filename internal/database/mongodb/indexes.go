package mongodb

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type collectionIndexes struct {
	collection string
	models     []mongo.IndexModel
}

// EnsureIndexes creates the stable identity, uniqueness, and primary query indexes
// required by the documented data model. MongoDB index creation is idempotent.
func (db *Database) EnsureIndexes(ctx context.Context) error {
	indexes := []collectionIndexes{
		{collection: "users", models: []mongo.IndexModel{
			{Keys: bson.D{{Key: "user_id", Value: 1}}, Options: uniqueIndex("uq_users_user_id")},
			{Keys: bson.D{{Key: "discord_id", Value: 1}}, Options: options.Index().SetUnique(true).SetName("uq_users_discord_id")},
			{Keys: bson.D{{Key: "role", Value: 1}}, Options: namedIndex("ix_users_role")},
		}},
		{collection: "police_members", models: []mongo.IndexModel{
			{Keys: bson.D{{Key: "member_id", Value: 1}}, Options: options.Index().SetUnique(true).SetName("uq_members_member_id")},
			{Keys: bson.D{{Key: "police_id", Value: 1}}, Options: options.Index().SetUnique(true).SetName("uq_members_police_id")},
			{Keys: bson.D{{Key: "generation", Value: 1}, {Key: "police_id", Value: 1}}, Options: options.Index().SetName("ix_members_generation_police_id")},
			{Keys: bson.D{{Key: "department", Value: 1}, {Key: "employment_status", Value: 1}, {Key: "penalty_count", Value: 1}}, Options: options.Index().SetName("ix_members_filters")},
			{Keys: bson.D{{Key: "full_name", Value: "text"}, {Key: "police_id", Value: "text"}, {Key: "rank", Value: "text"}, {Key: "department", Value: "text"}, {Key: "unit", Value: "text"}}, Options: options.Index().SetName("ix_members_text_search")},
		}},
		{collection: "name_history", models: []mongo.IndexModel{
			{Keys: bson.D{{Key: "history_id", Value: 1}}, Options: options.Index().SetUnique(true).SetName("uq_name_history_history_id")},
			{Keys: bson.D{{Key: "member_id", Value: 1}, {Key: "changed_at", Value: -1}}, Options: options.Index().SetName("ix_name_history_member_changed")},
		}},
		{collection: "penalties", models: []mongo.IndexModel{
			{Keys: bson.D{{Key: "penalty_id", Value: 1}}, Options: options.Index().SetUnique(true).SetName("uq_penalties_penalty_id")},
			{Keys: bson.D{{Key: "member_id", Value: 1}, {Key: "created_at", Value: -1}}, Options: options.Index().SetName("ix_penalties_member_created")},
			{Keys: bson.D{{Key: "pardoned_at", Value: 1}, {Key: "expires_at", Value: 1}}, Options: options.Index().SetName("ix_penalties_active")},
			{Keys: bson.D{{Key: "evidence_id", Value: 1}}, Options: options.Index().SetUnique(true).SetSparse(true).SetName("uq_penalties_evidence_id")},
		}},
		{collection: "penalty_types", models: []mongo.IndexModel{
			{Keys: bson.D{{Key: "type_id", Value: 1}}, Options: options.Index().SetUnique(true).SetName("uq_penalty_types_type_id")},
			{Keys: bson.D{{Key: "legacy_id", Value: 1}}, Options: options.Index().SetUnique(true).SetSparse(true).SetName("uq_penalty_types_legacy_id")},
			{Keys: bson.D{{Key: "name", Value: 1}}, Options: options.Index().SetUnique(true).SetName("uq_penalty_types_name")},
		}},
		{collection: "penalty_catalog", models: []mongo.IndexModel{
			{Keys: bson.D{{Key: "rule_id", Value: 1}}, Options: options.Index().SetUnique(true).SetName("uq_penalty_catalog_rule_id")},
			{Keys: bson.D{{Key: "category", Value: 1}, {Key: "name", Value: 1}}, Options: options.Index().SetName("ix_penalty_catalog_category_name")},
		}},
		{collection: "audit_logs", models: []mongo.IndexModel{
			{Keys: bson.D{{Key: "audit_id", Value: 1}}, Options: options.Index().SetUnique(true).SetName("uq_audit_logs_audit_id")},
			{Keys: bson.D{{Key: "created_at", Value: -1}}, Options: options.Index().SetName("ix_audit_logs_created")},
			{Keys: bson.D{{Key: "actor_id", Value: 1}, {Key: "created_at", Value: -1}}, Options: options.Index().SetName("ix_audit_logs_actor_created")},
		}},
	}

	for _, item := range indexes {
		if _, err := db.Collection(item.collection).Indexes().CreateMany(ctx, item.models); err != nil {
			return fmt.Errorf("create indexes for %s: %w", item.collection, err)
		}
	}
	return nil
}

func uniqueIndex(name string) *options.IndexOptions {
	return options.Index().SetUnique(true).SetName(name)
}

func namedIndex(name string) *options.IndexOptions {
	return options.Index().SetName(name)
}
