package fxmarkleasegate

import (
	"context"
	"sync"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/fxmark"
)

type Config struct {
	Store Store
	Reader MarkReader
	Book  *fxmark.Book
	Clock func() time.Time
	TTL          time.Duration
	ThresholdBPS int64
	AutoBind bool
}

type Engine struct {
	mu sync.Mutex
	store Store
	reader MarkReader
	book   *fxmark.Book
	clock func() time.Time
	ttl     time.Duration
	thresh  int64
	autoBnd bool
	keys    *keyLock
	fsm     interface {
		Apply(string, string) (string, error)
		Can(string, string) bool
	}
}

  func New(cfg Config) *Engine {
	   if cfg.Clock == nil {
		   cfg.Clock = func() time.Time { return time.Now().UTC() }
	   }
	   if cfg.TTL <= 0 {
		  cfg.TTL = DefaultLeaseTTL
	   }
	   if cfg.ThresholdBPS <= 0 {
		  cfg.ThresholdBPS = DefaultThresholdBPS
	  }
	   if cfg.Store == nil {
		  cfg.Store = NewMemStore()
	   }
	  rd := cfg.Reader
	   if rd == nil && cfg.Book != nil {
		  rd = AdaptBook(cfg.Book)
	  }
	  e := &Engine{
		   store: cfg.Store,
		   reader: rd,
		  book:   cfg.Book,
		   clock:  cfg.Clock,
		  ttl:    cfg.TTL,
		  thresh: cfg.ThresholdBPS,
		  autoBnd: cfg.AutoBind,
		  keys: newKeyLock(),
		  fsm:  LeaseStateMachine(),
	   }
	   if cfg.Book != nil {
		   cfg.Book.ObservePut(func(ctx context.Context, asset, usd string) {
			  _, _ = e.OnPutChange(ctx, asset, usd)
		   })
	   }
	   return e
   }

  func (e *Engine) now() time.Time {
	  if e == nil || e.clock == nil {
		   return time.Now().UTC()
	  }
	  return e.clock()
  }

  func (e *Engine) require() error {
	   if e == nil {
		   return ErrNilEngine
	   }
	  if e.store == nil {
		  return ErrStoreUnavailable
	  }
	  if e.reader == nil {
		   return ErrBookUnavailable
	   }
	   return nil
  }

   func (e *Engine) BindBook(b *fxmark.Book) *Engine {
	   if e != nil {
		  e.book = b
		   e.reader = AdaptBook(b)
		   if b != nil {
			  b.ObservePut(func(ctx context.Context, asset, usd string) {
				  _, _ = e.OnPutChange(ctx, asset, usd)
			  })
		   }
	  }
	  return e
   }

  func (e *Engine) BindReader(r MarkReader) *Engine {
	  if e != nil && r != nil {
		   e.reader = r
	   }
	  return e
  }

   func (e *Engine) GetLease(ctx context.Context, id string) (MarkLease, error) {
	  if err := e.require(); err != nil {
		   return MarkLease{}, err
	  }
	   return e.store.GetLease(ctx, id)
   }

   func (e *Engine) ListLeases(ctx context.Context, pair string, limit int) ([]MarkLease, error) {
	   if err := e.require(); err != nil {
		   return nil, err
	  }
	  return e.store.ListLeasesByPair(ctx, pair, limit)
   }

   func (e *Engine) Health(ctx context.Context) (HealthResp, error) {
	  if err := e.require(); err != nil {
		  return HealthResp{}, err
	  }
	   iss, _ := e.store.CountByState(ctx, LeaseIssued)
	  bnd,
		  _ := e.store.CountByState(ctx, LeaseBound)
	   con,
		   _ := e.store.CountByState(ctx, LeaseConsumed)
	  exp, _ := e.store.CountByState(ctx, LeaseExpired)
	  rev, _ := e.store.CountByState(ctx, LeaseRevoked)
	   proofs, _ := e.store.CountProofs(ctx)
	  aud, _ := e.store.CountAudits(ctx)
	   gens, _ := e.store.CountGenerations(ctx)
	   return HealthResp{
		  Issued: iss, Bound: bnd, Consumed: con,
		   Expired: exp, Revoked: rev, Proofs: proofs, Audits: aud, Gens: gens,
	   }, nil
   }

   func (e *Engine) readLive(ctx context.Context, from, to string, preferLive bool) (LiveSnap, error) {
	  from, to, err := NormalizePair(from, to)
	  if err != nil {
		   return LiveSnap{}, err
	  }
	  fromUSD, err := e.reader.USD(ctx, from)
	   if err != nil {
		  return LiveSnap{}, ErrRateMissing
	   }
	   toUSD, err := e.reader.USD(ctx, to)
	  if err != nil {
		  return LiveSnap{}, ErrRateMissing
	   }
	   rate, err := e.reader.Rate(ctx, from, to)
	   if err != nil {
		  return LiveSnap{}, ErrRateMissing
	   }
	  src := SourceKV
	  if preferLive {
		  if n, s, e2 := e.reader.TravelUSD(ctx, from); e2 == nil && n.IsPositive() {
			   fromUSD = n
			   if s != "" {
				   src = s
			  } else {
				  src = SourceLive
			   }
			  if t, ts, e3 := e.reader.TravelUSD(ctx, to); e3 == nil && t.IsPositive() {
				  toUSD = t
				  _ = ts
			   }
			   if toUSD.IsPositive() {
				  rate = fromUSD.Div(toUSD)
			  }
		  }
	   } else if e.book != nil {
		   // Rate 内部可能已走 HTTP；来源记 kv，复核仍数与指纹。
		  src = SourceKV
	  }
	  fromGen := e.reader.GenerationOf(from)
	  toGen := e.reader.GenerationOf(to)
	   gen := CombineGeneration(fromGen, toGen)
	  return LiveSnap{
		   From: from, To: to,
		  FromUSD: fromUSD.String(), ToUSD: toUSD.String(),
		   Rate: rate.String(), Source: src,
		   Generation: gen, FromGen: fromGen, ToGen: toGen,
	  }, nil
   }

   func newLeaseID() string { return "mkl-" + uuid.NewString() }
