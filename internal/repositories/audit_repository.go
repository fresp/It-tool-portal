package repositories

import (
	"context"
	"time"

	"github.com/fresp/it-tools-portal/internal/models"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// AuditStore defines the interface for audit log persistence.
type AuditStore interface {
	Record(ctx context.Context, log models.AuditLog) error
	ListByUser(ctx context.Context, userID string) ([]models.AuditLog, error)
	ListByTool(ctx context.Context, toolID string) ([]models.AuditLog, error)
	ListByDateRange(ctx context.Context, from, to time.Time) ([]models.AuditLog, error)
}

// MongoAuditRepository implements AuditStore against MongoDB.
type MongoAuditRepository struct {
	collection *mongo.Collection
	now        func() time.Time
}

// NewMongoAuditRepository creates a new audit repository.
func NewMongoAuditRepository(collection *mongo.Collection) *MongoAuditRepository {
	return &MongoAuditRepository{
		collection: collection,
		now:        time.Now,
	}
}

// EnsureIndexes creates the required indexes for the audit_logs collection.
func (r *MongoAuditRepository) EnsureIndexes(ctx context.Context) error {
	_, err := r.collection.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}},
			Options: options.Index().SetName("idx_audit_user_id"),
		},
		{
			Keys:    bson.D{{Key: "tool_id", Value: 1}},
			Options: options.Index().SetName("idx_audit_tool_id"),
		},
		{
			Keys:    bson.D{{Key: "timestamp", Value: -1}},
			Options: options.Index().SetName("idx_audit_timestamp"),
		},
	})
	return err
}

// Record inserts a new audit log entry.
func (r *MongoAuditRepository) Record(ctx context.Context, log models.AuditLog) error {
	if log.Timestamp.IsZero() {
		log.Timestamp = r.now().UTC()
	}
	if log.ID == "" {
		log.ID = bson.NewObjectID().Hex()
	}
	_, err := r.collection.InsertOne(ctx, log)
	return err
}

// ListByUser returns all audit logs for a given user.
func (r *MongoAuditRepository) ListByUser(ctx context.Context, userID string) ([]models.AuditLog, error) {
	return r.find(ctx, bson.D{{Key: "user_id", Value: userID}})
}

// ListByTool returns all audit logs for a given tool.
func (r *MongoAuditRepository) ListByTool(ctx context.Context, toolID string) ([]models.AuditLog, error) {
	return r.find(ctx, bson.D{{Key: "tool_id", Value: toolID}})
}

// ListByDateRange returns all audit logs within a date range.
func (r *MongoAuditRepository) ListByDateRange(ctx context.Context, from, to time.Time) ([]models.AuditLog, error) {
	filter := bson.D{
		{Key: "timestamp", Value: bson.D{
			{Key: "$gte", Value: from.UTC()},
			{Key: "$lte", Value: to.UTC()},
		}},
	}
	return r.find(ctx, filter)
}

func (r *MongoAuditRepository) find(ctx context.Context, filter bson.D) ([]models.AuditLog, error) {
	opts := options.Find().SetSort(bson.D{{Key: "timestamp", Value: -1}})
	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var logs []models.AuditLog
	if err := cursor.All(ctx, &logs); err != nil {
		return nil, err
	}
	if logs == nil {
		return []models.AuditLog{}, nil
	}
	return logs, nil
}
