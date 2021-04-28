package chainad

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/wallet"
	"github.com/shopspring/decimal"
)

type memAdapter struct {
	fam      Family
	mu       sync.Mutex
	inbounds map[string]Inbound
	outs     map[string]Receipt
	nonces map[string]string
	blocks []Block
	height int64
	autoMine bool
	clock    func() time.Time
}

  func newMem(fam Family) *memAdapter {
	   return &memAdapter{
		  fam: fam, inbounds: map[string]Inbound{}, outs: map[string]Receipt{},
		  nonces: map[string]string{}, autoMine: true,
		  clock: func() time.Time { return time.Now().UTC() },
	   }
   }

   func (m *memAdapter) Family() Family { return m.fam }

  func (m *memAdapter) NativeAsset() string { return NativeAsset(m.fam) }
   func (m *memAdapter) ConfirmNeed() int    { return ConfirmNeed(m.fam) }

  func (m *memAdapter) Head() (int64, string) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if len(m.blocks) == 0 {
		   return m.height, ""
	   }
	  return m.height, m.blocks[len(m.blocks)-1].Hash
   }

   func (m *memAdapter) SetAutoMine(v bool) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   m.autoMine = v
  }

  func (m *memAdapter) PutInbound(in Inbound) error {
	  if m == nil {
		   return ErrUnavailable
	   }
	  if err := ValidHash(m.fam, in.Hash); err != nil {
		  return err
	  }
	  if _, err := money.Parse(in.Amount); err != nil {
		  return err
	  }
	   if in.To != "" {
		   if err := wallet.ValidChainAddress(firstChain(m.fam), in.To); err != nil {
			  return fmt.Errorf("%w: %v", ErrAddress, err)
		  }
	   }
	   if in.Asset == "" {
		  in.Asset = NativeAsset(m.fam)
	   }
	  if in.Chain == "" {
		  in.Chain = firstChain(m.fam)
	   }
	   if in.Family == "" {
		  in.Family = m.fam
	   }
	  if in.Status == "" {
		  in.Status = StPending
	  }
	  if in.SeenAt.IsZero() {
		   in.SeenAt = m.clock()
	  }
	  if d, err := decimal.NewFromString(in.Amount); err == nil {
		   in.Raw = d
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  m.inbounds[in.Hash] = in
	  if m.autoMine {
		   m.mineLocked(in.Hash)
	  }
	  return nil
  }

   func (m *memAdapter) ObserveInbound(ctx context.Context, hash, to string, amount decimal.Decimal) (Inbound, error) {
	  _ = ctx
	   if m == nil {
		  return Inbound{}, ErrUnavailable
	  }
	   if err := ValidHash(m.fam, hash); err != nil {
		  return Inbound{}, err
	  }
	   m.mu.Lock()
	  in, ok := m.inbounds[hash]
	   m.mu.Unlock()
	   if !ok {
		   return Inbound{}, ErrNotFound
	   }
	   if in.Reorged || in.Status == StReorged {
		  return in, ErrReorged
	   }
	   if in.Status == StFailed {
		  return in, ErrFailed
	   }
	  if to != "" && in.To != "" && !strings.EqualFold(to, in.To) {
		   return in, fmt.Errorf("%w: to", ErrMismatch)
	   }
	  if amount.IsPositive() && !in.Raw.IsZero() && !in.Raw.Equal(amount) {
		  return in, fmt.Errorf("%w: amount", ErrMismatch)
	  }
	   if in.Confirmations < m.ConfirmNeed() && in.Status != StConfirmed {
		  return in, ErrPending
	  }
	  return in, nil
  }

   func (m *memAdapter) Inspect(ctx context.Context, chain, hash string) (wallet.Observation, error) {
	  _ = ctx
	  if _, err := FamilyOf(chain); err != nil {
		  return wallet.Observation{}, err
	  }
	  m.mu.Lock()
	   in, ok := m.inbounds[hash]
	   m.mu.Unlock()
	   if !ok {
		  if rec, err := m.receiptLocked(hash); err == nil {
			  st := wallet.TxPending
			  if rec.Status == StConfirmed {
				   st = wallet.TxConfirmed
			   }
			   return wallet.Observation{Hash: hash,
				   Status: st, Height: rec.Height}, nil
		   }
		  return wallet.Observation{}, ErrNotFound
	  }
	   return in.Observation(), nil
  }

  func (m *memAdapter) SubmitWithdraw(ctx context.Context, req Outbound) (Receipt, error) {
	   _ = ctx
	  if m == nil {
		   return Receipt{}, ErrUnavailable
	   }
	  if req.Family == "" {
		  req.Family = m.fam
	   }
	   if req.Family != m.fam {
		   return Receipt{}, ErrChain
	  }
	  if req.Chain == "" {
		  req.Chain = firstChain(m.fam)
	   }
	   if err := wallet.ValidChainAddress(req.Chain, req.To); err != nil {
		  return Receipt{}, fmt.Errorf("%w: %v", ErrAddress, err)
	  }
	  if _, err := money.Parse(req.Amount); err != nil {
		  return Receipt{}, err
	   }
	   if req.From != "" {
		   if err := wallet.ValidChainAddress(req.Chain, req.From); err != nil {
			   return Receipt{}, fmt.Errorf("%w: from", ErrAddress)
		  }
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  if req.Nonce != "" {
		  if h, ok := m.nonces[req.Nonce]; ok {
			   rec := m.outs[h]
			  rec.Replayed = true
			   return rec, nil
		   }
	   }
	  NormalizeOutboundMemo(&req)
	  destTag := strings.TrimSpace(req.DestinationTag)
	   if destTag == "" {
		  destTag = strings.TrimSpace(req.Memo)
	   }
	  hash := hashOf(m.fam, "out", req.From, req.To, req.Amount, req.Nonce, destTag, fmt.Sprintf("%d", m.height+1))
	   rec := Receipt{Hash: hash, Family: m.fam, Chain: req.Chain, Status: StPending, Nonce: req.Nonce}
	  m.outs[hash] = rec
	   if req.Nonce != "" {
		  m.nonces[req.Nonce] = hash
	  }
	  if m.autoMine {
		   m.mineLocked(hash)
		   rec = m.outs[hash]
	  }
	  return rec, nil
  }

   func (m *memAdapter) Receipt(ctx context.Context, hash string) (Receipt, error) {
	  _ = ctx
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   return m.receiptLocked(hash)
  }

  func (m *memAdapter) receiptLocked(hash string) (Receipt, error) {
	   if rec, ok := m.outs[hash]; ok {
		  return rec, nil
	  }
	  if in, ok := m.inbounds[hash]; ok {
		  return Receipt{Hash: hash, Family: m.fam, Chain: in.Chain, Status: in.Status, Height: in.Height}, nil
	  }
	  return Receipt{}, ErrNotFound
  }

  func (m *memAdapter) Mine(hashes ...string) int64 {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  return m.mineLocked(hashes...)
   }

  func (m *memAdapter) mineLocked(hashes ...string) int64 {
	   prev := ""
	  if len(m.blocks) > 0 {
		  prev = m.blocks[len(m.blocks)-1].Hash
	   }
	   m.height++
	  bh := blockHashOf(m.fam, m.height, prev)
	   blk := Block{Height: m.height,
		  Hash: bh, Txs: append([]string{}, hashes...)}
	  m.blocks = append(m.blocks, blk)
	  need := ConfirmNeed(m.fam)
	   for _, h := range hashes {
		  if in, ok := m.inbounds[h]; ok && !in.Reorged {
			   if in.Height == 0 {
				  in.Height = m.height
				   in.BlockHash = bh
			   }
			  in.Confirmations = int(m.height - in.Height + 1)
			   if in.Confirmations >= need {
				   in.Status = StConfirmed
			  }
			   m.inbounds[h] = in
		   }
		  if rec, ok := m.outs[h]; ok {
			   rec.Height = m.height
			  rec.Status = StConfirmed
			   m.outs[h] = rec
		   }
	  }
	   for k, in := range m.inbounds {
		  if in.Height == 0 || in.Reorged {
			  continue
		   }
		  in.Confirmations = int(m.height - in.Height + 1)
		  if in.Confirmations >= need && in.Status == StPending {
			  in.Status = StConfirmed
		  }
		   m.inbounds[k] = in
	   }
	   return m.height
  }

  func (m *memAdapter) Reorg(drop int) []string {
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
	   dropped := make([]string, 0)
	   dead := map[string]struct{}{}
	   for _, b := range cut {
		  for _, h := range b.Txs {
			  dead[h] = struct{}{}
			  dropped = append(dropped, h)
		   }
	   }
	  for h := range dead {
		  if in, ok := m.inbounds[h]; ok {
			  in.Reorged = true
			   in.Status = StReorged
			  in.Confirmations = 0
			   m.inbounds[h] = in
		  }
		  if rec, ok := m.outs[h]; ok {
			   rec.Status = StReorged
			  m.outs[h] = rec
		   }
	   }
	   return dropped
   }

   func firstChain(fam Family) string {
	  cs := ChainsOf(fam)
	   if len(cs) == 0 {
		  return string(fam)
	  }
	   return cs[0]
   }
