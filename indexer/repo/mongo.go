package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/go-pkgz/lgr"
	"github.com/portto/solana-go-sdk/common"
	"github.com/sgraph-protocol/sgraph/indexer/types"
	graph "github.com/sgraph-protocol/sgraph/sdk/go"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Mongo struct {
	database string

	c *mongo.Client
	l lgr.L
}

func NewMongo(c *mongo.Client, l lgr.L) Mongo {
	const database = "sgraph"
	return Mongo{database, c, l}
}

const (
	collectionEvents string = "relations"
)

func (m Mongo) SaveRelations(ctx context.Context, relations []graph.Relation) error {
	documents := make([]any, len(relations))
	for i := range relations {
		documents[i] = transformRelation(relations[i])
	}

	_, err := m.c.Database(m.database).Collection(collectionEvents).InsertMany(ctx, documents)
	if err != nil {
		return fmt.Errorf("insert documents: %w", err)
	}
	return nil
}

func (m Mongo) FetchRelations(ctx context.Context, from, to string, providers []string, after string, limit uint) ([]graph.Relation, error) {
	handleErr := func(err error) ([]graph.Relation, error) {
		return nil, fmt.Errorf("fetch events: %w", err)
	}

	c := m.c.Database(m.database).Collection(collectionEvents)

	opts := options.Find().SetSort(bson.D{{"_id", -1}}).SetLimit(int64(limit))

	query := primitive.M{}
	if from != "" {
		query["from"] = from
	}

	if to != "" {
		query["to"] = to
	}

	fmt.Println(providers)
	if len(providers) > 0 {
		query["provider"] = bson.M{"$in": providers}
	}

	if after != "" {
		oid, err := primitive.ObjectIDFromHex(after)
		if err != nil {
			return handleErr(err)
		}
		query["_id"] = primitive.M{"$lt": oid}
	}

	cur, err := c.Find(ctx, query, opts)
	if err != nil {
		return handleErr(fmt.Errorf("find records: %w", err))
	}

	var relations []types.Relation
	if err := cur.All(ctx, &relations); err != nil {
		return handleErr(fmt.Errorf("decode cursor: %w", err))
	}

	return sliceMap(relations, parseRelation), nil
}

func transformRelation(r graph.Relation) types.Relation {
	return types.Relation{
		From:           r.From.ToBase58(),
		To:             r.To.ToBase58(),
		Provider:       r.Provider.ToBase58(),
		ConnectedAt:    time.Unix(r.ConnectedAt, 0),
		DisconnectedAt: optionMap(r.DisconnectedAt, func(ts int64) time.Time { return time.Unix(ts, 0) }),
		Extra:          r.Extra,
	}
}

func parseRelation(r types.Relation) graph.Relation {
	return graph.Relation{
		From:           common.PublicKeyFromString(r.From),
		To:             common.PublicKeyFromString(r.To),
		Provider:       common.PublicKeyFromString(r.Provider),
		ConnectedAt:    r.ConnectedAt.Unix(),
		DisconnectedAt: optionMap(r.DisconnectedAt, func(ts time.Time) int64 { return ts.Unix() }),
		Extra:          r.Extra,
	}
}

func optionMap[T, G any](ptr *T, f func(T) G) *G {
	if ptr == nil {
		return nil
	}
	g := f(*ptr)
	return &g
}

func sliceMap[T, U any](input []T, f func(T) U) []U {
	output := make([]U, len(input))
	for i, elem := range input {
		output[i] = f(elem)
	}
	return output
}
