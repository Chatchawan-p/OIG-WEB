// Package mongodb owns the MongoDB connection lifecycle and infrastructure indexes.
package mongodb

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// Database wraps the driver types needed by repositories and health checks.
type Database struct {
	client   *mongo.Client
	database *mongo.Database
}

func Connect(ctx context.Context, uri, databaseName string) (*Database, error) {
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri).SetAppName("oig-api"))
	if err != nil {
		return nil, fmt.Errorf("connect mongodb: %w", err)
	}
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("ping mongodb: %w", err)
	}
	return &Database{client: client, database: client.Database(databaseName)}, nil
}

func (db *Database) Collection(name string) *mongo.Collection {
	return db.database.Collection(name)
}

func (db *Database) Ping(ctx context.Context) error {
	return db.client.Ping(ctx, readpref.Primary())
}

func (db *Database) Disconnect(ctx context.Context) error {
	return db.client.Disconnect(ctx)
}

// WithTransaction uses a MongoDB transaction when the deployment supports it.
// Standalone development MongoDB falls back to ordered writes; services can
// reconcile derived penalty counts through the repository reconciliation APIs.
func (db *Database) WithTransaction(ctx context.Context, fn func(context.Context) error) error {
	session, err := db.client.StartSession()
	if err != nil {
		return fmt.Errorf("start mongodb session: %w", err)
	}
	defer session.EndSession(ctx)

	_, err = session.WithTransaction(ctx, func(sessionCtx mongo.SessionContext) (any, error) {
		return nil, fn(sessionCtx)
	})
	if err == nil {
		return nil
	}
	var commandErr mongo.CommandError
	if errors.As(err, &commandErr) && (commandErr.Code == 20 || commandErr.Code == 303) {
		return fn(ctx)
	}
	if strings.Contains(err.Error(), "Transaction numbers are only allowed") {
		return fn(ctx)
	}
	return err
}
