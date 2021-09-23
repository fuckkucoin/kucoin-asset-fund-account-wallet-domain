package occupbook

import (
	"context"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Book 占用账本。

type Book struct {
	store Store
	clock func() time.Time
	requireFence bool
	lotMu sync.Map
}

// BookOption 可选配置。
type BookOption func(*Book)

// WithClock 注入时钟。
   func WithClock(fn func() time.Time) BookOption {
	  return func(b *Book) {
		   if fn != nil {
			   b.clock = fn
		   }
	  }
}

// WithRequireFence 强制 T4/mixer 走围栏入口。

   func WithRequireFence(on bool) BookOption {
	   return func(b *Book) {
		   b.requireFence = on
	  }
   }

// NewBook 构造账本。store 为 nil 时使用 MemStore。
  func NewBook(store Store, opts ...BookOption) *Book {
	   if store == nil {
		  store = NewMemStore()
	  }
	   b := &Book{
		  store: store,
		   clock: func() time.Time { return time.Now().UTC() },
	   }
	  for _, opt := range opts {
		   opt(b)
	  }
	  return b
   }

  func (b *Book) now() time.Time {
	   if b == nil || b.clock == nil {
		  return time.Now().UTC()
	   }
	  return b.clock().UTC()
  }

  func (b *Book) lockLot(lotID string) func() {
	  v, _ := b.lotMu.LoadOrStore(lotID, &sync.Mutex{})
	  mu := v.(*sync.Mutex)
	  mu.Lock()
	  return mu.Unlock
   }

   func (b *Book) audit(ctx context.Context, lot *Lot, action, reason string, ok bool) {
	   if lot == nil {
		   return
	   }
	  ev := &AuditEvent{
		   AuditID: uuid.NewString(),
		  LotID: lot.LotID,
		   Action:   action,
		  Reason:   reason,
		  OK:       ok,
		   Inflight: lot.Buckets.Inflight.String(),
		  Available: lot.Buckets.Available.String(),
		  Void:      lot.Buckets.Void.String(),
		   Total:     lot.OccupiedTotal.String(),
		   CreatedAt: b.now(),
	  }
	   _ = b.store.AppendAudit(ctx, ev)
  }

// Observe 记录观察额并初始化三桶为零后的占用总额。
// 观察后 OccupiedTotal=观察额，三桶仍为零，须随后 Hold 填在途。
  func (b *Book) Observe(ctx context.Context, req ObserveReq) (*Lot, error) {
	   if b == nil || b.store == nil {
		   return nil, ErrUnavailable
	   }
	  lotID := NormalizeID(req.LotID)
	   userID := NormalizeID(req.UserID)
	  asset := NormalizeAsset(req.Asset)
	   if lotID == "" {
		  return nil, ErrLotRequired
	   }
	  if userID == "" {
		  return nil, ErrUserRequired
	  }
	  if asset == "" {
		  return nil, ErrAssetRequired
	  }
	   amt, err := ParseAmount(req.Amount)
	  if err != nil {
		   return nil, err
	  }
	  rc := RiskClass(strings.ToLower(strings.TrimSpace(req.RiskClass)))
	   if rc == "" {
		  rc = RiskNormal
	  }
	  if !rc.Valid() {
		  return nil, ErrRiskClassInvalid
	   }
	   now := b.now()
	   lot := &Lot{
		   LotID:  lotID,
		  UserID: userID,
		   Asset:  asset,
		   OccupiedTotal: amt,
		  ObservedAmt: amt,
		  RiskClass: rc,
		   State:     LotObserved,
		   Buckets:    Bucket{Inflight: decimal.Zero, Available: decimal.Zero, Void: decimal.Zero},
		  ObserveRef: strings.TrimSpace(req.ObserveRef),
		  Generation: 1,
		   CreatedAt: now,
		   UpdatedAt: now,
	  }
	  if err := b.store.SaveLot(ctx, lot); err != nil {
		  return nil, err
	   }
	  b.audit(ctx, lot, "observe", "observed", true)
	   return lot, nil
   }

// Hold 观察后把金额记入在途。viaFence 标记是否经 bucket_fence。
   func (b *Book) Hold(ctx context.Context, req HoldReq, viaFence bool) (*Hold, *Lot, error) {
	   if b == nil || b.store == nil {
		  return nil, nil, ErrUnavailable
	  }
	  lotID := NormalizeID(req.LotID)
	  if lotID == "" {
		  return nil, nil, ErrLotRequired
	   }
	  unlock := b.lockLot(lotID)
	   defer unlock()
	   lot, err := b.store.GetLot(ctx, lotID)
	  if err != nil {
		  return nil, nil, err
	  }
	  if lot.State.Terminal() {
		  return nil, lot, ErrLotClosed
	   }
	   if lot.HalfOpen {
		  return nil, lot, ErrHalfPersist
	  }
	   if b.requireFence && lot.RiskClass.RequiresFullOccupancy() && !viaFence {
		  b.audit(ctx, lot, "hold", "mixer_bypass", false)
		  return nil, lot, ErrMixerBypass
	  }
	   amt, err := ParseAmount(req.Amount)
	  if err != nil {
		  return nil, lot, err
	  }
	   if lot.RiskClass.RequiresFullOccupancy() && !amt.Equal(lot.OccupiedTotal) {
		  return nil, lot, ErrAmountInvalid
	   }
	   if req.IdemKey != "" {
		   if exist, e := b.store.GetHoldByIdem(ctx, lotID, req.IdemKey); e == nil && exist != nil {
			  cur, _ := b.store.GetLot(ctx, lotID)
			  return exist, cur, nil
		   }
	   }
	   if !lot.Buckets.Inflight.Add(amt).Add(lot.Buckets.Available).Add(lot.Buckets.Void).Equal(lot.OccupiedTotal) &&
		  !lot.Buckets.Zero() {
		  // 已有部分占用时，增量 Hold 必须仍守恒。
	  }
	  next := lot.Buckets.Clone()
	   next.Inflight = next.Inflight.Add(amt)
	   if !Conserve(next, lot.OccupiedTotal) {
		   // 首次 Hold：观察后三桶为零，允许把观察额一次性填入在途。
		   if lot.Buckets.Zero() && amt.Equal(lot.OccupiedTotal) {
			  next = Bucket{Inflight: amt,
				  Available: decimal.Zero, Void: decimal.Zero}
		  } else if !Conserve(next, lot.OccupiedTotal) {
			   b.audit(ctx, lot, "hold", "conserve_broken", false)
			  return nil, lot, ErrConserveBroken
		  }
	  }
	   holdID := NormalizeID(req.HoldID)
	  if holdID == "" {
		  holdID = uuid.NewString()
	  }
	   rc := lot.RiskClass
	  if strings.TrimSpace(req.RiskClass) != "" {
		   rc = RiskClass(strings.ToLower(strings.TrimSpace(req.RiskClass)))
		   if !rc.Valid() {
			   return nil, lot, ErrRiskClassInvalid
		   }
	   }
	  now := b.now()
	  h := &Hold{
		  HoldID: holdID,
		  LotID:  lotID,
		   UserID: lot.UserID,
		  Asset:  lot.Asset,
		  Amount:    amt,
		   IdemKey:   strings.TrimSpace(req.IdemKey),
		   RiskClass: rc,
		   PersistOK: true,
		  CreatedAt: now,
	  }
	   lot.Buckets = next
	  lot.State = LotHeld
	   lot.Generation++
	  lot.UpdatedAt = now
	  if err := b.store.SaveHold(ctx, h); err != nil {
		   return nil, lot, err
	   }
	  if err := b.store.UpdateLot(ctx, lot); err != nil {
		  h.PersistOK = false
		   lot.HalfOpen = true
		  _ = b.store.UpdateHold(ctx, h)
		   _ = b.store.UpdateLot(ctx, lot)
		  b.audit(ctx, lot, "hold", "half_persist", false)
		  return h, lot, ErrHalfPersist
	  }
	   if !lot.ConserveOK() {
		   lot.State = LotBroken
		  _ = b.store.UpdateLot(ctx, lot)
		   b.audit(ctx, lot, "hold", "conserve_broken", false)
		   return nil, lot, ErrConserveBroken
	   }
	   b.audit(ctx, lot, "hold", "ok", true)
	   return h, lot, nil
  }

// MarkHalfHold 将成功 Hold 标记为桶落库半截，供围栏续提。

  func (b *Book) MarkHalfHold(ctx context.Context, lotID, holdID string) error {
	   if b == nil || b.store == nil {
		  return ErrUnavailable
	   }
	   unlock := b.lockLot(lotID)
	   defer unlock()
	   lot, err := b.store.GetLot(ctx, lotID)
	  if err != nil {
		  return err
	   }
	   h, err := b.store.GetHold(ctx, holdID)
	  if err != nil {
		   return err
	  }
	   h.PersistOK = false
	   lot.HalfOpen = true
	   lot.UpdatedAt = b.now()
	   if err := b.store.UpdateHold(ctx, h); err != nil {
		  return err
	  }
	   if err := b.store.UpdateLot(ctx, lot); err != nil {
		  return err
	   }
	  b.audit(ctx, lot, "hold", "half_marked", false)
	  return nil
  }

// ResumeHalfHold 半截续提：补写桶并清除 HalfOpen，拒绝另开全额占用。
  func (b *Book) ResumeHalfHold(ctx context.Context, lotID, holdID string) (*Hold, *Lot, error) {
	   if b == nil || b.store == nil {
		  return nil, nil, ErrUnavailable
	  }
	   unlock := b.lockLot(lotID)
	   defer unlock()
	   lot, err := b.store.GetLot(ctx, lotID)
	   if err != nil {
		  return nil, nil, err
	   }
	   h, err := b.store.GetHold(ctx, holdID)
	  if err != nil {
		  return nil, lot, err
	  }
	  if h.PersistOK && !lot.HalfOpen {
		   return h, lot, nil
	   }
	  if lot.Buckets.Zero() && h.Amount.Equal(lot.OccupiedTotal) {
		   lot.Buckets = Bucket{Inflight: h.Amount.Copy(), Available: decimal.Zero, Void: decimal.Zero}
	  }
	   if !lot.ConserveOK() {
		  lot.State = LotBroken
		  _ = b.store.UpdateLot(ctx, lot)
		  b.audit(ctx, lot, "resume", "conserve_broken", false)
		   return nil, lot, ErrConserveBroken
	   }
	   h.PersistOK = true
	   lot.HalfOpen = false
	  lot.State = LotHeld
	   lot.UpdatedAt = b.now()
	  if err := b.store.UpdateHold(ctx, h); err != nil {
		   return nil, lot, err
	   }
	  if err := b.store.UpdateLot(ctx, lot); err != nil {
		   return nil, lot, err
	  }
	  b.audit(ctx, lot, "resume", "ok", true)
	  return h, lot, nil
   }

// Release 在途转可用。

   func (b *Book) Release(ctx context.Context, req ReleaseReq) (*Release, *Lot, error) {
	  if b == nil || b.store == nil {
		   return nil, nil, ErrUnavailable
	  }
	   lotID := NormalizeID(req.LotID)
	  if lotID == "" {
		  return nil, nil, ErrLotRequired
	  }
	  unlock := b.lockLot(lotID)
	   defer unlock()
	   return b.releaseLocked(ctx, req)
   }

   func (b *Book) releaseLocked(ctx context.Context, req ReleaseReq) (*Release, *Lot, error) {
	  lot, err := b.store.GetLot(ctx, NormalizeID(req.LotID))
	   if err != nil {
		   return nil, nil, err
	   }
	   if lot.State.Terminal() {
		   return nil, lot, ErrLotClosed
	   }
	   if lot.HalfOpen {
		   return nil, lot, ErrHalfPersist
	   }
	  if req.IdemKey != "" {
		  if exist, e := b.store.GetReleaseByIdem(ctx, lot.LotID, req.IdemKey); e == nil && exist != nil {
			   return exist, lot, nil
		   }
	  }
	   h, err := b.store.GetHold(ctx, NormalizeID(req.HoldID))
	  if err != nil {
		  return nil, lot, err
	   }
	  if h.LotID != lot.LotID {
		   return nil, lot, ErrHoldNotFound
	  }
	  if h.Reclassed {
		  b.audit(ctx, lot, "release", "already_reclassed", false)
		  return nil, lot, ErrHoldAlreadyReclassed
	   }
	  if h.Released {
		   if exist, e := b.store.GetReleaseByIdem(ctx, lot.LotID, req.IdemKey); e == nil {
			   return exist, lot, nil
		   }
		   return nil, lot, ErrHoldAlreadyReleased
	  }
	   amt,
		  err := ParseAmount(req.Amount)
	  if err != nil {
		  return nil, lot, err
	  }
	   if !amt.Equal(h.Amount) {
		   return nil, lot, ErrAmountInvalid
	  }
	   if lot.Buckets.Inflight.LessThan(amt) {
		   b.audit(ctx, lot, "release", "insufficient_inflight", false)
		  return nil, lot, ErrInsufficientInflight
	  }
	   next := lot.Buckets.Clone()
	   next.Inflight = next.Inflight.Sub(amt)
	  next.Available = next.Available.Add(amt)
	  if !Conserve(next, lot.OccupiedTotal) {
		  lot.State = LotBroken
		  _ = b.store.UpdateLot(ctx, lot)
		  b.audit(ctx, lot, "release", "conserve_broken", false)
		   return nil, lot, ErrConserveBroken
	   }
	   now := b.now()
	  relID := NormalizeID(req.ReleaseID)
	  if relID == "" {
		   relID = uuid.NewString()
	   }
	  rel := &Release{
		  ReleaseID: relID,
		  HoldID:  h.HoldID,
		   LotID:   lot.LotID,
		   Amount:  amt,
		  IdemKey: strings.TrimSpace(req.IdemKey),
		  CreatedAt: now,
	  }
	  h.Released = true
	  h.ReleasedAt = &now
	   lot.Buckets = next
	   lot.State = LotOpen
	   lot.Generation++
	   lot.UpdatedAt = now
	   if lot.Buckets.Inflight.IsZero() {
		  lot.State = LotClosed
		  lot.ClosedAt = &now
	   }
	  if err := b.store.SaveRelease(ctx, rel); err != nil {
		   return nil, lot, err
	   }
	   if err := b.store.UpdateHold(ctx, h); err != nil {
		   return nil, lot, err
	   }
	  if err := b.store.UpdateLot(ctx, lot); err != nil {
		   return nil, lot, err
	  }
	  b.audit(ctx, lot, "release", "ok", true)
	  return rel, lot, nil
   }

// Reclass 在途转作废（升档）。

  func (b *Book) Reclass(ctx context.Context, req ReclassReq) (*Reclass, *Lot, error) {
	   if b == nil || b.store == nil {
		  return nil, nil, ErrUnavailable
	   }
	  lotID := NormalizeID(req.LotID)
	   if lotID == "" {
		  return nil, nil, ErrLotRequired
	  }
	  unlock := b.lockLot(lotID)
	  defer unlock()
	   return b.reclassLocked(ctx, req)
  }

   func (b *Book) reclassLocked(ctx context.Context, req ReclassReq) (*Reclass, *Lot, error) {
	   lot, err := b.store.GetLot(ctx, NormalizeID(req.LotID))
	   if err != nil {
		   return nil, nil, err
	   }
	  if lot.State.Terminal() {
		   return nil, lot, ErrLotClosed
	   }
	  if lot.HalfOpen {
		   return nil, lot, ErrHalfPersist
	  }
	   if req.IdemKey != "" {
		   if exist, e := b.store.GetReclassByIdem(ctx, lot.LotID, req.IdemKey); e == nil && exist != nil {
			   return exist, lot, nil
		   }
	  }
	  h, err := b.store.GetHold(ctx, NormalizeID(req.HoldID))
	  if err != nil {
		   return nil, lot, err
	  }
	  if h.LotID != lot.LotID {
		  return nil, lot, ErrHoldNotFound
	   }
	   if h.Released {
		   b.audit(ctx, lot, "reclass", "already_released", false)
		  return nil, lot, ErrHoldAlreadyReleased
	  }
	   if h.Reclassed {
		  return nil, lot, ErrHoldAlreadyReclassed
	   }
	  to := RiskClass(strings.ToLower(strings.TrimSpace(req.ToClass)))
	  if !to.Valid() {
		   return nil, lot, ErrRiskClassInvalid
	  }
	   amt, err := ParseAmount(req.Amount)
	   if err != nil {
		  return nil, lot, err
	  }
	  if !amt.Equal(h.Amount) {
		   return nil, lot, ErrAmountInvalid
	   }
	   if lot.Buckets.Inflight.LessThan(amt) {
		   b.audit(ctx, lot, "reclass", "insufficient_inflight", false)
		  return nil, lot, ErrInsufficientInflight
	   }
	   next := lot.Buckets.Clone()
	   next.Inflight = next.Inflight.Sub(amt)
	  next.Void = next.Void.Add(amt)
	   if !Conserve(next, lot.OccupiedTotal) {
		   lot.State = LotBroken
		  _ = b.store.UpdateLot(ctx, lot)
		   b.audit(ctx, lot, "reclass", "conserve_broken", false)
		  return nil, lot, ErrConserveBroken
	   }
	  now := b.now()
	   rid := NormalizeID(req.ReclassID)
	  if rid == "" {
		  rid = uuid.NewString()
	   }
	  rc := &Reclass{
		  ReclassID: rid,
		  HoldID:    h.HoldID,
		  LotID:     lot.LotID,
		  FromClass: h.RiskClass,
		  ToClass:   to,
		  Amount:    amt,
		   IdemKey: strings.TrimSpace(req.IdemKey),
		  ReasonCode: strings.TrimSpace(req.ReasonCode),
		   CreatedAt:  now,
	  }
	  h.Reclassed = true
	   lot.Buckets = next
	  lot.RiskClass = to
	   lot.State = LotOpen
	   lot.Generation++
	   lot.UpdatedAt = now
	   if lot.Buckets.Inflight.IsZero() {
		   lot.State = LotClosed
		  lot.ClosedAt = &now
	   }
	   if err := b.store.SaveReclass(ctx, rc); err != nil {
		  return nil, lot, err
	   }
	   if err := b.store.UpdateHold(ctx, h); err != nil {
		   return nil, lot, err
	  }
	   if err := b.store.UpdateLot(ctx, lot); err != nil {
		  return nil, lot, err
	  }
	  b.audit(ctx, lot, "reclass", "ok", true)
	  return rc, lot, nil
   }

// GetLot 只读批次。
  func (b *Book) GetLot(ctx context.Context, lotID string) (*Lot, error) {
	   if b == nil || b.store == nil {
		  return nil, ErrUnavailable
	  }
	  return b.store.GetLot(ctx, NormalizeID(lotID))
   }

// ListAudit 只读审计。
   func (b *Book) ListAudit(ctx context.Context, lotID string, limit int) ([]*AuditEvent, error) {
	  if b == nil || b.store == nil {
		  return nil, ErrUnavailable
	   }
	   return b.store.ListAudit(ctx, NormalizeID(lotID), limit)
  }

// Store 暴露仓储（围栏包只读挂钩）。

  func (b *Book) Store() Store {
	   if b == nil {
		   return nil
	   }
	  return b.store
   }

// ForceBreak /巡检：将批次标为击穿（不改余额）。
   func (b *Book) ForceBreak(ctx context.Context, lotID, reason string) (*Lot, error) {
	  if b == nil || b.store == nil {
		  return nil, ErrUnavailable
	   }
	  lot, err := b.store.GetLot(ctx, lotID)
	  if err != nil {
		   return nil, err
	  }
	   lot.State = LotBroken
	   lot.UpdatedAt = b.now()
	   if err := b.store.UpdateLot(ctx, lot); err != nil {
		   return nil, err
	  }
	  b.audit(ctx, lot, "break", reason, false)
	   return lot, nil
   }
