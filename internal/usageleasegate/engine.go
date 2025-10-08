package usageleasegate

import (
	"context"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/hitstore"
	"account-assets.kucoin.net/funding/internal/usage"
	"account-assets.kucoin.net/funding/pkg/money"
)

type CommitHook func(ctx context.Context, l UsageLease) (hitID string, err error)

type Config struct {
	Store Store
	Usage *usage.Store
	Hits  *hitstore.Store
	Source OccupancySource
	Commit CommitHook
	Clock    func() time.Time
	TTL      time.Duration
	AutoResv bool
}

type Engine struct {
	mu    sync.Mutex
	store Store
	usage *usage.Store
	hits   *hitstore.Store
	src    OccupancySource
	commit CommitHook
	clock  func() time.Time
	ttl     time.Duration
	autoRes bool
	keys *keyLock
	fsm  interface {
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
	  if cfg.Store == nil {
		   cfg.Store = NewMemStore()
	  }
	   if cfg.Usage == nil {
		  cfg.Usage = usage.New().WithClock(cfg.Clock)
	   }
	  src := cfg.Source
	   if src == nil {
		  src = AdaptUsage(cfg.Usage)
	  }
	   if cfg.Commit == nil {
		  cfg.Commit = defaultCommit(cfg.Usage, cfg.Hits)
	  }
	  return &Engine{
		  store:  cfg.Store,
		  usage:  cfg.Usage,
		  hits:   cfg.Hits,
		  src:    src,
		   commit: cfg.Commit,
		  clock:  cfg.Clock,
		  ttl: cfg.TTL,
		   autoRes: cfg.AutoResv,
		   keys: newKeyLock(),
		   fsm: LeaseStateMachine(),
	   }
   }

   func defaultCommit(us *usage.Store, hs *hitstore.Store) CommitHook {
	  return func(ctx context.Context, l UsageLease) (string, error) {
		   if us != nil {
			   if err := us.Record(l.Kind, l.UserID, l.Asset, l.Dest, l.Amount); err != nil {
				   return "", err
			  }
		   }
		  if hs != nil {
			   if err := hs.Add(ctx, l.Kind, l.UserID, l.Asset, l.Dest, l.Amount, l.Idem); err != nil {
				  return "", err
			   }
		   }
		  return l.ID, nil
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
	   if e.src == nil {
		  return ErrUsageUnavailable
	   }
	  return nil
  }

  func (e *Engine) BindUsage(s *usage.Store) *Engine {
	  if e != nil {
		  e.usage = s
		  e.src = AdaptUsage(s)
	  }
	   return e
  }

   func (e *Engine) BindHits(s *hitstore.Store) *Engine {
	  if e != nil {
		  e.hits = s
	   }
	   return e
  }

   func (e *Engine) BindSource(src OccupancySource) *Engine {
	  if e != nil && src != nil {
		   e.src = src
	   }
	   return e
  }

  func (e *Engine) Usage() *usage.Store {
	  if e == nil {
		   return nil
	  }
	   return e.usage
  }

   func (e *Engine) Hits() *hitstore.Store {
	   if e == nil {
		  return nil
	   }
	   return e.hits
  }

  func (e *Engine) GetLease(ctx context.Context, id string) (UsageLease, error) {
	   if err := e.require(); err != nil {
		  return UsageLease{}, err
	   }
	  return e.store.GetLease(ctx, id)
   }

   func (e *Engine) ListLeases(ctx context.Context, userID string, limit int) ([]UsageLease, error) {
	   if err := e.require(); err != nil {
		   return nil, err
	  }
	  return e.store.ListLeasesByUser(ctx, userID, limit)
  }

   func (e *Engine) Health(ctx context.Context) (HealthResp, error) {
	  if err := e.require(); err != nil {
		   return HealthResp{}, err
	  }
	   snap,
		   _ := e.store.CountByState(ctx, LeaseSnapshotted)
	   res, _ := e.store.CountByState(ctx, LeaseReserved)
	   com, _ := e.store.CountByState(ctx, LeaseCommitted)
	   exp, _ := e.store.CountByState(ctx, LeaseExpired)
	   rev, _ := e.store.CountByState(ctx, LeaseRevoked)
	  held,
		   _ := e.store.CountReservations(ctx, ResHeld)
	  proofs,
		   _ := e.store.CountProofs(ctx)
	  aud, _ := e.store.CountAudits(ctx)
	   return HealthResp{
		   Snapshotted: snap, Reserved: res, Committed: com,
		   Expired: exp, Revoked: rev, HeldRes: held, Proofs: proofs, Audits: aud,
	   }, nil
   }

   func (e *Engine) liveView(n NormalizedSnap) WindowView {
	  return ReadWindow(e.src, n.Kind, n.UserID, n.Asset, n.Dest, n.DailyCapRaw, n.VelocityCap)
  }

  func (e *Engine) liveFromLease(l UsageLease) WindowView {
	   return ReadWindow(e.src, l.Kind, l.UserID, l.Asset, l.Dest, l.DailyCap, l.VelocityCap)
   }

  func (e *Engine) todayAmt(kind usage.Kind, user, asset string) money.Amount {
	  if e.src == nil {
		  return money.Zero()
	   }
	   return e.src.TodaySum(kind, user, asset)
  }
