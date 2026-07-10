package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/fresp/it-tools-portal/internal/models"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var ErrToolNotFound = errors.New("tool not found")

type MongoToolRepository struct {
	collection *mongo.Collection
	now        func() time.Time
}

func NewMongoToolRepository(collection *mongo.Collection) *MongoToolRepository {
	return &MongoToolRepository{
		collection: collection,
		now:        time.Now,
	}
}

func (r *MongoToolRepository) EnsureIndexes(ctx context.Context) error {
	_, err := r.collection.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "is_active", Value: 1}},
			Options: options.Index().SetName("idx_tools_is_active"),
		},
		{
			Keys:    bson.D{{Key: "allowed_groups", Value: 1}},
			Options: options.Index().SetName("idx_tools_allowed_groups"),
		},
	})
	return err
}

func (r *MongoToolRepository) IndexNames(ctx context.Context) ([]string, error) {
	cursor, err := r.collection.Indexes().List(ctx)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	names := make([]string, 0)
	for cursor.Next(ctx) {
		var index struct {
			Name string `bson:"name"`
		}
		if err := cursor.Decode(&index); err != nil {
			return nil, err
		}
		names = append(names, index.Name)
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return names, nil
}

func (r *MongoToolRepository) Create(ctx context.Context, request models.CreateToolRequest) (models.Tool, error) {
	if err := request.Validate(); err != nil {
		return models.Tool{}, err
	}

	active := true
	if request.IsActive != nil {
		active = *request.IsActive
	}
	now := r.now().UTC()
	tool := models.Tool{
		ID:             bson.NewObjectID().Hex(),
		Name:           request.Name,
		BaseURL:        request.BaseURL,
		IconURL:        request.IconURL,
		AllowedGroups:  request.AllowedGroups,
		HealthCheckURL: request.HealthCheckURL,
		IsActive:       active,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	_, err := r.collection.InsertOne(ctx, tool)
	if err != nil {
		return models.Tool{}, err
	}
	return tool, nil
}

func (r *MongoToolRepository) List(ctx context.Context) ([]models.Tool, error) {
	return r.find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
}

func (r *MongoToolRepository) ListAvailable(ctx context.Context, groups []string) ([]models.Tool, error) {
	filter := bson.D{
		{Key: "is_active", Value: true},
		{Key: "allowed_groups", Value: bson.D{{Key: "$in", Value: groups}}},
	}
	return r.find(ctx, filter, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
}

func (r *MongoToolRepository) Get(ctx context.Context, id string) (models.Tool, error) {
	var tool models.Tool
	err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&tool)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return models.Tool{}, ErrToolNotFound
	}
	if err != nil {
		return models.Tool{}, err
	}
	return tool, nil
}

func (r *MongoToolRepository) Update(ctx context.Context, id string, request models.UpdateToolRequest) (models.Tool, error) {
	if err := request.Validate(); err != nil {
		return models.Tool{}, err
	}

	set := bson.D{{Key: "updated_at", Value: r.now().UTC()}}
	if request.Name != nil {
		set = append(set, bson.E{Key: "name", Value: *request.Name})
	}
	if request.BaseURL != nil {
		set = append(set, bson.E{Key: "base_url", Value: *request.BaseURL})
	}
	if request.IconURL != nil {
		set = append(set, bson.E{Key: "icon_url", Value: *request.IconURL})
	}
	if request.AllowedGroups != nil {
		set = append(set, bson.E{Key: "allowed_groups", Value: request.AllowedGroups})
	}
	if request.HealthCheckURL != nil {
		set = append(set, bson.E{Key: "health_check_url", Value: nullableString(*request.HealthCheckURL)})
	}
	if request.IsActive != nil {
		set = append(set, bson.E{Key: "is_active", Value: *request.IsActive})
	}

	result, err := r.collection.UpdateOne(ctx, bson.D{{Key: "_id", Value: id}}, bson.D{{Key: "$set", Value: set}})
	if err != nil {
		return models.Tool{}, err
	}
	if result.MatchedCount == 0 {
		return models.Tool{}, ErrToolNotFound
	}
	return r.Get(ctx, id)
}

func (r *MongoToolRepository) Delete(ctx context.Context, id string) error {
	result, err := r.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}})
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		return ErrToolNotFound
	}
	return nil
}

func IsToolNotFound(err error) bool {
	return errors.Is(err, ErrToolNotFound)
}

func (r *MongoToolRepository) find(ctx context.Context, filter bson.D, opts *options.FindOptionsBuilder) ([]models.Tool, error) {
	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var tools []models.Tool
	if err := cursor.All(ctx, &tools); err != nil {
		return nil, err
	}
	if tools == nil {
		return []models.Tool{}, nil
	}
	return tools, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
