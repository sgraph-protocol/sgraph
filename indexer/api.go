package main

import (
	"context"
	"fmt"
	"time"

	"github.com/sgraph-protocol/sgraph/indexer/types"
	graph "github.com/sgraph-protocol/sgraph/sdk/go"
)

type API struct {
	repo Mongo
}

func NewAPI(m Mongo) API {
	return API{m}
}

type GetRelationsParams struct {
	From      string   `json:"from"`
	To        string   `json:"to"`
	Providers []string `json:"providers"`
	After     string   `json:"after"`
	Limit     uint     `json:"limit"`
}

type GetRelationsResp struct {
	Relations []types.Relation `json:"relations"`
}

func (a API) FindRelations(ctx context.Context, params GetRelationsParams) (GetRelationsResp, error) {
	if params.Limit == 0 {
		params.Limit = 100
	}
	if params.Limit > 1000 {
		return GetRelationsResp{}, fmt.Errorf("invalid limit")
	}

	relations, err := a.repo.FetchRelations(ctx, params.From, params.To, params.Providers, params.After, params.Limit)
	if err != nil {
		return GetRelationsResp{}, fmt.Errorf("fetch relations: %w", err)
	}

	return GetRelationsResp{
		Relations: sliceMap(relations, func(r graph.Relation) types.Relation {
			return types.Relation{
				From:           r.From.ToBase58(),
				To:             r.To.ToBase58(),
				Provider:       r.Provider.ToBase58(),
				ConnectedAt:    time.Unix(r.ConnectedAt, 0),
				DisconnectedAt: optionMap(r.DisconnectedAt, func(ts int64) time.Time { return time.Unix(ts, 0) }),
				Extra:          r.Extra,
			}
		}),
	}, nil
}

func sliceMap[T, U any](input []T, f func(T) U) []U {
	output := make([]U, len(input))
	for i, elem := range input {
		output[i] = f(elem)
	}
	return output
}

func optionMap[T, G any](ptr *T, f func(T) G) *G {
	if ptr == nil {
		return nil
	}
	g := f(*ptr)
	return &g
}
