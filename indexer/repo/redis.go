package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-pkgz/lgr"
	"github.com/gomodule/redigo/redis"
)

type Redis struct {
	pool *redis.Pool
	l    lgr.L
}

func NewRedis(l lgr.L, pool *redis.Pool) (r Redis) {
	return Redis{
		pool,
		l,
	}
}

func (h Redis) InitializeRedis(ctx context.Context) error {
	handleErr := func(err error) error {
		return fmt.Errorf("error registering handlers library: %w", err)
	}

	conn, err := h.pool.GetContext(ctx)
	if err != nil {
		return err
	}

	// create consumer group
	_, err = redis.String(conn.Do("XGROUP", "CREATE", blockStreamKey, groupName, 0, "MKSTREAM"))
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return handleErr(fmt.Errorf("create consumer group: %w", err))
	}

	return nil
}

const lastSeenBlockKey = "last_seen_block"

func (h Redis) SaveLastSeenBlock(ctx context.Context, block uint64) error {
	handleErr := func(err error) error {
		return fmt.Errorf("error saving last seen block: %w", err)
	}

	conn, err := h.pool.GetContext(ctx)
	if err != nil {
		return handleErr(err)
	}

	if _, err := redis.String(conn.Do("SET", lastSeenBlockKey, fmt.Sprint(block))); err != nil {
		return handleErr(err)
	}

	return nil
}

func (h Redis) GetLastSeenBlock(ctx context.Context) (uint64, error) {
	handleErr := func(err error) (uint64, error) {
		return 0, fmt.Errorf("error getting last seen block: %w", err)
	}

	conn, err := h.pool.GetContext(ctx)
	if err != nil {
		return handleErr(err)
	}

	n, err := redis.String(conn.Do("GET", lastSeenBlockKey))
	if err == redis.ErrNil {
		return 0, nil
	} else if err != nil {
		return handleErr(err)
	}

	block, err := strconv.ParseUint(n, 10, 64)
	if err != nil {
		return handleErr(err)
	}

	return block, err
}

const blockStreamKey = "indexer:block_stream"
const groupName = "block_processor"

// we only store blockID
type blockEvent struct {
	BlockID uint64 `redis:"block"`
}

// AddTransaction tries to add blocks to the processing stream
// and returns blocks that have already been scheduled (if any)
func (r Redis) AddBlocks(ctx context.Context, blocks []uint64) error {
	handleErr := func(err error) error {
		return fmt.Errorf("adding blocks to pipeline: %w", err)
	}
	conn, err := r.pool.GetContext(ctx)
	if err != nil {
		return handleErr(err)
	}
	defer conn.Close()

	const maxStreamLen = "2000000" // TODO adjust or use MINID. Enough to sustain two weeks of harvester downtime

	// use pipelining
	for _, e := range blocks {
		e, err := json.Marshal(e)
		if err != nil {
			return handleErr(fmt.Errorf("failed to marshal event: %w", err))
		}

		if err := conn.Send("XADD", blockStreamKey, "MAXLEN", "~", maxStreamLen, "*", "block", string(e)); err != nil {
			return handleErr(err)
		}
	}

	if err := conn.Flush(); err != nil {
		return handleErr(err)
	}

	// check everything is ok
	for range blocks {
		_, err := redis.String(conn.Receive())
		if err != nil {
			return handleErr(err)
		}
	}

	return nil
}

// EventID is opaque identifier used identifying and ack'ing prococessed events
// In Redis implementations corresponds to entry ID returned by XADD, e.g. `1518951480106-0`
type EventID = string

func (r Redis) FetchStreamEvents(ctx context.Context, consumerID string, batchSize uint) (map[EventID]uint64, error) {
	handleErr := func(err error) (map[EventID]uint64, error) {
		return nil, fmt.Errorf("fetch event streams: %w", err)
	}

	if batchSize == 0 {
		return map[string]uint64{}, nil
	}

	conn, err := r.pool.GetContext(ctx)
	if err != nil {
		return handleErr(err)
	}
	defer conn.Close()

	args := []any{"GROUP", groupName, consumerID, "BLOCK", 500, "COUNT", batchSize, "STREAMS", blockStreamKey, ">"}
	notifications, err := StreamNotifications[blockEvent](conn.Do("XREADGROUP", args...))
	if err == redis.ErrNil {
		return make(map[EventID]uint64), nil // return empty batch
	} else if err != nil {
		return handleErr(fmt.Errorf("read transactions stream: %w", err))
	}

	events, ok := notifications[blockStreamKey]
	if !ok {
		return handleErr(fmt.Errorf("unexpected response: no items from subscribed stream"))
	}

	batch := make(map[string]uint64)

	for _, event := range events {
		batch[event.ID] = event.Value.BlockID
	}

	return batch, nil
}

func (r Redis) FindStaleBlocks(ctx context.Context, consumerID string, timeout time.Duration, batchSize uint) (map[EventID]uint64, error) {
	handleErr := func(err error) (map[EventID]uint64, error) {
		return nil, fmt.Errorf("fetch event streams: %w", err)
	}

	conn, err := r.pool.GetContext(ctx)
	if err != nil {
		return handleErr(err)
	}
	defer conn.Close()

	minIdleTime := timeout.Milliseconds()
	startID := "0"

	args := []any{blockStreamKey, groupName, consumerID, minIdleTime, startID, "COUNT", batchSize}
	resp, err := redis.Values(conn.Do("XAUTOCLAIM", args...))
	if err != nil {
		return handleErr(fmt.Errorf("find stale events: %w", err))
	}

	if len(resp) != 3 {
		return handleErr(fmt.Errorf("invalid xautoclaim response: %v", resp))
	}

	events, err := Entries[blockEvent](resp[1], nil)
	if err != nil {
		return handleErr(fmt.Errorf("unexpected response: no items from subscribed stream"))
	}

	batch := make(map[string]uint64)

	for _, event := range events {
		batch[event.ID] = event.Value.BlockID
	}

	return batch, nil
}

func (r Redis) AcknowledgeBlocks(ctx context.Context, events []EventID) error {
	conn, err := r.pool.GetContext(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	args := []any{blockStreamKey, groupName}

	for _, e := range events {
		args = append(args, e)
	}

	if _, err := redis.Int(conn.Do("XACK", args...)); err != nil {
		return err
	}

	return nil
}

// streamEntry represents a single stream entry.
type streamEntry[inner any] struct {
	ID    string
	Value inner
}

func StreamNotifications[inner any](reply any, err error) (map[string][]streamEntry[inner], error) {
	notifications, err := redis.Values(reply, err)
	if err != nil {
		return nil, err
	}

	result := make(map[string][]streamEntry[inner], len(notifications)/2)

	for _, notification := range notifications {
		evs, ok := notification.([]any)
		if !ok || len(evs) != 2 {
			return nil, errors.New("redigo: Entry expects two value result")
		}

		id, err := redis.String(evs[0], nil)
		if err != nil {
			return nil, err
		}

		entries, err := Entries[inner](evs[1], nil)
		if err != nil {
			return nil, err
		}

		result[id] = entries

	}

	return result, nil
}

// Entries is a helper that converts an array of stream entries into Entry values.
// Requires two values in each entry result, and an even number of field values.
func Entries[inner any](reply any, err error) ([]streamEntry[inner], error) {
	vs, err := redis.Values(reply, err)
	if err != nil {
		return nil, err
	}

	entries := make([]streamEntry[inner], len(vs))
	for i, v := range vs {
		evs, ok := v.([]any)
		if !ok || len(evs) != 2 {
			return nil, errors.New("invalid input: Entries expects two value result")
		}
		id, err := redis.String(evs[0], nil)
		if err != nil {
			return nil, err
		}

		fields, ok := evs[1].([]any)
		if !ok {
			return nil, errors.New("unexpected structure")
		}

		var value inner

		if err := redis.ScanStruct(fields, &value); err != nil {
			return nil, err
		}
		entries[i] = streamEntry[inner]{
			ID:    id,
			Value: value,
		}
	}
	return entries, nil
}
