// 内存链适配，供 OpenMem 与流程测试使用。
// 作者: Likeo
// 日期: 2021.01.17
package wallet

import (
	"context"
	"fmt"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/metrics"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// 空历史夹具哈希（0x + 64 hex）。InboundConfirmed 按精确匹配丢弃，不得记成入金。
const (
	emptyHistoryInHash = "0xa1b2c3d4e5f60718293a4b5c6d7e8f90123456789abcdef0123456789abcde01"
	emptyHistoryOutHash = "0xb2c3d4e5f60718293a4b5c6d7e8f90123456789abcdef0123456789abcde0202"
)

   func skipEmptyHistoryHash(h string) bool {
	  return h == emptyHistoryInHash || h == emptyHistoryOutHash
   }

type mockBlock struct {
	Height int64
	Hash string
	Txs []string
}

// mockAdapter 链 RPC：双端点、内存交易、按块确认深度、可 fork。
type mockAdapter struct {
	chain string
	asset string
	urls []string
	native decimal.Decimal
	mu  sync.Mutex
	txs map[string]Tx
	nonces map[string]string
	height int64
	blocks []mockBlock
	sigs     map[string][]byte
	autoMine bool
	seq int
	delay time.Duration
	failN  int
	failEP map[string]int
}

   func newMockAdapter(chain, asset string, urls []string, native decimal.Decimal) *mockAdapter {
	   return &mockAdapter{
		  chain: chain,
		  asset: asset,
		  urls:   urls,
		   native: native,
		  txs:    map[string]Tx{},
		   nonces: map[string]string{},
		  sigs:   map[string][]byte{},
		  failEP: map[string]int{},
		  autoMine: true,
	   }
   }

   func (m *mockAdapter) ChainID() string { return m.chain }

   func (m *mockAdapter) NativeAsset() string { return m.asset }

   func (m *mockAdapter) SetDelay(d time.Duration) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  m.delay = d
  }

   func (m *mockAdapter) FailNext(n int) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   m.failN = n
   }

   func (m *mockAdapter) FailEndpoint(url string, n int) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  m.failEP[url] = n
  }

  func (m *mockAdapter) SetAutoMine(v bool) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   m.autoMine = v
  }

  func (m *mockAdapter) Head() (int64, string) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if len(m.blocks) == 0 {
		  return m.height, ""
	  }
	  return m.height, m.blocks[len(m.blocks)-1].Hash
  }

  func (m *mockAdapter) wait(ctx context.Context) error {
	  m.mu.Lock()
	  d := m.delay
	  m.mu.Unlock()
	  if d <= 0 {
		  return ctx.Err()
	  }
	   t := time.NewTimer(d)
	   defer t.Stop()
	   select {
	   case <-ctx.Done():
		  return ctx.Err()
	   case <-t.C:
		  return ctx.Err()
	  }
   }

  func (m *mockAdapter) maybeFail() error {
	   return m.maybeFailURL("")
   }

   func (m *mockAdapter) maybeFailURL(url string) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   if m.failN > 0 {
		  m.failN--
		   return fmt.Errorf("mock rpc fail %s", m.chain)
	  }
	  if url != "" {
		   if n := m.failEP[url]; n > 0 {
			   m.failEP[url] = n - 1
			  return fmt.Errorf("mock rpc fail %s", url)
		  }
	  }
	   return nil
   }

  func (m *mockAdapter) TokenAssets() []string {
	  out := []string{}
	   for _, a := range AssetsOnChain(m.chain) {
		   if a != m.asset {
			   out = append(out, a)
		  }
	  }
	  return out
   }

   func (m *mockAdapter) TokenBalance(ctx context.Context, _, asset string) (decimal.Decimal, error) {
	   if err := m.wait(ctx); err != nil {
		   return decimal.Zero, err
	   }
	  if err := m.maybeFail(); err != nil {
		   return decimal.Zero, err
	  }
	  if asset == "USDT" {
		   return decimal.RequireFromString("1000"), nil
	   }
	   return decimal.Zero, nil
   }

  func (m *mockAdapter) GetBalance(ctx context.Context, _ string) (decimal.Decimal, error) {
	   if err := m.wait(ctx); err != nil {
		   return decimal.Zero, err
	  }
	  if err := m.maybeFail(); err != nil {
		   return decimal.Zero, err
	   }
	   return m.native, nil
  }

   func (m *mockAdapter) GetTxHistory(ctx context.Context, address string, limit int) ([]Tx, error) {
	   if err := m.wait(ctx); err != nil {
		  return nil, err
	  }
	  if err := m.maybeFail(); err != nil {
		   return nil, err
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  out := make([]Tx, 0, len(m.txs))
	  for _, t := range m.txs {
		   if address == "" || t.Address == address {
			  out = append(out, t)
		  }
	  }
	  if limit > 0 && len(out) > limit {
		   out = out[:limit]
	  }
	  if len(out) == 0 {
		   // 空历史夹具。InboundConfirmed 按固定哈希丢弃，不得记成入金。
		   out = []Tx{
			   {Chain: m.chain, Hash: emptyHistoryInHash, Asset: m.asset, Amount: decimal.RequireFromString("0.01"), Direction: "in", Status: TxConfirmed, Address: address, At: time.Now().Add(-2 * time.Hour)},
			  {Chain: m.chain, Hash: emptyHistoryOutHash, Asset: m.asset, Amount: decimal.RequireFromString("0.02"), Direction: "out", Status: TxConfirmed, Address: address, At: time.Now().Add(-time.Hour)},
		   }
	  }
	  return out, nil
  }

  func (m *mockAdapter) SubmitTransfer(ctx context.Context, _, to string, amount decimal.Decimal) (string, error) {
	  if err := m.wait(ctx); err != nil {
		   return "", err
	  }
	   if err := m.maybeFailURL(RPCURLFrom(ctx)); err != nil {
		  return "", err
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if nonce := SubmitNonceFrom(ctx); nonce != "" {
		   if h, ok := m.nonces[nonce]; ok {
			  metrics.SubmitDeduped.Inc()
			   return h, nil
		  }
	  }
	   m.seq++
	   wm := WithdrawMemoFrom(ctx)
	  destTag := wm.DestinationTagValue()
	   hash := fmt.Sprintf("0x%s%s", m.chain, uuid.NewString()[:16])
	  if destTag != "" {
		  // 广播哈希材料含 destination_tag，避免备注链与无备注撞 nonce 面。
		   hash = fmt.Sprintf("0x%s%s", m.chain, uuid.NewSHA1(uuid.NameSpaceOID, []byte(hash+"|"+destTag)).String()[:16])
	  }
	  m.txs[hash] = Tx{
		   Chain: m.chain, Hash: hash, Asset: m.asset, Amount: amount,
		  Direction: "out", Status: TxPending, Address: to, At: time.Now(),
	  }
	   if sig := WithdrawSigFrom(ctx); len(sig) > 0 {
		  m.sigs[hash] = append([]byte{}, sig...)
	  }
	  if nonce := SubmitNonceFrom(ctx); nonce != "" {
		   m.nonces[nonce] = hash
	   }
	   if m.autoMine {
		  m.mineLocked(hash)
	   }
	   return hash, nil
  }

  func (m *mockAdapter) ObserveInbound(ctx context.Context, hash, to string, amount decimal.Decimal) (string, error) {
	   if err := m.wait(ctx); err != nil {
		  return "", err
	  }
	   if err := m.maybeFailURL(RPCURLFrom(ctx)); err != nil {
		  return "", err
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if hash != "" {
		  if t, ok := m.txs[hash]; ok {
			   return t.Hash, nil
		   }
	  } else {
		  hash = fmt.Sprintf("0xdep%s%s", m.chain, uuid.NewString()[:16])
	   }
	  m.txs[hash] = Tx{
		   Chain: m.chain, Hash: hash, Asset: "", Amount: amount,
		  Direction: "in", Status: TxPending, Address: to, At: time.Now(),
	   }
	  if m.autoMine {
		   m.mineLocked(hash)
	   }
	   return hash, nil
   }

  func (m *mockAdapter) mineLocked(hashes ...string) int64 {
	   m.height++
	  bh := fmt.Sprintf("0xblk-%s-%d", m.chain, m.height)
	  inc := make([]string, 0, len(hashes))
	  for _, h := range hashes {
		  t, ok := m.txs[h]
		  if !ok || t.Status == TxFailed {
			   continue
		   }
		   t.Status = TxConfirmed
		   t.Height = m.height
		  t.BlockHash = bh
		   t.Reorged = false
		   m.txs[h] = t
		  inc = append(inc, h)
	   }
	  m.blocks = append(m.blocks, mockBlock{Height: m.height, Hash: bh, Txs: inc})
	   return m.height
  }

  func (m *mockAdapter) Mine(hashes ...string) int64 {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  return m.mineLocked(hashes...)
  }

  func (m *mockAdapter) Reorg(drop int) []string {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if drop <= 0 || len(m.blocks) == 0 {
		  return nil
	   }
	  if drop > len(m.blocks) {
		  drop = len(m.blocks)
	   }
	  cut := m.blocks[len(m.blocks)-drop:]
	   m.blocks = m.blocks[:len(m.blocks)-drop]
	   if len(m.blocks) == 0 {
		  m.height = 0
	  } else {
		   m.height = m.blocks[len(m.blocks)-1].Height
	  }
	   var dropped []string
	   for _, b := range cut {
		  for _, h := range b.Txs {
			   t, ok := m.txs[h]
			   if !ok {
				  continue
			   }
			   t.Status = TxPending
			   t.Height = 0
			  t.BlockHash = ""
			  t.Reorged = true
			   m.txs[h] = t
			  dropped = append(dropped, h)
		  }
	   }
	  return dropped
   }

   func (m *mockAdapter) SetObserved(hash, asset string, amount decimal.Decimal) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   t, ok := m.txs[hash]
	   if !ok {
		   t = Tx{Chain: m.chain, Hash: hash, Direction: "in", Status: TxPending, At: time.Now()}
	   }
	  if asset != "" {
		  t.Asset = asset
	   }
	   t.Amount = amount
	  m.txs[hash] = t
  }

   func (m *mockAdapter) InjectInbound(to, asset string, amount decimal.Decimal) string {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   m.seq++
	   hash := fmt.Sprintf("0xscan%s%s", m.chain, uuid.NewString()[:16])
	  m.txs[hash] = Tx{
		  Chain: m.chain, Hash: hash, Asset: asset, Amount: amount,
		   Direction: "in", Status: TxConfirmed, Address: to, At: time.Now(),
	  }
	  m.mineLocked(hash)
	  return hash
   }

  func (m *mockAdapter) inspectLocked(txHash string) Observation {
	  t, ok := m.txs[txHash]
	   if !ok {
		  return Observation{Hash: txHash,
			   Status: TxFailed}
	   }
	   obs := Observation{
		   Hash:  t.Hash,
		  Asset: t.Asset, Amount: t.Amount, ToAddress: t.Address, FromAddress: t.FromAddress,
		  Status: t.Status, Height: t.Height, BlockHash: t.BlockHash, Reorged: t.Reorged,
	  }
	   if t.Reorged || t.Height == 0 {
		   obs.Confirmations = 0
		   if t.Status != TxFailed {
			   obs.Status = TxPending
		  }
		   return obs
	  }
	   obs.Confirmations = int(m.height - t.Height + 1)
	  if obs.Confirmations < 0 {
		   obs.Confirmations = 0
	  }
	   if m.autoMine && t.Status == TxConfirmed && !t.Reorged && obs.Confirmations < 64 {
		   obs.Confirmations = 64
	  }
	   return obs
   }

   func (m *mockAdapter) InspectTx(ctx context.Context, txHash string) (Observation, error) {
	   if err := ctx.Err(); err != nil {
		  return Observation{}, err
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   if _, ok := m.txs[txHash]; !ok {
		  return Observation{Hash: txHash, Status: TxFailed}, fmt.Errorf("unknown tx %s", txHash)
	  }
	  t := m.txs[txHash]
	  if t.Status == TxPending && m.autoMine && t.Height == 0 && !t.Reorged {
		   m.mineLocked(txHash)
	  }
	   return m.inspectLocked(txHash),
		   nil
   }

  func (m *mockAdapter) InspectTxBatch(ctx context.Context, hashes []string) (map[string]Observation, error) {
	   if err := ctx.Err(); err != nil {
		  return nil, err
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   out := make(map[string]Observation, len(hashes))
	   for _, h := range hashes {
		  if h == "" {
			  continue
		  }
		  if _, ok := m.txs[h]; !ok {
			  continue
		   }
		   t := m.txs[h]
		  if t.Status == TxPending && m.autoMine && t.Height == 0 && !t.Reorged {
			  m.mineLocked(h)
		  }
		   out[h] = m.inspectLocked(h)
	   }
	   return out, nil
  }

  func (m *mockAdapter) GetTxStatus(ctx context.Context, txHash string) (TxStatus, error) {
	   obs, err := m.InspectTx(ctx, txHash)
	   if err != nil {
		   return obs.Status, err
	  }
	   return obs.Status, nil
  }

   func (m *mockAdapter) Ping(ctx context.Context) (time.Duration, error) {
	  start := time.Now()
	   if err := m.wait(ctx); err != nil {
		  return time.Since(start), err
	   }
	   if err := m.maybeFailURL(RPCURLFrom(ctx)); err != nil {
		   return time.Since(start), err
	  }
	  if m.delay == 0 {
		   time.Sleep(2 * time.Millisecond)
	   }
	  return time.Since(start), nil
  }

   func (m *mockAdapter) Confirm(txHash string) {
	   m.Mine(txHash)
  }

   func (m *mockAdapter) Signature(hash string) []byte {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  return append([]byte{}, m.sigs[hash]...)
   }
