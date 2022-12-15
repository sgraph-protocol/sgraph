package main

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cristalhq/aconfig"
	"github.com/go-pkgz/lgr"
	"github.com/gomodule/redigo/redis"
	"github.com/portto/solana-go-sdk/common"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/sgraph-protocol/sgraph/indexer/cli"
	"github.com/sgraph-protocol/sgraph/indexer/repo"
	"github.com/sgraph-protocol/sgraph/indexer/srv"
	graph "github.com/sgraph-protocol/sgraph/sdk/go"

	"golang.org/x/exp/constraints"

	"net/http"
	_ "net/http/pprof"
)

// todo graceful exit
// todo metrics

//go:generate go run github.com/mailru/easyjson/... -all ./cli/cli.go
//go:generate go run github.com/matryer/moq@v0.2.7 -pkg mocks -fmt goimports -rm -skip-ensure -out ./mocks/rpc.go . RPC:RpcMock
//go:generate go run github.com/matryer/moq@v0.2.7 -pkg mocks -fmt goimports -rm -skip-ensure -out ./mocks/redis.go . Redis:RedisMock

type config struct {
	LogLevel                  string
	RpcEndpoint               string
	BlockProcessorConcurrency int

	RedisHost string
	RedisPort int

	MongoHost string
}

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}

type redisCfg struct {
	redisAddr string
}

func newRedisPool(cfg redisCfg, l lgr.L) (*redis.Pool, func(), error) {
	handleErr := func(err error) (*redis.Pool, func(), error) {
		return nil, nil, fmt.Errorf("new redis pool: %w", err)
	}

	const (
		maxIdle     = 10
		idleTimeout = 5 * time.Minute
	)

	dialFunc := func(ctx context.Context) (redis.Conn, error) {
		return redis.DialContext(ctx, "tcp", cfg.redisAddr)
	}

	p := redis.Pool{
		MaxIdle:     maxIdle,
		IdleTimeout: idleTimeout,
		DialContext: func(ctx context.Context) (redis.Conn, error) {
			c, err := dialFunc(ctx)
			if err != nil {
				return nil, fmt.Errorf("connect to redis: %v", err)
			}
			if _, err = c.Do("PING"); err != nil {
				return nil, fmt.Errorf("redis ping: %w", err)
			}
			return c, nil
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			if time.Since(t) < time.Minute {
				return nil
			}
			if _, err := c.Do("PING"); err != nil {
				return fmt.Errorf("redis ping: %w", err)
			}
			return nil
		},
	}

	conn, err := p.DialContext(context.Background())
	if err != nil {
		return handleErr(fmt.Errorf("make test redis connection on initialization: %w", err))
	}
	conn.Close()

	cleanup := func() {
		if err := p.Close(); err != nil {
			l.Logf("%v", err)
		}
	}

	return &p, cleanup, nil
}

func prepare(ctx context.Context, l lgr.L, cfg config) (*redis.Pool, repo.Redis, func(), error) {
	handleErr := func(err error) (*redis.Pool, repo.Redis, func(), error) {
		return nil, repo.Redis{}, nil, err
	}

	fmt.Println(cfg)
	addr := fmt.Sprintf("%s:%d", cfg.RedisHost, cfg.RedisPort)
	fmt.Println(addr)

	pool, cleanup2, err := newRedisPool(redisCfg{redisAddr: fmt.Sprintf("%s:%d", cfg.RedisHost, cfg.RedisPort)}, lgr.New())
	if err != nil {
		return handleErr(fmt.Errorf("error connecting to redis: %w", err))
	}

	redis := repo.NewRedis(l, pool)

	if err := redis.InitializeRedis(ctx); err != nil {
		return handleErr(fmt.Errorf("error initializing redis: %v", err))
	}

	return pool, redis, func() {
		cleanup2()
	}, nil
}

const (
	mongoPort   = 27017
	mongoDBName = "graph"
)

func MakeMongo(ctx context.Context, host string, l lgr.L) (repo.Mongo, func(), error) {
	handleErr := func(err error) (repo.Mongo, func(), error) {
		return repo.Mongo{}, nil, fmt.Errorf("new mongo client: %w", err)
	}

	mongoURI := fmt.Sprintf("mongodb://%s:%d/%s", host, mongoPort, mongoDBName)

	opts := options.Client().ApplyURI(mongoURI)
	client, err := mongo.Connect(ctx, opts)
	if err != nil {
		return handleErr(err)
	}
	// TODO figure out readPreference
	if err := client.Ping(ctx, nil); err != nil {
		return handleErr(err)
	}
	cleanup := func() {
		if err := client.Disconnect(ctx); err != nil {
			l.Logf("disconnect from mongodb: %v", err, ctx)
		}
	}
	return repo.NewMongo(client, l), cleanup, nil
}

func run() error {
	var cfg config
	loader := aconfig.LoaderFor(&cfg, aconfig.Config{AllFieldRequired: true})
	if err := loader.Load(); err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	logLevel := lgr.Debug
	if strings.ToLower(cfg.LogLevel) == "trace" {
		logLevel = lgr.Trace
	}

	l := lgr.New(logLevel, lgr.CallerFile)

	_, redis, cleanup, err := prepare(ctx, l, cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	mongo, cleanup2, err := MakeMongo(ctx, cfg.MongoHost, l)
	if err != nil {
		return fmt.Errorf("init mongo: %w", err)
	}
	defer cleanup2()

	rpc := cli.NewRpc(l, cfg.RpcEndpoint)

	h, err := NewBlockHarvester(l, rpc, redis)
	if err != nil {
		return fmt.Errorf("fail to initialize harvester instance: %w", err)
	}

	p, err := NewProcesor(l, rpc, redis, mongo)
	if err != nil {
		return fmt.Errorf("fail to initialize processor instance: %w", err)
	}

	api := NewAPI(mongo)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := h.HarvestBlocks(ctx); !errors.Is(err, context.Canceled) {
			lgr.Fatalf("error starting block harvester: %v", err)
		}
	}()

	const consumerID = 0 // todo specify when we scale

	processors := cfg.BlockProcessorConcurrency
	if processors < 1 {
		processors = runtime.GOMAXPROCS(-1)
	}

	l.Logf("using %d threads for processing blocks (gomaxprocs = %d)", processors, runtime.GOMAXPROCS(-1))

	for i := 0; i < processors; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.Logf("starting processor #%d", i)

			if err := p.StartProcessingBlocks(ctx, consumerID, uint(i)); !errors.Is(err, context.Canceled) {
				panic(fmt.Sprintf("error starting processor: %v", err))
			}
		}()
	}

	const reportInterval = time.Second * 30

	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(reportInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.ReportProgress()
			}
		}
	}()

	// pprof
	go func() {
		l.Logf("[ERROR] bind pprof server: %v", http.ListenAndServe("0.0.0.0:4444", nil))
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		runServer(ctx, api)
	}()

	WaitForShutdownSignal(
		func() { l.Logf("[WARN] received exit signal. waiting for all processes to finish") },
		cancelCtx,
		wg.Wait,
	)

	l.Logf("goodbye")
	return nil
}

func runServer(ctx context.Context, a API) {
	s := srv.NewServer()
	s.Register("sg_findRelations", srv.WrapH(a.FindRelations))

	if err := s.Run(ctx); err != http.ErrServerClosed {
		panic(err)
	}
}

type Redis interface {
	// initialize exists only on implementation
	SaveLastSeenBlock(ctx context.Context, block uint64) error
	GetLastSeenBlock(ctx context.Context) (uint64, error)

	AddBlocks(ctx context.Context, blocks []uint64) error
	FetchStreamEvents(ctx context.Context, consumerID string, batchSize uint) (map[repo.EventID]uint64, error)
	FindStaleBlocks(ctx context.Context, consumerID string, staleTimeout time.Duration, batchSize uint) (map[repo.EventID]uint64, error)
	AcknowledgeBlocks(ctx context.Context, events []repo.EventID) error
}

type Mongo interface {
	FetchRelations(ctx context.Context, from, to string, providers []string, after string, limit uint) ([]graph.Relation, error)
	SaveRelations(ctx context.Context, relations []graph.Relation) error
}

type RPC interface {
	GetBlocks(ctx context.Context, retries uint, blocksIds ...uint64) ([]cli.Block, []int, error)
	GetBlocksWithLimit(ctx context.Context, from, limit uint64) ([]uint64, error)
	GetLatestBlock(ctx context.Context) (uint64, error)
}

type BlockHarvester struct {
	l lgr.L

	rpc RPC

	redis Redis
}

func NewBlockHarvester(l lgr.L, rpc RPC, redis repo.Redis) (BlockHarvester, error) {
	return BlockHarvester{l, rpc, redis}, nil
}

const (
	blockHarvestInterval = 400 * time.Millisecond // average block time
)

// called once
func (h *BlockHarvester) HarvestBlocks(ctx context.Context) error {
	handleErr := func(err error) error {
		return fmt.Errorf("harvest blocks: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return handleErr(ctx.Err())
		default:
		}

		// get latest processed block, if not present start with latest
		startBlock, err := h.redis.GetLastSeenBlock(ctx)
		if err != nil {
			return handleErr(err)
		}

		if startBlock == 0 {
			startBlock, err = h.rpc.GetLatestBlock(ctx)
			if err != nil {
				return handleErr(err)
			}
			h.l.Logf("[WARN] No saved block in Redis. Starting with latest block %d", startBlock)
		}

		// fetch all block that came after this one, insert them into stream
		const blockLimit = 1000
		blocks, err := h.rpc.GetBlocksWithLimit(ctx, startBlock, blockLimit)
		if err != nil {
			return handleErr(err)
		}

		h.l.Logf("[TRACE] adding blocks %d", blocks)
		if err := h.redis.AddBlocks(ctx, blocks); err != nil {
			return handleErr(fmt.Errorf("error adding blocks to the queue: %w", err))
		}

		h.l.Logf("[TRACE] added %d blocks\n", len(blocks))

		lastSeen := startBlock

		if len(blocks) > 0 {
			lastSeen = blocks[len(blocks)-1] + 1 // start from next one
		}

		if err := h.redis.SaveLastSeenBlock(ctx, lastSeen); err != nil {
			return handleErr(err)
		}

		// sleep some more
		time.Sleep(blockHarvestInterval)
	}
}

type ParsedEvent struct {
	// Involved user
	InvolvedUser ed25519.PublicKey
	// Event kind
	Kind eventKind
	// Extra values event could have
	Extra map[string]any
}

type eventKind string

// * swap
// * buy
// * send/receive tokens
// * send/receive SOL
// * stake/unstake
const (
	Buy     eventKind = "BUY"
	Send    eventKind = "SEND"
	Receive eventKind = "RECEIVE"
	// NFT repost rewards
	RewardsReceived eventKind = "REWARDS_RECEIVED"
	// Split distribution was credited to user voucher
	SplitReceived eventKind = "SPLIT_RECEIVED"
	// Split voucher balance was claimed by user
	SplitClaimed eventKind = "SPLIT_CLAIMED"
	// Split distributed
	SplitDistributed eventKind = "SPLIT_DISTRIBUTED"
	// Treasury has received something
	TreasuryReceive eventKind = "TREASURY_RECEIVE"
)

func abs[N constraints.Signed](n N) N {
	if n < 0 {
		return -n
	}
	return n
}

func toCommonPub(pub ed25519.PublicKey) common.PublicKey {
	return *(*common.PublicKey)(pub)
}

func keys[K comparable, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}

func values[K comparable, V any](m map[K]V) []V {
	values := make([]V, 0, len(m))
	for _, value := range m {
		values = append(values, value)
	}
	return values
}

// func runHealthz(mongoClient *mongo.Client, redisPool *redis.Pool, l lgr.L) {
// 	mux := http.NewServeMux()
// 	healthzHandler := newHealthzHandler(mongoClient, redisPool)
// 	// TODO readyz: it should probably somehow ensure harvester is up and running
// 	mux.HandleFunc("/healthz", healthzHandler)
// 	if err := http.ListenAndServe(":8080", mux); err != nil {
// 		l.Logf("healthz: %v", err)
// 	}
// }

// func newHealthzHandler(mongoClient *mongo.Client, redisPool *redis.Pool) func(w http.ResponseWriter, r *http.Request) {
// 	return func(w http.ResponseWriter, r *http.Request) {
// 		ctx := r.Context()
// 		if err := mongoClient.Ping(ctx, nil); err != nil {
// 			w.WriteHeader(http.StatusInternalServerError)
// 			return
// 		}
// 		c, err := redisPool.DialContext(ctx)
// 		if err != nil {
// 			w.WriteHeader(http.StatusInternalServerError)
// 			return
// 		}
// 		defer c.Close()
// 		if err := c.Send("PING"); err != nil {
// 			w.WriteHeader(http.StatusInternalServerError)
// 			return
// 		}
// 	}
// }

func WaitForShutdownSignal(cf ...func()) {
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	<-done
	signal.Stop(done)

	lgr.Printf("shutting things down")

	for _, f := range cf {
		f()
	}
}
