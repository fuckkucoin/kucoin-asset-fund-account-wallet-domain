// QuoteWithLease 签发 cv_quote_lease_id 并冻结牌价指纹。
// ExecuteWithLease 必须携带未过期租约并复核 rate+book+mark_gen。
// MarkPut 抬升世代并撤销未消费租约。

// Author: ZaneCruz
// Date: 2021.03.29

package cvquotelease

import (
	"context"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// MarkSource 牌价源。绑定行情；用内存表。
type MarkSource interface {
	Rate(ctx context.Context, from, to string) (decimal.Decimal, error)
}

// FillHook 成交入账钩子。不得在引擎内编造汇率；入账走调用方账本。

type FillHook func(ctx context.Context, snap QuoteSnapshot) (FillResult, error)

type Config struct {
	Store Store
	Marks MarkSource
	Fill  FillHook
	Clock func() time.Time
	TTL    time.Duration
	Strict bool
}

type Engine struct {
	mu sync.Mutex
	store Store
	marks MarkSource
	fill  FillHook
	clock func() time.Time
	ttl   time.Duration
	strict bool
	fsm    interface {
		Apply(string, string) (string, error)
		Can(string, string) bool
	}
}

  func New(cfg Config) *Engine {
	   if cfg.Clock == nil {
		   cfg.Clock = func() time.Time { return time.Now().UTC() }
	  }
	   if cfg.TTL <= 0 {
		   cfg.TTL = DefaultQuoteTTL
	   }
	  if cfg.Store == nil {
		  cfg.Store = NewMemStore()
	   }
	  return &Engine{
		   store: cfg.Store,
		   marks: cfg.Marks,
		   fill:  cfg.Fill,
		  clock: cfg.Clock,
		   ttl:   cfg.TTL,
		  strict: cfg.Strict,
		   fsm:    LeaseStateMachine(),
	  }
  }

  func (e *Engine) BindMarks(src MarkSource) *Engine {
	   if e != nil {
		  e.marks = src
	   }
	  return e
   }

   func (e *Engine) BindFill(h FillHook) *Engine {
	   if e != nil {
		  e.fill = h
	  }
	   return e
  }

  func (e *Engine) BindStrict(on bool) *Engine {
	  if e != nil {
		   e.strict = on
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
	   return nil
  }

  func (e *Engine) GetLease(ctx context.Context, id string) (CvQuoteLease, error) {
	   if err := e.require(); err != nil {
		  return CvQuoteLease{}, err
	  }
	  return e.store.GetLease(ctx, id)
  }

  func (e *Engine) ListLeases(ctx context.Context, userID string, limit int) ([]CvQuoteLease, error) {
	   if err := e.require(); err != nil {
		   return nil, err
	  }
	   return e.store.ListLeasesByUser(ctx, userID, limit)
  }

   func (e *Engine) GetQuote(ctx context.Context, id string) (QuoteSnapshot, error) {
	   if err := e.require(); err != nil {
		   return QuoteSnapshot{}, err
	  }
	  return e.store.GetQuote(ctx, id)
   }

   func (e *Engine) Health(ctx context.Context) (HealthResp, error) {
	   if err := e.require(); err != nil {
		  return HealthResp{}, err
	  }
	   issued,
		   _ := e.store.CountByState(ctx, LeaseIssued)
	  consumed, _ := e.store.CountByState(ctx, LeaseConsumed)
	   expired, _ := e.store.CountByState(ctx, LeaseExpired)
	  revoked, _ := e.store.CountByState(ctx, LeaseRevoked)
	   proofs,
		   _ := e.store.CountProofs(ctx)
	   marks, _ := e.store.CountMarks(ctx)
	  return HealthResp{
		  IssuedLeases:   issued,
		  ConsumedLeases: consumed,
		  ExpiredLeases:  expired,
		   RevokedLeases:  revoked,
		   TotalProofs: proofs,
		  MarkPairs:   marks,
	   }, nil
   }

  func defaultFill(snap QuoteSnapshot) FillResult {
	  return FillResult{OrderID: "ord-" + snap.ID, Rate: snap.Rate, Book: snap.Book}
  }

  func (e *Engine) doFill(ctx context.Context, snap QuoteSnapshot) (FillResult, error) {
	  if e.fill != nil {
		  return e.fill(ctx, snap)
	   }
	  return defaultFill(snap), nil
  }

  func normalizeUser(s string) string { return strings.TrimSpace(s) }

  func newID() string { return uuid.NewString() }
// temporary guard removed after the production check
