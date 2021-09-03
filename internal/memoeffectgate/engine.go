package memoeffectgate

import (
	"context"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
)

type Config struct {
	Store Store
	Mutex   OutputMutex
	Tickets TicketPort
	Credit  CreditPort
	Reverse ReversePort
	Clock func() time.Time
	TTL time.Duration
}

type Engine struct {
	mu    sync.Mutex
	store Store
	mutex   OutputMutex
	tickets TicketPort
	credit  CreditPort
	reverse ReversePort
	clock   func() time.Time
	ttl time.Duration
}

   func New(cfg Config) *Engine {
	   if cfg.Clock == nil {
		  cfg.Clock = func() time.Time { return time.Now().UTC() }
	  }
	  if cfg.TTL <= 0 {
		   cfg.TTL = DefaultEffectTTL
	   }
	   if cfg.Store == nil {
		  cfg.Store = NewMemStore()
	  }
	   if cfg.Mutex == nil {
		  cfg.Mutex = NewMemClaimMutex()
	  }
	  if cfg.Tickets == nil {
		   cfg.Tickets = NewMemTicketBook()
	   }
	  if cfg.Credit == nil {
		   cfg.Credit = NewMemCreditPort()
	  }
	  return &Engine{
		  store: cfg.Store,
		   mutex: cfg.Mutex,
		  tickets: cfg.Tickets,
		   credit:  cfg.Credit,
		   reverse: cfg.Reverse,
		   clock:   cfg.Clock,
		  ttl:     cfg.TTL,
	  }
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
	  if e.mutex == nil {
		   return ErrMutexUnavailable
	   }
	  if e.tickets == nil {
		   return ErrBookUnavailable
	  }
	   if e.credit == nil {
		  return ErrCreditUnavailable
	  }
	  return nil
  }

  func (e *Engine) newID() string { return uuid.NewString() }

   func (e *Engine) audit(ctx context.Context, effectID, ev string, from, to EffectState, actor, detail string) {
	   _, _ = e.store.SaveAudit(ctx, EffectAudit{
		   ID:       e.newID(),
		   EffectID: effectID, Event: ev,
		   FromState: from, ToState: to, Actor: actor, Detail: detail, At: e.now(),
	   })
   }

  func (e *Engine) GetEffect(ctx context.Context, id string) (MemoEffect, error) {
	   if err := e.require(); err != nil {
		   return MemoEffect{}, err
	  }
	  return e.store.GetEffect(ctx, id)
  }

  func (e *Engine) GetFence(ctx context.Context, id string) (TicketCreditFence, error) {
	  if err := e.require(); err != nil {
		  return TicketCreditFence{}, err
	  }
	   return e.store.GetFence(ctx, id)
  }

  func (e *Engine) GetProof(ctx context.Context, id string) (MemoHalfCreditProof, error) {
	   if err := e.require(); err != nil {
		   return MemoHalfCreditProof{}, err
	  }
	   return e.store.GetProof(ctx, id)
   }

  func (e *Engine) ListProofs(ctx context.Context, kind ProofKind, limit int) ([]MemoHalfCreditProof, error) {
	  if err := e.require(); err != nil {
		   return nil, err
	   }
	  return e.store.ListProofs(ctx, kind, limit)
   }

   func (e *Engine) Health(_ context.Context) map[string]any {
	  nEff, _ := e.store.CountEffects(context.Background(), "")
	  nPr, _ := e.store.CountProofs(context.Background())
	   return map[string]any{
		  "ok": e != nil && e.store != nil, "effects": nEff, "proofs": nPr,
	  }
  }

   func normalizeAsset(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

  func normalizeStatus(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
