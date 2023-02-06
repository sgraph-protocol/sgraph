package main

import (
	"bytes"
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/go-pkgz/lgr"
	"github.com/hmn-fnd/borsh-go"
	"github.com/portto/solana-go-sdk/common"
	"github.com/portto/solana-go-sdk/types"
	"github.com/sgraph-protocol/sgraph/indexer/cli"
	graph "github.com/sgraph-protocol/sgraph/sdk/go"
)

type Processor struct {
	l   lgr.L
	rpc RPC

	redis Redis
	mongo Mongo

	lastProcessedBlock   uint64 // atomic
	processedBlocksCount uint64 // atomic

	// never accessed by multiple goroutines
	lastReportTime  time.Time
	lastReportBlock uint64
}

func NewProcesor(l lgr.L, rpc cli.RPC, redis Redis, mongo Mongo) (*Processor, error) {
	return &Processor{
		l,
		rpc,
		redis,
		mongo,
		0,
		0,
		time.Now(),
		0,
	}, nil
}

func (h *Processor) StartProcessingBlocks(pctx context.Context, consumerID, threadID uint) error {

	id := fmt.Sprintf("replica-%d-consumer-%d", consumerID, threadID)

	for {
		select {
		case <-pctx.Done():
			return pctx.Err()
		default:
		}

		// manage separate context cancellation to gracefully process remaining batches
		ctx := context.Background()

		if err := h.ProcessBlocks(ctx, id); err != nil {
			h.l.Logf("[ERROR] occured while processing blocks: %v", err)
		}
	}
}

func (p *Processor) ReportProgress() {
	ctx, cleanup := context.WithTimeout(context.Background(), time.Second*15)
	defer cleanup()

	latestBlock, err := p.rpc.GetLatestBlock(ctx)
	if err != nil {
		p.l.Logf("[ERROR] fetch latest block: %v", err)
		return
	}

	lastProcessed := atomic.LoadUint64(&p.lastProcessedBlock)
	count := atomic.SwapUint64(&p.processedBlocksCount, 0)

	delta := time.Since(p.lastReportTime).Seconds()
	gainRate := float64(count) / delta

	status := calcStatus(latestBlock, lastProcessed, gainRate)

	if p.lastReportBlock != 0 {
		p.l.Logf("[INFO] %d new blocks since %.1fs; rate = %.2f b/s; status = [%s]", count, delta, gainRate, status)
	}

	p.lastReportTime = time.Now()
	p.lastReportBlock = lastProcessed
}

func calcStatus(latestBlock, lastProcessed uint64, gainRate float64) string {
	if latestBlock < lastProcessed {
		return "UNKNOWN"
	}

	remaining := latestBlock - lastProcessed

	const delayTolerance = 250 // slots

	if remaining < delayTolerance {
		return fmt.Sprintf("UP-TO-DATE (%d slots behind)", remaining)
	}

	if gainRate > 1.0 {
		return fmt.Sprintf("CATCHING UP (%d slots behind)", remaining)
	} else {
		return fmt.Sprintf("STALLED!!! (%d slots behind)", remaining)
	}
}

const staleTimeout = time.Minute * 4

func (p *Processor) ProcessBlocks(ctx context.Context, consumerID string) error {
	const batchSize = 20

	staleBatch, err := p.redis.FindStaleBlocks(ctx, consumerID, staleTimeout, batchSize)
	if err != nil {
		return fmt.Errorf("await new transaction events: %w", err)
	}

	if len(staleBatch) > 0 {
		p.l.Logf("[WARN] found some stale blocks: %v", values(staleBatch))
	}

	remaining := uint(batchSize - len(staleBatch))

	batch, err := p.redis.FetchStreamEvents(ctx, consumerID, remaining)
	if err != nil {
		return fmt.Errorf("await new transaction events: %w", err)
	}

	// merge the two
	for id, b := range staleBatch {
		batch[id] = b
	}

	if len(batch) == 0 {
		return nil
	}

	ids := values(batch)

	p.l.Logf("[TRACE] processing %d blocks, latest is %d", len(batch), ids[0])
	start := time.Now()

	failed, err := p.processBlocks(ctx, consumerID, ids)
	if err != nil {
		return err
	}

	elapsed := time.Since(start).Milliseconds()
	// todo metrics
	p.l.Logf("[TRACE] batch of %d blocks is processed in %dms, ACK'ing transactions", len(batch), elapsed)

	if len(failed) > 0 {
		if err := p.redis.AddBlocks(ctx, failed); err != nil {
			return fmt.Errorf("adding failed blocks: %w", err)
		}
	}

	if err := p.redis.AcknowledgeBlocks(ctx, keys(batch)); err != nil {
		return fmt.Errorf("aknowledge blocks: %w", err)
	}

	atomic.StoreUint64(&p.lastProcessedBlock, ids[len(ids)-1])
	atomic.AddUint64(&p.processedBlocksCount, uint64(len(ids)))

	return nil
}

// returns blocks that we failed to process
func (p *Processor) processBlocks(ctx context.Context, id string, ids []uint64) (failed []uint64, err error) {
	handleErr := func(err error) ([]uint64, error) {
		return nil, fmt.Errorf("process batch: %w", err)
	}

	now := time.Now()

	const retries = 4 // 5 attemps in total

	// batch rpc get transactions
	blocks, failedIdx, err := p.rpc.GetBlocks(ctx, retries, ids...)
	if err != nil {
		return handleErr(fmt.Errorf("get blocks: %w", err))
	}

	p.l.Logf("[TRACE] processing blocks %v on ID %s", ids, id)

	if len(failedIdx) != 0 {
		failed = make([]uint64, len(failedIdx))

		for i, f := range failedIdx {
			failed[i] = ids[f]
		}

		p.l.Logf("failed to fetch blocks. Adding them to the back of the queue: %v", failed)
	}

	p.l.Logf("[TRACE] got blocks from rpc in %dms", time.Since(now).Milliseconds())

	// classify
	for _, block := range blocks {
		for _, tx := range block.Transactions {
			// hello
			if err := p.process(ctx, tx, block.BlockTime); err != nil {
				return handleErr(err)
			}
		}
	}

	return failed, nil
}

func (p *Processor) process(ctx context.Context, tx cli.Tx, blockTime uint64) error {
	addTxs := p.findAddInst(tx)
	if len(addTxs) == 0 {
		return nil
	}

	relations := sliceMap(addTxs, func(tx addIx) graph.Relation {
		return graph.Relation{
			From:           tx.params.From,
			To:             tx.params.To,
			Provider:       tx.accounts[0].PubKey,
			ConnectedAt:    int64(blockTime),
			DisconnectedAt: nil,
			Extra:          tx.params.Extra,
		}
	})

	// save it
	p.l.Logf("New relation: %v", relations)
	if err := p.mongo.SaveRelations(ctx, relations); err != nil {
		return fmt.Errorf("save relations: %w", err)
	}

	return nil
}

var (
	graphProgramID = common.PublicKeyFromString("graph8zS8zjLVJHdiSvP7S9PP7hNJpnHdbnJLR81FMg")
)

type addRelationParams struct {
	From, To common.PublicKey
	Extra    []byte
}

type addIx struct {
	params   addRelationParams
	accounts []types.AccountMeta
}

func (p Processor) findAddInst(tx cli.Tx) []addIx {
	var results []addIx

	// combine all outer and inner instructions
	allInsts := tx.Insts
	for _, insts := range tx.InnerInsts {
		allInsts = append(allInsts, insts...)
	}

	for _, inst := range allInsts {
		if inst.ProgramID != graphProgramID {
			continue
		}

		if len(inst.Data) < 8 && !bytes.Equal(inst.Data[:8], graph.AddRelationInstructionDiscriminator[:]) {
			// not an add_relation instruction
			continue
		}

		var params addRelationParams

		if err := borsh.Deserialize(&params, inst.Data[8:]); err != nil {
			p.l.Logf("[WARN] parse add instruction: %v", err)
			continue
		}

		results = append(results, addIx{params, inst.Accounts})
	}

	return results

}
