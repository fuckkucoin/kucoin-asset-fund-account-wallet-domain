package cvquotelease

import (
	"context"
	"strings"
	"sync"
	"github.com/shopspring/decimal"
)

   func (e *Engine) MarkPut(ctx context.Context, req MarkPutReq) (MarkPutResp, error) {
	  if err := e.require(); err != nil {
		  return MarkPutResp{}, err
	  }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	   from := NormalizeAsset(req.From)
	   to := NormalizeAsset(req.To)
	   if from == "" || to == "" || from == to {
		  return MarkPutResp{}, ErrAssetRequired
	  }
	   if _, err := ParsePositiveRate(req.Rate); err != nil {
		   return MarkPutResp{}, ErrRateInvalid
	   }
	  pair := PairKey(from, to)
	   now := e.now()
	   src := strings.TrimSpace(req.Source)
	   if src == "" {
		  src = "mark_put"
	   }
	  prev, err := e.store.GetMark(ctx, pair)
	  gen := int64(1)
	  if err == nil {
		   gen = prev.Gen + 1
	  }
	  mk := MarkGeneration{
		  PairKey:   pair,
		  FromAsset: from, ToAsset: to,
		  Gen: gen, Rate: strings.TrimSpace(req.Rate),
		  BookPrint: ComputeBookPrint("funding", "1"),
		  Source:    src,
		  UpdatedAt: now, UpdatedBy: strings.TrimSpace(req.By),
	  }
	   revoked := 0
	   issued, _ := e.store.ListIssuedByPair(ctx, pair, DefaultRevokeBatch)
	   for _, lease := range issued {
		   if !CanRevoke(lease.State) {
			  continue
		   }
		  next, ferr := e.fsm.Apply(string(lease.State), LeaseEvRevoke)
		   if ferr != nil {
			  continue
		  }
		  ts := now
		   lease.State = LeaseState(next)
		  lease.RevokedAt = &ts
		  lease.UpdatedAt = now
		  lease.RevokeReason = "mark_gen_bump"
		  proof := e.sealProofLocked(ctx, lease.ID, lease.QuoteID, lease.UserID, lease.FromAsset, lease.ToAsset, DriftRevoked, lease.MarkGen, gen, lease.Rate, mk.Rate, lease.Book, lease.Book, lease.Fingerprint, "", "MarkPut bumped mark_gen; unconsumed lease revoked")
		  lease.ProofID = proof.ID
		   if _, serr := e.store.SaveLease(ctx, lease); serr != nil {
			   continue
		   }
		   revoked++
	   }
	   mk.RevokeCount = revoked
	  if _, err := e.store.SaveMark(ctx, mk); err != nil {
		   return MarkPutResp{}, err
	  }
	   if mem, ok := e.marks.(*MemoryMarks); ok {
		  mem.Put(from, to, mk.Rate)
	   }
	  return MarkPutResp{PairKey: pair, MarkGen: gen, Rate: mk.Rate, Revoked: revoked, BookPrint: mk.BookPrint}, nil
   }

  func (e *Engine) CurrentMarkGen(ctx context.Context, from, to string) (int64, error) {
	  if err := e.require(); err != nil {
		   return 0, err
	  }
	  mk, err := e.store.GetMark(ctx, PairKey(from, to))
	   if err != nil {
		   return 0, err
	  }
	   return mk.Gen, nil
   }

// MemoryMarks 与 cmd 用内存牌价源。MarkPut 同步写入。

type MemoryMarks struct {
	mu    sync.Mutex
	rates map[string]string
}

  func NewMemoryMarks() *MemoryMarks {
	   return &MemoryMarks{rates: map[string]string{}}
   }

  func (m *MemoryMarks) Put(from, to, rate string) {
	   if m == nil {
		   return
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  if m.rates == nil {
		   m.rates = map[string]string{}
	  }
	   m.rates[PairKey(from, to)] = rate
  }

  func (m *MemoryMarks) Rate(_ context.Context, from, to string) (decimal.Decimal, error) {
	  if m == nil {
		   return decimal.Zero, ErrQuoteSrc
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  s,
		  ok := m.rates[PairKey(from, to)]
	  if !ok {
		  return decimal.Zero, ErrQuoteSrc
	   }
	  d, err := ParsePositiveRate(s)
	  if err != nil {
		   return decimal.Zero, ErrQuoteSrc
	  }
	  return d, nil
   }
