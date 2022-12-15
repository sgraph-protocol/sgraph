package cli

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-pkgz/lgr"
	"github.com/mailru/easyjson"
	"github.com/mr-tron/base58"
	"github.com/portto/solana-go-sdk/common"
	"github.com/portto/solana-go-sdk/rpc"
	"github.com/portto/solana-go-sdk/types"
)

type RPC struct {
	l           lgr.L
	endpoint    string
	blockLoader *BlockLoader
}

func NewRpc(l lgr.L, endpoint string) RPC {
	rpc := RPC{
		l:        l,
		endpoint: endpoint,
	}

	rpc.blockLoader = NewBlockLoader(BlockLoaderConfig{
		Fetch:    rpc.fetchBlocks,
		Wait:     time.Millisecond * 10, // TODO
		MaxBatch: 10,                    // TODO
	})

	return rpc
}

func (r RPC) fetchBlocks(keys []uint64) ([]Block, []error) {
	handleErr := func(err error) ([]Block, []error) {
		return nil, []error{fmt.Errorf("batch fetch transactions: %w", err)}
	}

	calls := make([]call, len(keys))

	for i, block := range keys {
		calls[i] = call{
			"getBlock",
			[]any{
				block,
				map[string]any{
					"encoding":                       rpc.GetBlockConfigEncodingBase64,
					"commitment":                     rpc.CommitmentConfirmed,
					"maxSupportedTransactionVersion": 1,
				},
			},
		}
	}

	resp, err := r.batchRequest(context.Background(), calls...)
	if err != nil {
		return handleErr(err)
	}

	var response getBlockRpcResponses

	if err := easyjson.UnmarshalFromReader(resp, &response); err != nil {
		return handleErr(err)
	}

	if len(response) != len(keys) {
		return handleErr(fmt.Errorf("wtf, rpc returned more responses that requested: %v", resp))
	}

	results := make([]Block, len(keys))
	errors := make([]error, len(keys))

	for i, resp := range response {
		if resp.Error != nil {
			errors[i] = fmt.Errorf("rpc errored with message: %v", resp.Error)
			continue
		}

		block := resp.Result

		txs := make([]Tx, 0, len(block.Transactions))

		for _, btx := range block.Transactions {
			tx, include, err := TxFromBlockTransaction(r.l, btx)
			if err != nil {
				errors[i] = err
				continue
			}

			if include {
				txs = append(txs, tx)
			}
		}

		results[i] = Block{
			ParentSlot:   block.ParentSlot,
			BlockTime:    block.BlockTime,
			Blockhash:    block.Blockhash,
			Transactions: txs,
		}
	}

	return results, errors
}

//easyjson:skip
type TxMeta struct {
	BalanceChanges      map[common.PublicKey]int64
	TokenBalanceChanges map[common.PublicKey]TokenBalanceChange
	Logs                []string
}

//easyjson:skip
type TokenBalanceChange struct {
	Delta       int64
	Decimals    uint8
	Owner, Mint common.PublicKey
}

// Parsed tx
//
//easyjson:skip
type Tx struct {
	TxHash     string
	Meta       TxMeta
	Insts      []types.Instruction
	InnerInsts map[int][]types.Instruction
}

// Parsed block
//
//easyjson:skip
type Block struct {
	ParentSlot uint64
	BlockTime  uint64
	Blockhash  string

	Transactions []Tx
}

type generaResponse struct {
	JsonRpc string            `json:"jsonrpc"`
	ID      uint64            `json:"id"`
	Error   *rpc.JsonRpcError `json:"error,omitempty"`
}

//easyjson:json
type getBlockRpcResponses []getBlockRpcResponse

type getBlockRpcResponse struct {
	generaResponse
	Result getBlockResult `json:"result"`
}

type getBlockResult struct {
	BlockHeight uint64 `json:"blockHeight"`
	BlockTime   uint64 `json:"blockTime"`
	ParentSlot  uint64 `json:"parentSlot"`

	Blockhash         string `json:"blockhash"`
	PreviousBlockhash string `json:"previousBlockhash"`

	Transactions []BlockRawTransaction `json:"transactions"`
}

type BlockRawTransaction struct {
	Meta        txMeta    `json:"meta"`
	Transaction [2]string `json:"transaction"`
	Version     any       `json:"version"`
}

type txMeta struct {
	Err               any                               `json:"err"`
	Fee               uint64                            `json:"fee"`
	PreBalances       []int64                           `json:"preBalances"`
	PostBalances      []int64                           `json:"postBalances"`
	PreTokenBalances  []rpc.TransactionMetaTokenBalance `json:"preTokenBalances"`
	PostTokenBalances []rpc.TransactionMetaTokenBalance `json:"postTokenBalances"`
	LogMessages       []string                          `json:"logMessages"`
	InnerInstructions []metaInnerInstructions           `json:"innerInstructions"`
	LoadedAddresses   rpc.TransactionLoadedAddresses    `json:"loadedAddresses"`
}

type metaInnerInstructions struct {
	Index        uint64 `json:"index"`
	Instructions []struct {
		Accounts     []int  `json:"accounts"`
		Data         string `json:"data"`
		ProgramIDIdx int    `json:"programIdIndex"`
	} `json:"instructions"`
}

//easyjson:json
type getBlocksRpcResponses []getBlocksRpcResponse

type getBlocksRpcResponse struct {
	generaResponse
	Result []uint64 `json:"result"`
}

//easyjson:json
type getEpochInfoRpcResponses []getEpochInfoRpcResponse

type epochInfo struct {
	AbsoluteSlot uint64 `json:"absoluteSlot"`
	BlockHeight  uint64 `json:"blockHeight"`
}

type getEpochInfoRpcResponse struct {
	generaResponse
	Result epochInfo `json:"result"`
}

// GetBlocks will recursevily try to fetch specified block ids until 0 retries left
// todo use backoff
func (r RPC) GetBlocks(ctx context.Context, retries uint, blocksIds ...uint64) ([]Block, []int, error) {
	const backoffDuration = 150 * time.Millisecond

	failed := make(map[uint64]int, 0) // blockID -> idx in original

	blocksResp, errs := r.blockLoader.fetch(blocksIds)
	for i, err := range errs {
		if err != nil {
			r.l.Logf("[TRACE] failed to fetch block %d, will retry %d more times: %v", blocksIds[i], retries, err)

			failed[blocksIds[i]] = i
		}
	}

	// make sure we always return array of right size
	blocks := make([]Block, len(blocksIds))
	copy(blocks, blocksResp)

	if len(failed) == 0 {
		// fast path
		return blocks, []int{}, nil
	}

	if retries == 0 {
		r.l.Logf("[ERROR] failed to fetch %d blocks even after several retries: %v", len(failed), keys(failed))
		return blocks, values(failed), nil
	}

	// backoff path
	time.Sleep(backoffDuration)

	toRetryIds := keys(failed)

	missingBlocks, failedIdx, err := r.GetBlocks(ctx, retries-1, toRetryIds...)
	if err != nil {
		return blocks, failedIdx, err
	}

	// augment with missing
	for i, id := range toRetryIds {
		blocks[failed[id]] = missingBlocks[i]
	}

	return blocks, failedIdx, nil
}

// Retruns latest slot and total block count
func (r RPC) GetLatestBlock(ctx context.Context) (uint64, error) {
	resp, err := r.batchRequest(ctx, call{
		method: "getEpochInfo",
		params: []any{commitmentConfirmed},
	})
	if err != nil {
		return 0, fmt.Errorf("get latest block: %w", err)
	}

	var response getEpochInfoRpcResponses

	if err := easyjson.UnmarshalFromReader(resp, &response); err != nil {
		return 0, fmt.Errorf("unmarshal latest block %w", err)
	}

	res := response[0].Result

	return res.AbsoluteSlot, nil
}

type commitmentConfig struct {
	Commitment rpc.Commitment `json:"commitment"`
}

var commitmentConfirmed = commitmentConfig{rpc.CommitmentConfirmed}

func (r RPC) GetBlocksWithLimit(ctx context.Context, from, limit uint64) ([]uint64, error) {
	resp, err := r.batchRequest(ctx, call{
		method: "getBlocksWithLimit",
		params: []any{from, limit, commitmentConfirmed},
	})
	if err != nil {
		return nil, fmt.Errorf("get blocks: %w", err)
	}

	var response getBlocksRpcResponses

	if err := easyjson.UnmarshalFromReader(resp, &response); err != nil {
		return nil, fmt.Errorf("unmarshal blockWithLimit block %w", err)
	}

	return response[0].Result, nil
}

type call struct {
	method string
	params []any
}

// will return body of response. if http code beyond 200~300, the error also returns.
func (c RPC) batchRequest(ctx context.Context, calls ...call) (io.ReadCloser, error) {
	// prepare payload
	type msg struct {
		JsonRPC string        `json:"jsonrpc"`
		ID      uint64        `json:"id"`
		Method  string        `json:"method"`
		Params  []interface{} `json:"params,omitempty"`
	}

	payload := make([]msg, len(calls))

	for i, call := range calls {
		payload[i] = msg{
			JsonRPC: "2.0",
			ID:      1,
			Method:  call.method,
			Params:  call.params,
		}
	}

	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	// prepare request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewBuffer(rawPayload))
	if err != nil {
		return nil, fmt.Errorf("failed to do http.NewRequestWithContext, err: %v", err)
	}
	req.Header.Add("Content-Type", "application/json")

	// do request
	httpclient := &http.Client{}
	res, err := httpclient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to do request, err: %w", err)
	}

	// check response code
	if res.StatusCode < 200 || res.StatusCode > 300 {
		return res.Body, fmt.Errorf("get status code: %v", res.StatusCode)
	}

	return res.Body, nil
}

// TxFromBlockTransaction parses raw rpc transaction to appropriate form for parsing
// todo: exposed for tests. parsing can happen in different entity. fine for now
func TxFromBlockTransaction(l lgr.L, blockTx BlockRawTransaction) (transaction Tx, include bool, err error) {
	handleErr := func(err error) (Tx, bool, error) {
		return Tx{}, false, fmt.Errorf("parse tx: %w", err)
	}

	if v, ok := blockTx.Version.(string); blockTx.Version != nil && (!ok || v != "legacy") {
		l.Logf("[WARN] skipping transaction v%v", blockTx.Version)
		return Tx{}, false, nil
	}

	rawTx, err := base64.StdEncoding.DecodeString(blockTx.Transaction[0])
	if err != nil {
		return handleErr(err)
	}

	tx, err := types.TransactionDeserialize(rawTx)
	if err != nil {
		return handleErr(fmt.Errorf("deserializing transaction: %w", err))
	}

	// pretty sure that can error
	insts := tx.Message.DecompileInstructions()

	if len(insts) > 0 && insts[0].ProgramID == common.VoteProgramID {
		return Tx{}, false, nil
	}

	meta := blockTx.Meta

	if meta.Err != nil {
		return Tx{}, false, nil
	}

	var txHash string
	if len(tx.Signatures) > 0 {
		txHash = base58.Encode(tx.Signatures[0])
	}

	inner := decompileInnerInstructions(meta, tx)

	balanceChanges := parseSolChange(meta, tx.Message.Accounts)

	tokenBalanceChanges, err := parseTokenChange(l, meta, tx.Message.Accounts)
	if err != nil {
		return handleErr(fmt.Errorf("parse token balance changes: %w", err))
	}

	return Tx{
		TxHash: txHash,
		Meta: TxMeta{
			BalanceChanges:      balanceChanges,
			TokenBalanceChanges: tokenBalanceChanges,
			Logs:                blockTx.Meta.LogMessages,
		},
		Insts:      insts,
		InnerInsts: inner,
	}, true, nil
}

func decompileInnerInstructions(meta txMeta, tx types.Transaction) map[int][]types.Instruction {
	result := make(map[int][]types.Instruction, len(meta.InnerInstructions))

	for _, inner := range meta.InnerInstructions {
		result[int(inner.Index)] = parseIx(tx.Message, inner)
	}

	return result
}

// thanks github.com/portto/solana-go-sdk
func parseIx(m types.Message, inner metaInnerInstructions) []types.Instruction {
	instructions := make([]types.Instruction, 0, len(inner.Instructions))
	for _, cins := range inner.Instructions {
		accounts := make([]types.AccountMeta, 0, len(cins.Accounts))
		for i := 0; i < len(cins.Accounts); i++ {
			accounts = append(accounts, types.AccountMeta{
				PubKey:   m.Accounts[cins.Accounts[i]],
				IsSigner: cins.Accounts[i] < int(m.Header.NumRequireSignatures),
				IsWritable: cins.Accounts[i] < int(m.Header.NumRequireSignatures-m.Header.NumReadonlySignedAccounts) ||
					(cins.Accounts[i] >= int(m.Header.NumRequireSignatures) &&
						cins.Accounts[i] < len(m.Accounts)-int(m.Header.NumReadonlyUnsignedAccounts)),
			})
		}
		data, _ := base58.Decode(cins.Data)
		instructions = append(instructions, types.Instruction{
			ProgramID: m.Accounts[cins.ProgramIDIdx],
			Accounts:  accounts,
			Data:      data,
		})
	}
	return instructions
}

func parseSolChange(meta txMeta, accounts []common.PublicKey) map[common.PublicKey]int64 {
	balanceChanges := make(map[common.PublicKey]int64, len(meta.PostBalances))
	for i, post := range meta.PostBalances {
		account := accounts[i]
		pre := meta.PreBalances[i]

		balanceChanges[account] = post - pre
	}

	return balanceChanges
}

func parseTokenChange(l lgr.L, meta txMeta, accounts []common.PublicKey) (map[common.PublicKey]TokenBalanceChange, error) {
	tokenBalanceChanges := make(map[common.PublicKey]TokenBalanceChange, len(meta.PostTokenBalances))

	for _, postChange := range meta.PostTokenBalances {
		account := accounts[postChange.AccountIndex]

		owner, err := base58.Decode(postChange.Owner)
		if err != nil {
			return nil, fmt.Errorf("decode owner: %w", err)
		}

		mint, err := base58.Decode(postChange.Mint)
		if err != nil {
			return nil, fmt.Errorf("decode mint: %w", err)
		}

		change := TokenBalanceChange{
			Owner:    toCommonPub(owner),
			Mint:     toCommonPub(mint),
			Decimals: postChange.UITokenAmount.Decimals,
		}

		postAmount, err := strconv.ParseUint(postChange.UITokenAmount.Amount, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse post UITokenAmount: %w", err)
		}

		preIdx := find(meta.PreTokenBalances, func(t rpc.TransactionMetaTokenBalance) bool {
			return t.AccountIndex == postChange.AccountIndex
		})

		if preIdx == -1 {
			// wallet was just created. Post balance is the starting balance
			change.Delta = int64(postAmount)
			tokenBalanceChanges[account] = change
			continue
		}

		pre := meta.PreTokenBalances[preIdx]
		if pre.Owner != postChange.Owner {
			//l.Logf("[TRACE] unhandled case: owner has changed")
		}
		preAmount, err := strconv.ParseUint(pre.UITokenAmount.Amount, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse pre UITokenAmount: %w", err)
		}

		// I had 1000 and sent 300
		// 700 (post) - 1000 (pre) = -300 (delta)
		change.Delta = int64(postAmount) - int64(preAmount)
		tokenBalanceChanges[account] = change
	}

	return tokenBalanceChanges, nil
}

// find first match in array. returns -1 if not found
func find[T any](in []T, predicate func(T) bool) int {
	for i, elem := range in {
		if predicate(elem) {
			return i
		}
	}

	return -1
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
