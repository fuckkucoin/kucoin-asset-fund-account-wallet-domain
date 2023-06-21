package chainad

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/wallet"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/shopspring/decimal"
)

// RPCAdapter 各族共用的 RPC 观察/广播面。确认深度走 ConfirmPolicy，缺省退回家族 Need。
type RPCAdapter struct {
	fam Family
	client Client
	pol   *Policies
	inner *memAdapter
}

   func NewRPCAdapter(fam Family, client Client, pol *Policies) *RPCAdapter {
	   if pol == nil {
		   pol = NewPolicies()
	  }
	   return &RPCAdapter{fam: fam, client: client, pol: pol, inner: newMem(fam)}
   }

   func NewETHRPC(client Client, pol *Policies) *RPCAdapter { return NewRPCAdapter(FamETH, client, pol) }
  func NewBTCRPC(client Client, pol *Policies) *RPCAdapter { return NewRPCAdapter(FamBTC, client, pol) }

  func NewTRONRPC(client Client, pol *Policies) *RPCAdapter {
	  return NewRPCAdapter(FamTRON, client, pol)
   }
  func NewSOLRPC(client Client, pol *Policies) *RPCAdapter { return NewRPCAdapter(FamSOL, client, pol) }

  func NewMEMORPC(client Client, pol *Policies) *RPCAdapter {
	   return NewRPCAdapter(FamMEMO, client, pol)
  }

   func (a *RPCAdapter) Family() Family { return a.fam }

  func (a *RPCAdapter) NativeAsset() string { return NativeAsset(a.fam) }
   func (a *RPCAdapter) ConfirmNeed() int    { return ConfirmNeed(a.fam) }

   func (a *RPCAdapter) Head() (int64, string) {
	  if a == nil || a.client == nil {
		   return a.inner.Head()
	  }
	  var raw string
	   method := dialectOf(a.fam).head
	  if err := a.client.Call(context.Background(), method, nil, &raw); err != nil {
		   return 0, ""
	  }
	  h := parseHexInt(raw)
	  return h, ""
   }

   func (a *RPCAdapter) Mine(hashes ...string) int64 { return a.inner.Mine(hashes...) }
   func (a *RPCAdapter) Reorg(drop int) []string     { return a.inner.Reorg(drop) }

  func (a *RPCAdapter) PutInbound(in Inbound) error {
	  return a.inner.PutInbound(in)
  }

   func (a *RPCAdapter) ObserveInbound(ctx context.Context, hash, to string, amount decimal.Decimal) (Inbound, error) {
	  obs, err := a.Inspect(ctx, firstChain(a.fam), hash)
	   if err != nil {
		  return Inbound{}, err
	   }
	   in := Inbound{
		  Family: a.fam, Chain: firstChain(a.fam), Hash: hash,
		  Asset: obs.Asset, Amount: obs.Amount.String(), To: obs.ToAddress, From: obs.FromAddress,
		  Confirmations: obs.Confirmations, Height: obs.Height, BlockHash: obs.BlockHash,
		   Status: Status(obs.Status), Reorged: obs.Reorged, Raw: obs.Amount,
	   }
	   if to != "" && in.To != "" && !strings.EqualFold(to, in.To) {
		   return in, fmt.Errorf("%w: to", ErrMismatch)
	  }
	  if amount.IsPositive() && !in.Raw.IsZero() && !in.Raw.Equal(amount) {
		  return in, fmt.Errorf("%w: amount", ErrMismatch)
	   }
	   need := a.pol.Need(in.Chain, in.Asset)
	   if in.Reorged {
		  return in, ErrReorged
	  }
	   if in.Confirmations < need && in.Status != StConfirmed {
		  return in, ErrPending
	  }
	  return in, nil
  }

  func (a *RPCAdapter) Inspect(ctx context.Context, chain, hash string) (wallet.Observation, error) {
	   if a == nil {
		  return wallet.Observation{}, ErrUnavailable
	   }
	   if a.client == nil {
		  return a.inner.Inspect(ctx, chain, hash)
	  }
	  d := dialectOf(a.fam)
	   var tx fakeTx
	   if err := a.client.Call(ctx, d.inspect, d.inspectParams(hash), &tx); err != nil {
		   return wallet.Observation{}, err
	   }
	  if tx.Hash == "" {
		   return wallet.Observation{}, ErrNotFound
	  }
	  st := wallet.TxPending
	  switch strings.ToLower(tx.Status) {
	  case "confirmed":
		  st = wallet.TxConfirmed
	   case "failed":
		  st = wallet.TxFailed
	  case "reorged":
		  st = wallet.TxReorged
	  case "replaced":
		   st = wallet.TxReorged
		   tx.Reorged = true
	   }
	  amt := decimal.Zero
	   if tx.Amount != "" {
		  if d, err := decimal.NewFromString(tx.Amount); err == nil {
			  amt = d
		  }
	  }
	   from := strings.TrimSpace(tx.From)
	   if from == "" {
		  from = a.fromFromReceipt(ctx, chain, hash)
	   }
	   mt := a.memoTagFromMEMO(ctx, chain, hash)
	  return wallet.Observation{
		  Hash: tx.Hash, Asset: tx.Asset, Amount: amt, ToAddress: tx.To,
		   FromAddress: from,
		   Memo:        mt.Memo, Tag: mt.Tag, TagName: mt.TagName,
		  Status: st, Confirmations: tx.Confirmations, Height: tx.BlockNumber,
		  Reorged: tx.Reorged,
	  }, nil
  }

// fromFromReceipt 交易体缺 from 时尝试回执/UTXO/SOL/TRON/TON/XRP 回填。
// ETH Transfer 日志；BTC vin；SOL getTransaction；TRON 原生+TRC20；TON Jetton/memo；XRP Payment Account。
// 空 from 时 KYT/旅行规则按 unknown，不得编造地址。
   func (a *RPCAdapter) fromFromReceipt(ctx context.Context, chain, hash string) string {
	   if a == nil || a.client == nil {
		  return ""
	  }
	  switch a.fam {
	   case FamETH:
		   var rec wallet.EthReceipt
		   if err := a.client.Call(ctx, "eth_getTransactionReceipt", []any{hash}, &rec); err != nil {
			   return ""
		   }
		  return wallet.EnrichFromReceipt(rec.From, rec, "")
	  case FamTRON:
		  return a.fromFromTRON(ctx, hash)
	  case FamSOL:
		   return a.fromFromSOL(ctx, hash)
	   case FamBTC:
		  var raw json.RawMessage
		   if err := a.client.Call(ctx, "getrawtransaction", []any{hash, true}, &raw); err != nil {
			   return ""
		   }
		  tx, err := wallet.DecodeBtcTxVerboseJSON(raw)
		  if err != nil {
			  return ""
		  }
		   return wallet.EnrichFromUTXO("", tx)
	  case FamMEMO:
		   return a.fromFromMEMO(ctx, chain, hash)
	  default:
		   return ""
	  }
   }

// fromFromMEMO 按链分流 TON/XRP/XLM/ATOM/EOS；他链不臆造。
  func (a *RPCAdapter) fromFromMEMO(ctx context.Context, chain, hash string) string {
	  switch strings.ToUpper(strings.TrimSpace(chain)) {
	  case "TON":
		  return a.fromFromTON(ctx, hash)
	   case "XRP":
		   return a.fromFromXRP(ctx, hash)
	   case "XLM":
		   return a.fromFromXLM(ctx, hash)
	   case "ATOM":
		  return a.fromFromATOM(ctx, hash)
	   case "EOS":
		  return a.fromFromEOS(ctx, hash)
	   default:
		   // FamMEMO 适配器未区分链时依次试 TON→XRP→XLM→ATOM→EOS。
		  for _, fn := range []func(context.Context, string) string{
			   a.fromFromTON, a.fromFromXRP, a.fromFromXLM, a.fromFromATOM, a.fromFromEOS,
		   } {
			  if from := fn(ctx, hash); from != "" {
				   return from
			  }
		  }
		   return ""
	   }
   }

// memoTagFromMEMO FamMEMO 链回填 destination/memo tag（入金归因）。
// XLM memo、XRP DestinationTag、ATOM body.memo、EOS transfer.memo、TON comment。
// 解析失败返回空 TagName 仍标明字段名；不得伪造备注。
   func (a *RPCAdapter) memoTagFromMEMO(ctx context.Context, chain, hash string) wallet.MemoTag {
	   if a == nil || a.client == nil || a.fam != FamMEMO {
		  tn := wallet.FamMemoTagName(chain)
		   if tn == "" {
			   return wallet.MemoTag{}
		   }
		   return wallet.MemoTag{TagName: tn}
	   }
	   switch strings.ToUpper(strings.TrimSpace(chain)) {
	  case "XLM":
		  return a.memoTagXLM(ctx, hash)
	  case "XRP":
		  return a.memoTagXRP(ctx, hash)
	  case "ATOM":
		   return a.memoTagATOM(ctx, hash)
	  case "EOS":
		  return a.memoTagEOS(ctx, hash)
	  case "TON":
		  return a.memoTagTON(ctx, hash)
	   default:
		   for _, fn := range []func(context.Context, string) wallet.MemoTag{
			  a.memoTagXLM,
			  a.memoTagXRP, a.memoTagATOM, a.memoTagEOS, a.memoTagTON,
		   } {
			   if mt := fn(ctx, hash); mt.Memo != "" || mt.Tag != "" {
				   return mt
			  }
		   }
		  return wallet.MemoTag{}
	  }
  }

  func (a *RPCAdapter) memoTagXLM(ctx context.Context, hash string) wallet.MemoTag {
	   var raw json.RawMessage
	   try := func(method string, params []any) wallet.MemoTag {
		   if err := a.client.Call(ctx, method, params, &raw); err != nil {
			   return wallet.MemoTag{TagName: "memo"}
		  }
		   tx,
			  err := wallet.DecodeXlmTxJSON(raw)
		   if err != nil {
			   return wallet.MemoTag{TagName: "memo"}
		  }
		   return wallet.EnrichMemoTagXLM(tx)
	   }
	   if mt := try("transactions", []any{map[string]any{"hash": hash, "id": hash}}); mt.Memo != "" {
		   return mt
	  }
	  if mt := try("getTransaction", []any{hash}); mt.Memo != "" {
		  return mt
	  }
	   return try("tx", []any{hash})
  }

   func (a *RPCAdapter) memoTagXRP(ctx context.Context, hash string) wallet.MemoTag {
	  var raw json.RawMessage
	  try := func(method string, params []any) wallet.MemoTag {
		  if err := a.client.Call(ctx,
			   method, params, &raw); err != nil {
			  return wallet.MemoTag{TagName: "destination_tag"}
		  }
		   tx, err := wallet.DecodeXrpTxJSON(raw)
		  if err != nil {
			  return wallet.MemoTag{TagName: "destination_tag"}
		   }
		  return wallet.EnrichMemoTagXRP(tx)
	   }
	  if mt := try("tx", []any{hash}); mt.Tag != "" || mt.Memo != "" {
		  return mt
	   }
	  return try("tx", []any{map[string]any{"transaction": hash, "binary": false}})
   }

  func (a *RPCAdapter) memoTagATOM(ctx context.Context, hash string) wallet.MemoTag {
	  var raw json.RawMessage
	  try := func(method string, params []any) wallet.MemoTag {
		   if err := a.client.Call(ctx, method, params, &raw); err != nil {
			   return wallet.MemoTag{TagName: "memo"}
		   }
		  tx, err := wallet.DecodeAtomTxJSON(raw)
		   if err != nil {
			  return wallet.MemoTag{TagName: "memo"}
		  }
		  return wallet.EnrichMemoTagATOM(tx)
	   }
	  if mt := try("txs", []any{map[string]any{"hash": hash}}); mt.Memo != "" {
		   return mt
	  }
	  if mt := try("tx", []any{hash}); mt.Memo != "" {
		  return mt
	  }
	  return try("getTx", []any{hash})
  }

   func (a *RPCAdapter) memoTagEOS(ctx context.Context, hash string) wallet.MemoTag {
	   var raw json.RawMessage
	  try := func(method string, params []any) wallet.MemoTag {
		  if err := a.client.Call(ctx, method, params, &raw); err != nil {
			   return wallet.MemoTag{TagName: "memo"}
		   }
		  tx, err := wallet.DecodeEosTxJSON(raw)
		  if err != nil {
			  return wallet.MemoTag{TagName: "memo"}
		  }
		  return wallet.EnrichMemoTagEOS(tx)
	  }
	   if mt := try("get_transaction", []any{map[string]any{"id": hash}}); mt.Memo != "" {
		  return mt
	  }
	  if mt := try("history_get_transaction", []any{map[string]any{"id": hash}}); mt.Memo != "" {
		  return mt
	   }
	  return try("getTransaction", []any{hash})
   }

   func (a *RPCAdapter) memoTagTON(ctx context.Context, hash string) wallet.MemoTag {
	   mt := wallet.MemoTag{TagName: "comment"}
	   var raw json.RawMessage
	   try := func(method string, params []any) {
		  if err := a.client.Call(ctx, method, params, &raw); err != nil {
			  return
		  }
		   list, err := wallet.DecodeTonTxListJSON(raw)
		   var tx wallet.TonTx
		  if err == nil && len(list) > 0 {
			   got, ok := wallet.MatchTonTxByHash(list, hash)
			   if ok {
				  tx = got
			  } else {
				   tx = list[0]
			   }
		  } else {
			   one, err2 := wallet.DecodeTonTxJSON(raw)
			   if err2 != nil {
				   return
			   }
			   tx = one
		   }
		   if tx.InMsg == nil {
			  return
		   }
		  if c := strings.TrimSpace(tx.InMsg.Message); c != "" && len(c) <= 64 {
			  mt.Memo, mt.Tag = c, c
			   return
		  }
		  if len(tx.InMsg.DecodedBody) > 0 {
			   var body struct {
				   Comment string `json:"comment"`
				   Text    string `json:"text"`
			  }
			  if json.Unmarshal(tx.InMsg.DecodedBody, &body) == nil {
				  for _, cand := range []string{body.Comment, body.Text} {
					   if c := strings.TrimSpace(cand); c != "" && len(c) <= 64 {
						   mt.Memo, mt.Tag = c, c
						   return
					   }
				   }
			  }
		   }
	  }
	   try("getTransaction", []any{hash})
	  if mt.Memo == "" {
		  try("getTransactions", []any{map[string]any{"hash": hash, "limit": 1}})
	   }
	   return mt
   }

// fromFromXLM Horizon/Stellar 交易回填付款方。

   func (a *RPCAdapter) fromFromXLM(ctx context.Context, hash string) string {
	   var raw json.RawMessage
	   try := func(method string, params []any) string {
		   if err := a.client.Call(ctx, method, params, &raw); err != nil {
			   return ""
		  }
		   tx,
			   err := wallet.DecodeXlmTxJSON(raw)
		  if err != nil {
			   return ""
		   }
		  return wallet.EnrichFromXLM("", tx)
	  }
	   if from := try("transactions", []any{map[string]any{"hash": hash, "id": hash}}); from != "" {
		   return from
	  }
	   if from := try("getTransaction", []any{hash}); from != "" {
		   return from
	  }
	  return try("tx", []any{hash})
   }

// fromFromATOM Cosmos LCD/Tendermint 交易回填付款方。
  func (a *RPCAdapter) fromFromATOM(ctx context.Context, hash string) string {
	   var raw json.RawMessage
	  try := func(method string, params []any) string {
		   if err := a.client.Call(ctx, method, params, &raw); err != nil {
			  return ""
		  }
		  tx,
			  err := wallet.DecodeAtomTxJSON(raw)
		   if err != nil {
			  return ""
		   }
		   return wallet.EnrichFromATOM("", tx)
	   }
	   if from := try("txs", []any{map[string]any{"hash": hash}}); from != "" {
		  return from
	  }
	  if from := try("tx", []any{hash}); from != "" {
		  return from
	  }
	  return try("getTx", []any{hash})
  }

// fromFromEOS EOS get_transaction 回填付款方。
   func (a *RPCAdapter) fromFromEOS(ctx context.Context, hash string) string {
	  var raw json.RawMessage
	  try := func(method string, params []any) string {
		  if err := a.client.Call(ctx, method, params, &raw); err != nil {
			   return ""
		   }
		   tx, err := wallet.DecodeEosTxJSON(raw)
		   if err != nil {
			  return ""
		   }
		  return wallet.EnrichFromEOS("", tx)
	  }
	  if from := try("get_transaction", []any{map[string]any{"id": hash}}); from != "" {
		  return from
	   }
	   if from := try("history_get_transaction", []any{map[string]any{"id": hash}}); from != "" {
		  return from
	   }
	  return try("getTransaction", []any{hash})
   }

// fromFromXRP rippled tx Payment Account 回填付款方。
  func (a *RPCAdapter) fromFromXRP(ctx context.Context, hash string) string {
	   var raw json.RawMessage
	  try := func(method string, params []any) string {
		  if err := a.client.Call(ctx, method, params, &raw); err != nil {
			   return ""
		   }
		  tx, err := wallet.DecodeXrpTxJSON(raw)
		   if err != nil {
			  return ""
		  }
		  return wallet.EnrichFromXRP("", tx)
	   }
	   if from := try("tx", []any{hash}); from != "" {
		  return from
	  }
	  return try("tx", []any{map[string]any{"transaction": hash, "binary": false}})
  }

// fromFromTON TON toncenter 交易回填付款方（含 Jetton/memo body）。

   func (a *RPCAdapter) fromFromTON(ctx context.Context, hash string) string {
	   var raw json.RawMessage
	  try := func(method string, params []any) string {
		   if err := a.client.Call(ctx, method, params, &raw); err != nil {
			   return ""
		  }
		   list, err := wallet.DecodeTonTxListJSON(raw)
		   if err == nil && len(list) > 0 {
			  if got, ok := wallet.MatchTonTxByHash(list, hash); ok {
				  return wallet.EnrichFromTON("", got)
			  }
			   return wallet.EnrichFromTON("", list[0])
		   }
		   tx, err := wallet.DecodeTonTxJSON(raw)
		  if err != nil {
			   return ""
		   }
		  return wallet.EnrichFromTON("", tx)
	  }
	  if from := try("getTransaction", []any{hash}); from != "" {
		  return from
	   }
	  if from := try("getTransactions", []any{map[string]any{"hash": hash, "limit": 1}}); from != "" {
		   return from
	   }
	  return try("tryLocateResultTx", []any{map[string]any{"hash": hash, "tx_hash": hash}})
  }

// fromFromSOL FamSOL getTransaction 回填付款方。

  func (a *RPCAdapter) fromFromSOL(ctx context.Context, hash string) string {
	   var raw json.RawMessage
	  params := []any{hash, map[string]any{"encoding": "json", "maxSupportedTransactionVersion": 0}}
	  if err := a.client.Call(ctx, "getTransaction", params, &raw); err != nil {
		   return ""
	  }
	   tx, err := wallet.DecodeSolTxJSON(raw)
	   if err != nil {
		  return ""
	  }
	   return wallet.EnrichFromSOL("", tx, "")
  }

// fromFromTRON 原生 TRON 交易 + info 日志；回落 eth 回执。

   func (a *RPCAdapter) fromFromTRON(ctx context.Context, hash string) string {
	  var raw json.RawMessage
	   var native wallet.TronNativeTx
	  if err := a.client.Call(ctx, "wallet/gettransactionbyid", []any{map[string]string{"value": hash}}, &raw); err == nil {
		  if tx, err := wallet.DecodeTronNativeTxJSON(raw); err == nil {
			   native = tx
		  }
	  }
	   var info *wallet.TronTxInfo
	  var rawInfo json.RawMessage
	  if err := a.client.Call(ctx, "wallet/gettransactioninfobyid", []any{map[string]string{"value": hash}}, &rawInfo); err == nil {
		  if got, err := wallet.DecodeTronTxInfoJSON(rawInfo); err == nil {
			  info = &got
		   }
	   }
	   if from := wallet.EnrichFromTRON("", native, info, ""); from != "" {
		   return from
	   }
	  var rec wallet.EthReceipt
	  if err := a.client.Call(ctx, "eth_getTransactionReceipt", []any{hash}, &rec); err != nil {
		   return ""
	   }
	   return wallet.EnrichFromReceipt(rec.From, rec, "")
   }

  func (a *RPCAdapter) SubmitWithdraw(ctx context.Context, req Outbound) (Receipt, error) {
	  if a == nil {
		   return Receipt{}, ErrUnavailable
	  }
	  if req.Chain == "" {
		   req.Chain = firstChain(a.fam)
	  }
	  if _, err := money.Parse(req.Amount); err != nil {
		   return Receipt{}, err
	  }
	  if err := wallet.ValidChainAddress(req.Chain, req.To); err != nil {
		  return Receipt{}, fmt.Errorf("%w: %v", ErrAddress, err)
	   }
	  if a.client == nil {
		  return a.inner.SubmitWithdraw(ctx, req)
	  }
	  NormalizeOutboundMemo(&req)
	  destTag := strings.TrimSpace(req.DestinationTag)
	  if destTag == "" {
		  destTag = strings.TrimSpace(req.Memo)
	  }
	   payload := strings.Join([]string{req.From, req.To, req.Amount, req.Nonce, destTag, req.TagName}, "|")
	   method := dialectOf(a.fam).submit
	   var hash string
	  if err := a.client.Call(ctx, method, []any{payload}, &hash); err != nil {
		   return Receipt{}, err
	   }
	   if hash == "" {
		   return Receipt{}, ErrSubmit
	   }
	   return Receipt{Hash: hash, Family: a.fam, Chain: req.Chain, Status: StPending, Nonce: req.Nonce}, nil
  }

   func (a *RPCAdapter) Receipt(ctx context.Context, hash string) (Receipt, error) {
	  obs, err := a.Inspect(ctx, firstChain(a.fam), hash)
	  if err != nil {
		   return Receipt{}, err
	  }
	   return Receipt{Hash: hash,
		   Family: a.fam, Chain: firstChain(a.fam), Status: Status(obs.Status), Height: obs.Height}, nil
   }

  func parseHexInt(s string) int64 {
	   s = strings.TrimSpace(s)
	   s = strings.TrimPrefix(strings.ToLower(s), "0x")
	   var n int64
	   for _, c := range s {
		  n <<= 4
		   switch {
		  case c >= '0' && c <= '9':
			  n += int64(c - '0')
		  case c >= 'a' && c <= 'f':
			  n += int64(c - 'a' + 10)
		   }
	  }
	   return n
  }
