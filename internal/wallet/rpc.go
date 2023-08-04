// 公链 JSON-RPC：查余额、广播、看回执。
// 作者: Likeo
// 日期: 2020.11.19
package wallet

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/metrics"
	"github.com/btcsuite/btcd/btcutil/base58"
	"github.com/shopspring/decimal"
)

type TokenReader interface {
	TokenAssets() []string
	TokenBalance(ctx context.Context, address, asset string) (decimal.Decimal, error)
}

type rpcAdapter struct {
	chain string
	asset string
	urls   []string
	tokens map[string]config.TokenSpec
	indexer *indexerClient
	client *http.Client
	mu     sync.Mutex
	nonces map[string]string
}

   func newRPCAdapter(chain, asset string, urls []string, timeout time.Duration, tokens map[string]config.TokenSpec, indexer *indexerClient) *rpcAdapter {
	  if timeout <= 0 {
		  timeout = 180 * time.Millisecond
	  }
	   return &rpcAdapter{
		  chain: chain,
		  asset:  asset,
		  urls:   urls,
		  tokens: tokens,
		  indexer: indexer,
		  client:  &http.Client{Timeout: timeout},
		  nonces:  map[string]string{},
	   }
   }

   func (a *rpcAdapter) ChainID() string     { return a.chain }
   func (a *rpcAdapter) NativeAsset() string { return a.asset }

  func (a *rpcAdapter) GetBalance(ctx context.Context, address string) (decimal.Decimal, error) {
	  raw, err := a.call(ctx, a.balanceMethod(), a.balanceParams(address))
	  if err != nil {
		   return decimal.Zero, err
	  }
	   return parseRPCAmount(a.chain, raw)
  }

  func (a *rpcAdapter) GetTxHistory(ctx context.Context, address string, limit int) ([]Tx, error) {
	  if limit <= 0 {
		  limit = 20
	  }
	   if a.indexer == nil {
		   return nil, fmt.Errorf("tx history requires indexer for %s", a.chain)
	  }
	  return a.indexer.History(ctx, a.chain, address, limit)
   }

  func (a *rpcAdapter) TokenAssets() []string {
	   specs := a.tokenSpecs()
	   out := make([]string, 0, len(specs))
	  for asset := range specs {
		   out = append(out, asset)
	  }
	  return out
   }

  func (a *rpcAdapter) TokenBalance(ctx context.Context, address, asset string) (decimal.Decimal, error) {
	  specs := a.tokenSpecs()
	  spec, ok := specs[strings.ToUpper(strings.TrimSpace(asset))]
	  if !ok || strings.TrimSpace(spec.Contract) == "" {
		  return decimal.Zero, fmt.Errorf("token %s not configured on %s", asset, a.chain)
	  }
	  contract, err := contractCallAddress(a.chain, spec.Contract)
	  if err != nil {
		   return decimal.Zero, err
	  }
	  data, err := balanceOfData(address)
	   if err != nil {
		  return decimal.Zero, err
	   }
	  raw, err := a.call(ctx, "eth_call", []any{
		   map[string]string{"to": contract, "data": data},
		   "latest",
	  })
	   if err != nil {
		   return decimal.Zero, err
	   }
	   dec := spec.Decimals
	   if dec <= 0 {
		  dec = 6
	  }
	  return hexToDecimalUnits(rawString(raw), int32(dec))
  }

   func (a *rpcAdapter) SubmitTransfer(ctx context.Context, from, to string, amount decimal.Decimal) (string, error) {
	   if nonce := SubmitNonceFrom(ctx); nonce != "" {
		   if h := a.lookupNonce(nonce); h != "" {
			  metrics.SubmitDeduped.Inc()
			  return h, nil
		   }
	   }
	  raw, err := a.call(ctx, a.submitMethod(), a.submitParams(ctx, from, to, amount))
	   if err != nil {
		   return "", err
	  }
	  var hash string
	  if err := json.Unmarshal(raw, &hash); err != nil {
		   return "", fmt.Errorf("rpc submit result: %w", err)
	   }
	  if nonce := SubmitNonceFrom(ctx); nonce != "" {
		   a.rememberNonce(nonce, hash)
	  }
	  return hash, nil
   }

  func (a *rpcAdapter) lookupNonce(nonce string) string {
	   a.mu.Lock()
	   defer a.mu.Unlock()
	  return a.nonces[nonce]
   }

   func (a *rpcAdapter) rememberNonce(nonce, hash string) {
	  if nonce == "" || hash == "" {
		  return
	   }
	  a.mu.Lock()
	   defer a.mu.Unlock()
	   if a.nonces == nil {
		   a.nonces = map[string]string{}
	   }
	   if _, ok := a.nonces[nonce]; !ok {
		   a.nonces[nonce] = hash
	  }
   }

   func (a *rpcAdapter) GetTxStatus(ctx context.Context, txHash string) (TxStatus, error) {
	  raw,
		   err := a.call(ctx, a.statusMethod(), a.statusParams(txHash))
	   if err != nil {
		   return "", err
	   }
	   if string(raw) == "null" || len(raw) == 0 {
		  return TxPending, nil
	  }
	  var obj map[string]any
	   if json.Unmarshal(raw, &obj) != nil {
		   return TxPending, nil
	   }
	  if obj["blockHash"] != nil || obj["blockNumber"] != nil || obj["confirmations"] != nil {
		  return TxConfirmed, nil
	   }
	   if s, ok := obj["status"].(string); ok && (s == "failed" || s == "0x0") {
		  return TxFailed, nil
	   }
	  return TxPending, nil
   }

  func (a *rpcAdapter) ObserveInbound(ctx context.Context, hash, to string, amount decimal.Decimal) (string, error) {
	  if strings.TrimSpace(hash) == "" {
		   return "", fmt.Errorf("observe hash required")
	  }
	  st, err := a.GetTxStatus(ctx, hash)
	  if err != nil {
		  return "", err
	   }
	  if st == TxFailed {
		  return "", fmt.Errorf("inbound %s failed on %s", hash, a.chain)
	  }
	   _ = to
	   _ = amount
	  return hash, nil
  }

   func (a *rpcAdapter) Ping(ctx context.Context) (time.Duration, error) {
	  start := time.Now()
	   _, err := a.call(ctx, a.pingMethod(), []any{})
	   return time.Since(start), err
  }

  func (a *rpcAdapter) InspectTx(ctx context.Context, txHash string) (Observation, error) {
	  st,
		  err := a.GetTxStatus(ctx, txHash)
	   if err != nil {
		  return Observation{}, err
	  }
	  confs := 0
	  if st == TxConfirmed {
		  confs = 1
	   }
	   obs := Observation{Hash: txHash, Asset: a.asset, Confirmations: confs, Reorged: false, Status: st}
	   from, to, mt := a.enrichFromChain(ctx, txHash)
	   if from != "" {
		   obs.FromAddress = from
	  }
	   if to != "" {
		  obs.ToAddress = to
	  }
	  if mt.Memo != "" {
		  obs.Memo = mt.Memo
	  }
	  if mt.Tag != "" {
		   obs.Tag = mt.Tag
	   }
	   if mt.TagName != "" {
		   obs.TagName = mt.TagName
	   } else if tn := FamMemoTagName(a.chain); tn != "" {
		  obs.TagName = tn
	   }
	   a.applyBookedTransfer(ctx, &obs)
	   return obs, nil
  }

func (a *rpcAdapter) applyBookedTransfer(ctx context.Context, obs *Observation) {
	if obs == nil || !IsEVM(a.chain) {
		return
	}
	specs := a.tokenSpecs()
	if len(specs) == 0 {
		return
	}
	raw, err := a.call(ctx, "eth_getTransactionReceipt", []any{obs.Hash})
	if err != nil || len(raw) == 0 || string(raw) == "null" {
		return
	}
	rec, err := DecodeEthReceiptJSON(raw)
	if err != nil {
		return
	}
	asset, amt, to, ok := MatchBookedTransfer(rec.Logs, specs)
	if !ok {
		return
	}
	obs.Asset = asset
	obs.Amount = amt
	if to != "" {
		obs.ToAddress = to
	}
}

func (a *rpcAdapter) nativeContract() string {
	spec, ok := a.tokenSpecs()[strings.ToUpper(strings.TrimSpace(a.asset))]
	if !ok {
		return ""
	}
	return spec.Contract
}

func (a *rpcAdapter) tokenSpecs() map[string]config.TokenSpec {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make(map[string]config.TokenSpec, len(a.tokens))
	for k, v := range a.tokens {
		out[k] = v
	}
	return out
}

func (a *rpcAdapter) MergeToken(asset string, spec config.TokenSpec) {
	asset = strings.ToUpper(strings.TrimSpace(asset))
	if asset == "" || strings.TrimSpace(spec.Contract) == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	next := make(map[string]config.TokenSpec, len(a.tokens)+1)
	for k, v := range a.tokens {
		next[k] = v
	}
	next[asset] = spec
	a.tokens = next
}

// enrichFromChain 交易体 from + 回执 Transfer/UTXO/SOL/TRON/TON/XRP/XLM/ATOM/EOS 回填付款方与备注标签。
// ETH 族 ERC20；BTC UTXO；SOL getTransaction；TRON 原生+TRC20；TON source+Jetton/memo；XRP Payment+DestinationTag；XLM/ATOM/EOS FamMEMO memo。
// 解析失败返回空，不得伪造付款方或备注。
   func (a *rpcAdapter) enrichFromChain(ctx context.Context, txHash string) (from, to string, mt MemoTag) {
	  chain := strings.ToUpper(strings.TrimSpace(a.chain))
	  switch chain {
	   case "BTC":
		  from, to = a.enrichFromBTC(ctx, txHash)
		  return from, to, MemoTag{}
	  case "SOL":
		   from, to = a.enrichFromSOL(ctx, txHash)
		   return from, to, MemoTag{}
	  case "TRON":
		  from,
			   to = a.enrichFromTRON(ctx, txHash)
		  return from, to, MemoTag{}
	  case "TON":
		  from, to, mt = a.enrichFromTONMemo(ctx, txHash)
		  return from, to, mt
	  case "XRP":
		  return a.enrichFromXRP(ctx, txHash)
	   case "XLM":
		   return a.enrichFromXLM(ctx,
			   txHash)
	   case "ATOM":
		  return a.enrichFromATOM(ctx, txHash)
	  case "EOS":
		  return a.enrichFromEOS(ctx, txHash)
	   case "ETH", "BSC", "ARB", "OP", "MATIC", "POLYGON", "AVAX", "BASE":
	   default:
		   return "", "", MemoTag{}
	  }
	  nativeFrom, nativeTo := "", ""
	  if raw, err := a.call(ctx, "eth_getTransactionByHash", []any{txHash}); err == nil && len(raw) > 0 && string(raw) != "null" {
		  var tx struct {
			   From string `json:"from"`
			  To string `json:"to"`
		  }
		  if json.Unmarshal(raw, &tx) == nil {
			  nativeFrom = strings.TrimSpace(tx.From)
			   nativeTo = strings.TrimSpace(tx.To)
		  }
	   }
	   contract := a.nativeContract()
	  if raw, err := a.call(ctx, "eth_getTransactionReceipt", []any{txHash}); err == nil && len(raw) > 0 && string(raw) != "null" {
		   if rec, err := DecodeEthReceiptJSON(raw); err == nil {
			  from = EnrichFromReceipt(nativeFrom, rec, contract)
			   if to == "" {
				   to = nativeTo
			  }
			   if to == "" {
				  to = strings.TrimSpace(rec.To)
			   }
			   return from, to, MemoTag{}
		  }
	  }
	  return nativeFrom,
		  nativeTo, MemoTag{}
  }

// enrichFromSOL getTransaction 回填 SOL/SPL 付款方。
// encoding json + maxSupportedTransactionVersion；SPL mint 过滤。
// 节点无交易或解析失败返回空。

  func (a *rpcAdapter) enrichFromSOL(ctx context.Context, txHash string) (from, to string) {
	   params := []any{txHash, map[string]any{"encoding": "json", "maxSupportedTransactionVersion": 0}}
	  raw, err := a.call(ctx, "getTransaction", params)
	   if err != nil || len(raw) == 0 || string(raw) == "null" {
		  return "", ""
	   }
	   tx, err := DecodeSolTxJSON(raw)
	   if err != nil {
		  return "", ""
	  }
	   mint := a.nativeContract()
	   from = EnrichFromSOL("", tx, mint)
	  to = SolToAddress(tx)
	  return from, to
  }

// enrichFromTRON 原生 TRON + 可选 eth 兼容层回填付款方。
// 先 wallet/gettransactionbyid；再 gettransactioninfobyid TRC20；最后回落 eth_*。
// 全路径失败返回空，不得用合约地址冒充。
   func (a *rpcAdapter) enrichFromTRON(ctx context.Context, txHash string) (from, to string) {
	   contract := a.nativeContract()
	   var native TronNativeTx
	  if raw, err := a.call(ctx, "wallet/gettransactionbyid", []any{map[string]string{"value": txHash}}); err == nil && len(raw) > 0 && string(raw) != "null" {
		  if tx, err := DecodeTronNativeTxJSON(raw); err == nil {
			  native = tx
		  }
	   }
	   if native.TxID == "" {
		   if raw, err := a.call(ctx, "gettransactionbyid", []any{txHash}); err == nil && len(raw) > 0 && string(raw) != "null" {
			   if tx, err := DecodeTronNativeTxJSON(raw); err == nil {
				  native = tx
			   }
		   }
	  }
	  var info *TronTxInfo
	   if raw, err := a.call(ctx, "wallet/gettransactioninfobyid", []any{map[string]string{"value": txHash}}); err == nil && len(raw) > 0 && string(raw) != "null" {
		   if got, err := DecodeTronTxInfoJSON(raw); err == nil {
			  info = &got
		   }
	   }
	   from = EnrichFromTRON("", native, info, contract)
	  to = TronToAddress(native)
	  if from != "" {
		   return from, to
	   }
	   // 回落 eth 兼容 JSON-RPC（部分网关）
	  nativeFrom, nativeTo := "", ""
	   if raw, err := a.call(ctx, "eth_getTransactionByHash", []any{txHash}); err == nil && len(raw) > 0 && string(raw) != "null" {
		   var tx struct {
			   From string `json:"from"`
			   To   string `json:"to"`
		   }
		   if json.Unmarshal(raw, &tx) == nil {
			  nativeFrom = strings.TrimSpace(tx.From)
			  nativeTo = strings.TrimSpace(tx.To)
		  }
	   }
	   if to == "" {
		   to = nativeTo
	   }
	  if raw, err := a.call(ctx, "eth_getTransactionReceipt", []any{txHash}); err == nil && len(raw) > 0 && string(raw) != "null" {
		   if rec, err := DecodeEthReceiptJSON(raw); err == nil {
			   from = EnrichFromReceipt(nativeFrom, rec, contract)
			  if to == "" {
				  to = strings.TrimSpace(rec.To)
			  }
			   return from, to
		  }
	   }
	   return nativeFrom, to
  }

// enrichFromBTC getrawtransaction verbose + UTXO vin 回填付款方。
// verbose=true；首个非 coinbase vin 地址；收款取首个 vout 地址。
// 缺 prevout/地址返回空，不得伪造。
   func (a *rpcAdapter) enrichFromBTC(ctx context.Context, txHash string) (from, to string) {
	  raw, err := a.call(ctx, "getrawtransaction", []any{txHash, true})
	  if err != nil || len(raw) == 0 || string(raw) == "null" {
		  return "", ""
	   }
	   tx, err := DecodeBtcTxVerboseJSON(raw)
	   if err != nil {
		  return "", ""
	   }
	   from = EnrichFromUTXO("", tx)
	  for _, vout := range tx.Vout {
		   if fa := firstAddr(vout.ScriptPubKey.Address, vout.ScriptPubKey.Addresses); fa != "" {
			  to = fa
			   break
		  }
	   }
	   return from, to
   }

// enrichFromXRP rippled tx 回填 Payment 付款方与 DestinationTag。
// method tx / tx_json；Account 为付款方；DestinationTag 入金归因。
// 非 Payment 或缺 Account/Tag 返回空，不得伪造经典地址或标签。

  func (a *rpcAdapter) enrichFromXRP(ctx context.Context, txHash string) (from, to string, mt MemoTag) {
	  txHash = strings.TrimSpace(txHash)
	   if txHash == "" {
		  return "", "", MemoTag{TagName: "destination_tag"}
	   }
	  try := func(method string, params any) (XrpTx, bool) {
		   raw, err := a.call(ctx, method, params)
		   if err != nil || len(raw) == 0 || string(raw) == "null" {
			  return XrpTx{}, false
		   }
		   tx, err := DecodeXrpTxJSON(raw)
		  if err != nil {
			  return XrpTx{}, false
		   }
		  return tx, true
	  }
	   var tx XrpTx
	   var ok bool
	  if tx, ok = try("tx", []any{txHash}); !ok {
		  tx, ok = try("tx", map[string]any{"transaction": txHash, "binary": false})
	   }
	   if !ok {
		  return "", "", MemoTag{TagName: "destination_tag"}
	  }
	  from = EnrichFromXRP("", tx)
	   to = XrpToAddress(tx)
	  return from, to, EnrichMemoTagXRP(tx)
   }

// enrichFromXLM Horizon/Stellar 交易回填付款方与 memo。
// transactions/{hash} 或 getTransaction；source_account；memo/memo_type。
// 缺 source_account/memo 返回空，不得伪造 G 地址或备注。
   func (a *rpcAdapter) enrichFromXLM(ctx context.Context, txHash string) (from, to string, mt MemoTag) {
	  txHash = strings.TrimSpace(txHash)
	   if txHash == "" {
		  return "", "", MemoTag{TagName: "memo"}
	  }
	   try := func(method string, params any) (XlmTx, bool) {
		  raw, err := a.call(ctx, method, params)
		  if err != nil || len(raw) == 0 || string(raw) == "null" {
			   return XlmTx{}, false
		  }
		   tx, err := DecodeXlmTxJSON(raw)
		  if err != nil {
			  return XlmTx{}, false
		   }
		   return tx, true
	  }
	   var tx XlmTx
	   var ok bool
	  if tx, ok = try("transactions", map[string]any{"hash": txHash, "id": txHash}); !ok {
		   if tx, ok = try("getTransaction", []any{txHash}); !ok {
			  tx, ok = try("tx", []any{txHash})
		   }
	  }
	  if !ok {
		  return "", "", MemoTag{TagName: "memo"}
	  }
	   from = EnrichFromXLM("", tx)
	  to = XlmToAddress(tx)
	   return from,
		   to, EnrichMemoTagXLM(tx)
  }

// enrichFromATOM Cosmos LCD/Tendermint 交易回填付款方与 memo。
// txs/{hash} / tx；MsgSend.from_address 或 fee.payer；body.memo。
// 缺 signer/memo 返回空，不得伪造 bech32 或备注。
   func (a *rpcAdapter) enrichFromATOM(ctx context.Context, txHash string) (from, to string, mt MemoTag) {
	   txHash = strings.TrimSpace(txHash)
	  if txHash == "" {
		   return "", "", MemoTag{TagName: "memo"}
	   }
	   try := func(method string, params any) (AtomTx, bool) {
		   raw, err := a.call(ctx, method, params)
		  if err != nil || len(raw) == 0 || string(raw) == "null" {
			   return AtomTx{}, false
		  }
		  tx, err := DecodeAtomTxJSON(raw)
		   if err != nil {
			  return AtomTx{}, false
		   }
		  return tx, true
	  }
	   var tx AtomTx
	   var ok bool
	   if tx, ok = try("txs", map[string]any{"hash": txHash}); !ok {
		   if tx, ok = try("tx", []any{txHash}); !ok {
			  tx, ok = try("getTx", []any{txHash})
		  }
	   }
	  if !ok {
		   return "", "", MemoTag{TagName: "memo"}
	  }
	  from = EnrichFromATOM("", tx)
	   return from, "", EnrichMemoTagATOM(tx)
  }

// enrichFromEOS EOS get_transaction 回填付款方与 memo。
// transfer data.from；否则 authorization.actor；data.memo 入金归因。
// 缺 from/actor/memo 返回空，不得伪造账户名或备注。

  func (a *rpcAdapter) enrichFromEOS(ctx context.Context, txHash string) (from, to string, mt MemoTag) {
	   txHash = strings.TrimSpace(txHash)
	  if txHash == "" {
		   return "", "", MemoTag{TagName: "memo"}
	   }
	  try := func(method string, params any) (EosTx, bool) {
		   raw, err := a.call(ctx, method, params)
		  if err != nil || len(raw) == 0 || string(raw) == "null" {
			  return EosTx{}, false
		   }
		  tx, err := DecodeEosTxJSON(raw)
		  if err != nil {
			   return EosTx{}, false
		  }
		   return tx, true
	  }
	  var tx EosTx
	  var ok bool
	   if tx, ok = try("get_transaction", map[string]any{"id": txHash}); !ok {
		   if tx, ok = try("history_get_transaction", map[string]any{"id": txHash}); !ok {
			  tx, ok = try("getTransaction", []any{txHash})
		   }
	  }
	  if !ok {
		  return "", "", MemoTag{TagName: "memo"}
	   }
	   from = EnrichFromEOS("", tx)
	   return from, "", EnrichMemoTagEOS(tx)
   }

// enrichFromTON toncenter 交易回填 TON/Jetton 付款方。
// getTransaction / getTransactions(hash) / tryLocateResultTx；in_msg.source；空则 Jetton notify/memo body 解码。
// 无地址仅靠哈希定位失败时返回空，不得伪造友好地址。
   func (a *rpcAdapter) enrichFromTON(ctx context.Context, txHash string) (from, to string) {
	   txHash = strings.TrimSpace(txHash)
	   if txHash == "" {
		  return "", ""
	   }
	   try := func(method string, params any) (TonTx, bool) {
		   raw, err := a.call(ctx, method, params)
		  if err != nil || len(raw) == 0 || string(raw) == "null" {
			   return TonTx{}, false
		  }
		  list, err := DecodeTonTxListJSON(raw)
		   if err != nil || len(list) == 0 {
			   if one, err2 := DecodeTonTxJSON(raw); err2 == nil {
				   return one, true
			  }
			   return TonTx{}, false
		  }
		   if got, ok := MatchTonTxByHash(list, txHash); ok {
			   return got, true
		   }
		  return list[0], true
	   }
	  var tx TonTx
	   var ok bool
	   if tx, ok = try("getTransaction", []any{txHash}); !ok {
		   if tx, ok = try("getTransactions", map[string]any{"hash": txHash, "limit": 1}); !ok {
			   tx, ok = try("tryLocateResultTx", map[string]any{"hash": txHash, "tx_hash": txHash})
		   }
	  }
	  if !ok {
		  return "", ""
	   }
	   from = EnrichFromTON("", tx)
	   to = TonToAddress(tx)
	   return from, to
  }

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params any `json:"params"`
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// enrichFromTONMemo TON 付款方 + comment/memo 备注同回填（FamMEMO 归因）。
// 复用 enrichFromTON；备注取 in_msg.message / decoded comment。
// 无 comment 时 TagName=comment 仍回填空 memo，不得伪造。

   func (a *rpcAdapter) enrichFromTONMemo(ctx context.Context, txHash string) (from, to string, mt MemoTag) {
	  from, to = a.enrichFromTON(ctx, txHash)
	   mt = MemoTag{TagName: "comment"}
	  txHash = strings.TrimSpace(txHash)
	   if txHash == "" {
		  return from, to, mt
	  }
	  try := func(method string, params any) {
		  raw, err := a.call(ctx, method, params)
		   if err != nil || len(raw) == 0 || string(raw) == "null" {
			   return
		  }
		  list,
			   err := DecodeTonTxListJSON(raw)
		   if err == nil && len(list) > 0 {
			   got, ok := MatchTonTxByHash(list, txHash)
			   if !ok {
				  got = list[0]
			  }
			  mt = EnrichMemoTagTON(got)
			  return
		   }
		  tx, err := DecodeTonTxJSON(raw)
		  if err != nil {
			  return
		  }
		  mt = EnrichMemoTagTON(tx)
	   }
	  try("getTransaction", []any{txHash})
	   if mt.Memo == "" {
		  try("getTransactions", map[string]any{"hash": txHash, "limit": 1})
	   }
	   if mt.Memo == "" {
		  try("tryLocateResultTx", map[string]any{"hash": txHash, "tx_hash": txHash})
	  }
	  return from, to, mt
   }

   func tonCommentOf(tx TonTx) string {
	   // comment 与 Jetton notify body 分立；复用 TonCommentOnly。
	  return TonCommentOnly(tx)
   }

  func (a *rpcAdapter) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	   urls := a.urls
	  if pref := RPCURLFrom(ctx); pref != "" {
		  urls = []string{pref}
	  }
	   if len(urls) == 0 {
		  return nil, fmt.Errorf("no rpc endpoint for %s", a.chain)
	   }
	   body, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: 1, Method: method, Params: params})
	   if err != nil {
		   return nil, err
	  }
	  var last error
	   for _, u := range urls {
		  req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
		  if err != nil {
			   last = err
			  continue
		  }
		   req.Header.Set("Content-Type", "application/json")
		   resp, err := a.client.Do(req)
		  if err != nil {
			  last = err
			   continue
		  }
		  raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		   _ = resp.Body.Close()
		   if resp.StatusCode >= 300 {
			   last = fmt.Errorf("rpc %s %s: status %d", a.chain, u, resp.StatusCode)
			   continue
		  }
		  var out rpcResponse
		  if err := json.Unmarshal(raw, &out); err != nil {
			   last = err
			  continue
		  }
		   if out.Error != nil {
			  last = fmt.Errorf("rpc %s %s: %s", a.chain, method, out.Error.Message)
			   continue
		   }
		   return out.Result, nil
	  }
	   if last == nil {
		   last = fmt.Errorf("rpc %s unreachable", a.chain)
	  }
	   return nil, last
  }

  func (a *rpcAdapter) balanceMethod() string {
	   switch a.chain {
	  case "BTC":
		  return "getreceivedbyaddress"
	   case "SOL":
		   return "getBalance"
	   case "TRON":
		   return "eth_getBalance"
	  case "TON":
		  return "getAddressBalance"
	   default:
		   return "eth_getBalance"
	  }
  }

  func (a *rpcAdapter) balanceParams(address string) any {
	  switch a.chain {
	   case "BTC":
		   return []any{address}
	   case "SOL":
		   return []any{address}
	  case "TON":
		   return []any{address}
	  default:
		   return []any{address, "latest"}
	   }
  }

  func (a *rpcAdapter) submitMethod() string {
	   switch a.chain {
	  case "BTC":
		   return "sendtoaddress"
	  case "SOL":
		   return "sendTransaction"
	   case "TON":
		   return "sendBoc"
	  default:
		   return "eth_sendTransaction"
	   }
   }

   func (a *rpcAdapter) submitParams(ctx context.Context, from, to string, amount decimal.Decimal) any {
	  wm := WithdrawMemoFrom(ctx)
	   destTag := wm.DestinationTagValue()
	   switch a.chain {
	  case "BTC":
		  return []any{to, amount.String()}
	  case "SOL", "TON":
		   return []any{map[string]string{"from": from, "to": to, "amount": amount.String(), "memo": destTag, "destination_tag": destTag, "tag_name": wm.TagName}}
	   default:
		   m := map[string]string{"from": from, "to": to, "value": evmWei(amount)}
		   if destTag != "" {
			   m["memo"] = destTag
			  m["destination_tag"] = destTag
			  m["tag_name"] = wm.TagName
		   }
		   return []any{m}
	  }
  }

   func (a *rpcAdapter) statusMethod() string {
	  if a.chain == "BTC" {
		  return "gettransaction"
	  }
	  if a.chain == "SOL" {
		  return "getSignatureStatuses"
	  }
	  return "eth_getTransactionReceipt"
  }

  func (a *rpcAdapter) statusParams(hash string) any {
	   if a.chain == "SOL" {
		   return []any{[]string{hash}}
	  }
	   return []any{hash}
  }

  func (a *rpcAdapter) pingMethod() string {
	  if a.chain == "BTC" {
		   return "getblockchaininfo"
	  }
	  if a.chain == "SOL" {
		  return "getHealth"
	   }
	   if a.chain == "TON" {
		   return "getMasterchainInfo"
	   }
	  return "eth_blockNumber"
  }

  func parseRPCAmount(chain string,
	  raw json.RawMessage) (decimal.Decimal, error) {
	  var s string
	  if json.Unmarshal(raw, &s) == nil && s != "" {
		  if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
			   return hexToDecimal(s, chain)
		  }
		  return decimal.NewFromString(s)
	  }
	  var n json.Number
	   if json.Unmarshal(raw, &n) == nil {
		  return decimal.NewFromString(n.String())
	  }
	   var wrap struct {
		   Value json.RawMessage `json:"value"`
	  }
	  if json.Unmarshal(raw, &wrap) == nil && len(wrap.Value) > 0 {
		  return parseRPCAmount(chain, wrap.Value)
	   }
	   return decimal.Zero, nil
   }

   func hexToDecimal(s, chain string) (decimal.Decimal, error) {
	   dec := int32(0)
	  if IsEVM(chain) || chain == "ETH" {
		   dec = 18
	   }
	   return hexToDecimalUnits(s, dec)
   }

   func hexToDecimalUnits(s string, decimals int32) (decimal.Decimal, error) {
	   s = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(s), "0x"), "0X")
	  s = strings.Trim(s, "\"")
	  if s == "" || s == "null" {
		   return decimal.Zero, nil
	   }
	   n,
		  ok := new(big.Int).SetString(s, 16)
	   if !ok {
		  return decimal.Zero, fmt.Errorf("bad hex amount")
	   }
	   d := decimal.NewFromBigInt(n, 0)
	  if decimals > 0 {
		  return d.Div(decimal.New(1,
			   decimals)), nil
	   }
	   return d, nil
   }

  func rawString(raw json.RawMessage) string {
	   var s string
	   if json.Unmarshal(raw, &s) == nil {
		  return s
	  }
	  return strings.TrimSpace(string(raw))
   }

   func contractCallAddress(chain, contract string) (string, error) {
	   contract = strings.TrimSpace(contract)
	   if strings.HasPrefix(strings.ToLower(contract), "0x") && len(contract) == 42 {
		   return contract, nil
	  }
	  if chain == "TRON" && strings.HasPrefix(contract, "T") {
		   payload, _, err := base58.CheckDecode(contract)
		   if err != nil {
			  return "", fmt.Errorf("token contract: %w", err)
		  }
		   return "0x" + hex.EncodeToString(payload), nil
	  }
	  return "", fmt.Errorf("unsupported token contract")
  }

   func balanceOfData(address string) (string, error) {
	  addr20, err := addressPayload20(address)
	  if err != nil {
		  return "", err
	  }
	  return "0x70a08231000000000000000000000000" + hex.EncodeToString(addr20), nil
   }

  func addressPayload20(address string) ([]byte, error) {
	   address = strings.TrimSpace(address)
	   if strings.HasPrefix(strings.ToLower(address), "0x") && len(address) == 42 {
		   return hex.DecodeString(address[2:])
	   }
	   if strings.HasPrefix(address, "T") {
		   payload, _, err := base58.CheckDecode(address)
		  if err != nil {
			   return nil, err
		   }
		   return payload, nil
	  }
	  return nil, fmt.Errorf("token balance address")
  }

   func evmWei(amount decimal.Decimal) string {
	   wei := amount.Mul(decimal.New(1, 18)).BigInt()
	   if wei == nil || wei.Sign() == 0 {
		  return "0x0"
	  }
	   return "0x" + hex.EncodeToString(wei.Bytes())
   }
