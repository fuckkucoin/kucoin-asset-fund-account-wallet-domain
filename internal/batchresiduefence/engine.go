package batchresiduefence

import (
	"context"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
)

type Config struct {
	Store Store
	Viewer   BatchViewer
	Clock    func() time.Time
	FenceTTL time.Duration
	HalfTTL  time.Duration
	MutexTTL time.Duration
}

type Engine struct {
	mu     sync.Mutex
	store  Store
	viewer BatchViewer
	clock  func() time.Time
	fenceTTL time.Duration
	halfTTL  time.Duration
	mutexTTL time.Duration
}

  func New(cfg Config) *Engine {
	   if cfg.Clock == nil {
		  cfg.Clock = func() time.Time { return time.Now().UTC() }
	  }
	  if cfg.FenceTTL <= 0 {
		  cfg.FenceTTL = DefaultFenceTTL
	  }
	   if cfg.HalfTTL <= 0 {
		  cfg.HalfTTL = DefaultHalfTTL
	   }
	  if cfg.MutexTTL <= 0 {
		  cfg.MutexTTL = 2 * time.Minute
	   }
	   if cfg.Store == nil {
		   cfg.Store = NewMemStore()
	  }
	   if cfg.Viewer == nil {
		   cfg.Viewer = NewStaticViewer()
	  }
	  return &Engine{
		   store: cfg.Store,
		   viewer: cfg.Viewer, clock: cfg.Clock,
		   fenceTTL: cfg.FenceTTL, halfTTL: cfg.HalfTTL, mutexTTL: cfg.MutexTTL,
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
	  if e.viewer == nil {
		   return ErrViewer
	  }
	   return nil
   }

   func (e *Engine) newID() string { return uuid.NewString() }

   func (e *Engine) audit(ctx context.Context, batchID, fenceID, ev, from, to, actor, detail string) {
	   _ = e.store.SaveAudit(ctx, AuditEvent{
		  ID: e.newID(), BatchID: batchID, FenceID: fenceID, Event: ev,
		   From: from, To: to, Actor: actor, Detail: detail, At: e.now(),
	   })
  }

// IssueFence 仅为失败项签发围栏。已提交/跳过直接失败闭锁。
  func (e *Engine) IssueFence(ctx context.Context, req IssueFenceReq) (ResidueFence, error) {
	  if err := e.require(); err != nil {
		   return ResidueFence{}, err
	  }
	   batchID, err := requireBatchID(req.BatchID)
	  if err != nil {
		  return ResidueFence{}, err
	   }
	  itemIdem, err := requireItemIdem(req.ItemIdem)
	  if err != nil {
		   return ResidueFence{}, err
	   }
	   actor, err := requireActor(req.Actor)
	  if err != nil {
		   return ResidueFence{}, err
	  }
	  user,
		  err := requireUser(req.UserID)
	  if err != nil {
		  return ResidueFence{}, err
	   }
	  asset, err := requireAsset(req.Asset)
	   if err != nil {
		  return ResidueFence{}, err
	   }
	   amt, err := requireAmount(req.Amount)
	  if err != nil {
		   return ResidueFence{}, err
	  }
	   view, verr := e.viewer.ViewItem(ctx, batchID, itemIdem)
	   if verr == nil {
		  if err := forbidRetryPhase(view.Phase); err != nil {
			  return ResidueFence{}, err
		  }
	   }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	   exist, err := e.store.ListFencesByItem(ctx, batchID, itemIdem)
	  if err != nil {
		  return ResidueFence{}, err
	   }
	   gen := 1
	  for _, f := range exist {
		  if CanRetryHold(f.State) {
			  return f, nil
		  }
		   if f.Generation >= gen {
			  gen = f.Generation + 1
		  }
	  }
	   if gen > MaxGeneration {
		  return ResidueFence{}, ErrMaxGeneration
	   }
	   ttl := e.fenceTTL
	  if req.TTL > 0 {
		  ttl = time.Duration(req.TTL) * time.Millisecond
	   }
	  now := e.now()
	   fp := FreezeFingerprint(batchID, itemIdem, user, asset, amt, req.OriginalFreezeRef)
	  row := ResidueFence{
		  ID: e.newID(), BatchID: batchID, ItemIdem: itemIdem,
		  FreezeFingerprint: fp,
		  Generation:        gen, TTL: ttl.Milliseconds(),
		  IssuedAt: now, ExpireAt: now.Add(ttl), State: FenceIssued,
		  Actor: actor, UserID: user, Asset: asset, Amount: amt,
		  OriginalFreezeRef: strings.TrimSpace(req.OriginalFreezeRef),
	   }
	  if err := e.store.SaveFence(ctx, row); err != nil {
		  return ResidueFence{}, err
	   }
	  e.audit(ctx, batchID, row.ID, AuditIssueFence, "", string(FenceIssued), actor, fp)
	  return row, nil
  }

  func (e *Engine) GetFence(ctx context.Context, id string) (ResidueFence, error) {
	   if err := e.require(); err != nil {
		  return ResidueFence{}, err
	  }
	  f, ok, err := e.store.GetFence(ctx, strings.TrimSpace(id))
	   if err != nil {
		  return ResidueFence{}, err
	  }
	   if !ok {
		  return ResidueFence{}, ErrFence
	  }
	  return f, nil
   }

   func (e *Engine) ListFences(ctx context.Context, batchID string) ([]ResidueFence, error) {
	  if err := e.require(); err != nil {
		   return nil, err
	   }
	   id, err := requireBatchID(batchID)
	  if err != nil {
		  return nil, err
	   }
	  return e.store.ListFencesByBatch(ctx, id)
   }

   func (e *Engine) ReleaseFence(ctx context.Context, fenceID, actor string) (ResidueFence, error) {
	   if err := e.require(); err != nil {
		  return ResidueFence{}, err
	   }
	  act, err := requireActor(actor)
	  if err != nil {
		   return ResidueFence{}, err
	   }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	   f, ok, err := e.store.GetFence(ctx, strings.TrimSpace(fenceID))
	   if err != nil {
		   return ResidueFence{}, err
	  }
	  if !ok {
		   return ResidueFence{}, ErrFence
	   }
	  next, err := MustApplyFence(f.State, FenceEvRelease)
	  if err != nil {
		  return ResidueFence{}, err
	  }
	  from := f.State
	   now := e.now()
	  f.State = next
	  f.ReleasedAt = &now
	  if err := e.store.SaveFence(ctx, f); err != nil {
		   return ResidueFence{}, err
	  }
	   if slot, ok, _ := e.store.GetSlot(ctx, f.ItemIdem); ok && slot.FenceID == f.ID {
		   slot.Status = FreezeSlotReleased
		  slot.ReleasedAt = &now
		  _ = e.store.SaveSlot(ctx, slot)
	  }
	   e.audit(ctx, f.BatchID, f.ID, AuditReleaseFence, string(from), string(next), act, "")
	   return f, nil
  }
