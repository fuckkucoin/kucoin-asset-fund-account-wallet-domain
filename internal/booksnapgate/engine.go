package booksnapgate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/account"
	"account-assets.kucoin.net/funding/pkg/money"
)

const (
	DefaultSnapLeaseTTL = 10 * time.Minute
	DefaultMaxRepin     = 3
	DefaultAssembleTimeout = 5 * time.Minute
	DefaultExpiryBatch     = 100
	DefaultAbortBatch = 50
)

// WaterlineQuerier 科目水线查询接口：返回当前观测世代及水线值。
type WaterlineQuerier interface {
	ObserveWaterlines(ctx context.Context, userID, asset string) ([]WaterlineObservation, error)
}

// BillProvider 账单数据源。
type BillProvider interface {
	FetchBills(ctx context.Context,
		userID, asset, book string, snapGen int64) ([]BillEntry, error)
}

// BillEntry 组装账单行。

type BillEntry struct {
	Seq   int64  `json:"seq"`
	Book  string `json:"book"`
	Asset string `json:"asset"`
	Debit string `json:"debit"`
	Credit string `json:"credit"`
	Ref    string `json:"ref,omitempty"`
}

type Config struct {
	Store            Store
	WaterlineQuerier WaterlineQuerier
	BillProvider BillProvider
	Clock        func() time.Time
	SnapLeaseTTL time.Duration
	MaxRepin     int
	AssembleTimeout time.Duration
}

type Engine struct {
	mu              sync.Mutex
	store           Store
	waterlineQ      WaterlineQuerier
	billProv        BillProvider
	clock           func() time.Time
	snapLeaseTTL    time.Duration
	maxRepin        int
	assembleTimeout time.Duration
	fsmMachine interface {
		Apply(string, string) (string, error)
		Can(string, string) bool
	}
}

   func New(cfg Config) *Engine {
	   if cfg.Clock == nil {
		  cfg.Clock = func() time.Time { return time.Now().UTC() }
	   }
	  if cfg.SnapLeaseTTL <= 0 {
		  cfg.SnapLeaseTTL = DefaultSnapLeaseTTL
	  }
	   if cfg.MaxRepin <= 0 {
		   cfg.MaxRepin = DefaultMaxRepin
	   }
	  if cfg.AssembleTimeout <= 0 {
		  cfg.AssembleTimeout = DefaultAssembleTimeout
	  }
	   return &Engine{
		   store:      cfg.Store,
		  waterlineQ: cfg.WaterlineQuerier,
		  billProv:     cfg.BillProvider,
		  clock:        cfg.Clock,
		  snapLeaseTTL: cfg.SnapLeaseTTL,
		   maxRepin:        cfg.MaxRepin,
		   assembleTimeout: cfg.AssembleTimeout,
		  fsmMachine:      LeaseStateMachine(),
	  }
   }

// ──────────────────────────────────────────
// PinSnap 为每科目拍观测世代，创建快照租约
// ──────────────────────────────────────────

  func (e *Engine) PinSnap(ctx context.Context, userID, asset string, ttl time.Duration, window string) (PinSnapResp, error) {
	  if e == nil {
		   return PinSnapResp{}, ErrNilEngine
	   }
	   e.mu.Lock()
	   defer e.mu.Unlock()
	  userID = strings.TrimSpace(userID)
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  if userID == "" {
		  return PinSnapResp{},
			  ErrUserInvalid
	  }
	  if asset == "" {
		  return PinSnapResp{}, ErrAssetInvalid
	  }
	   if ttl <= 0 {
		  ttl = e.snapLeaseTTL
	  }
	   if window == "" {
		   window = e.defaultWindow(userID, asset)
	   }
	   if sealed, err := e.store.GetSealedByWindow(ctx, userID, asset, window); err == nil {
		  now := e.clock()
		   if sealed.State == LeaseSealed && sealed.ExpiresAt.After(now) {
			  return PinSnapResp{
				   LeaseID: sealed.ID,
				   SnapGen: sealed.SnapGen,
				   State:     sealed.State,
				   BookSnaps: sealed.BookSnaps,
				  ExpiresAt: sealed.ExpiresAt,
				  Reused:    true,
			   }, nil
		   }
	   }
	  if active, err := e.store.GetActiveLease(ctx, userID, asset); err == nil {
		   now := e.clock()
		   if active.ExpiresAt.After(now) && active.State == LeasePinned {
			  return PinSnapResp{
				  LeaseID: active.ID,
				  SnapGen:   active.SnapGen,
				  State:     active.State,
				   BookSnaps: active.BookSnaps,
				   ExpiresAt: active.ExpiresAt,
				   Reused:    true,
			  }, nil
		   }
	  }
	  observations, err := e.observeAllBooks(ctx, userID, asset)
	   if err != nil {
		   return PinSnapResp{}, err
	   }
	   gen, err := e.store.LatestSnapGen(ctx, userID, asset)
	  if err != nil {
		   return PinSnapResp{}, err
	  }
	  gen++
	  now := e.clock()
	  snaps := make([]BookSnap, 0, len(observations))
	  for _, obs := range observations {
		  snaps = append(snaps, BookSnap{
			   Book:   obs.Book,
			   SnapID: uuid.NewString(),
			  ObserveGen: obs.ObserveGen,
			   Waterline:  obs.Waterline,
			   Checksum: obs.Checksum,
		   })
	   }
	  lease := BookSnapLease{
		  ID: uuid.NewString(),
		  UserID:     userID,
		   Asset:      asset,
		   SnapWindow: window,
		  State:      LeasePinned,
		  SnapGen:    gen,
		   BookSnaps:  snaps,
		  PinTTL:    Duration{Duration: ttl},
		   PinnedAt:  now,
		  ExpiresAt: now.Add(ttl),
		  MaxRepin:  e.maxRepin,
		   CreatedAt: now,
		   UpdatedAt: now,
	   }
	  saved, err := e.store.SaveLease(ctx, lease)
	   if err != nil {
		   return PinSnapResp{}, err
	   }
	  return PinSnapResp{
		   LeaseID: saved.ID,
		  SnapGen: saved.SnapGen,
		   State:   saved.State,
		  BookSnaps: saved.BookSnaps,
		  ExpiresAt: saved.ExpiresAt,
	  }, nil
   }

// ──────────────────────────────────────────
// BeginAssemble 开始组装（pinned → assembling）
// ──────────────────────────────────────────

  func (e *Engine) BeginAssemble(ctx context.Context, leaseID string) error {
	  if e == nil {
		   return ErrNilEngine
	   }
	  e.mu.Lock()
	  defer e.mu.Unlock()
	  lease, err := e.store.GetLease(ctx, leaseID)
	  if err != nil {
		   return err
	  }
	  now := e.clock()
	   if lease.ExpiresAt.Before(now) {
		  return e.transitionToExpired(ctx, lease, now)
	  }
	   if !CanAssemble(lease.State) {
		  return ErrLeaseNotPinned
	  }
	  newState, err := e.fsmMachine.Apply(string(lease.State), LeaseEvBeginAssemble)
	  if err != nil {
		  return ErrIllegalTransition
	  }
	   lease.State = LeaseState(newState)
	   lease.AssembleStart = &now
	   lease.UpdatedAt = now
	   _, err = e.store.SaveLease(ctx, lease)
	   return err
   }

// ──────────────────────────────────────────
// Assemble 在已 pin 的快照代次下执行全科目组装
// ──────────────────────────────────────────

  func (e *Engine) Assemble(ctx context.Context, leaseID string) (AssembleResp, error) {
	   if e == nil {
		  return AssembleResp{}, ErrNilEngine
	  }
	  e.mu.Lock()
	  defer e.mu.Unlock()
	   lease,
		   err := e.store.GetLease(ctx, leaseID)
	  if err != nil {
		   return AssembleResp{}, err
	   }
	  now := e.clock()
	   if lease.State == LeaseSealed && lease.SealedResult != nil && lease.ExpiresAt.After(now) {
		  return AssembleResp{
			   LeaseID: lease.ID,
			   State:   lease.State,
			   Result:  lease.SealedResult,
			  Reused:  true,
		  }, nil
	  }
	   if lease.ExpiresAt.Before(now) {
		   _ = e.transitionToExpired(ctx, lease, now)
		  return AssembleResp{}, ErrLeaseExpired
	   }
	  if lease.State != LeaseAssembling {
		  if lease.State == LeasePinned {
			   newState, fsmErr := e.fsmMachine.Apply(string(lease.State), LeaseEvBeginAssemble)
			  if fsmErr != nil {
				  return AssembleResp{},
					   ErrIllegalTransition
			   }
			   lease.State = LeaseState(newState)
			   lease.AssembleStart = &now
			   lease.UpdatedAt = now
			  lease, err = e.store.SaveLease(ctx, lease)
			  if err != nil {
				  return AssembleResp{}, err
			   }
		   } else if IsTerminal(lease.State) {
			  return AssembleResp{}, fmt.Errorf("%w: state=%s", ErrIllegalTransition, lease.State)
		  } else {
			  return AssembleResp{}, ErrLeaseNotAssembling
		  }
	   }
	  drifted, proof, err := e.checkDrift(ctx, lease, now)
	  if err != nil {
		   return AssembleResp{}, err
	   }
	  if drifted {
		   lease, _ = e.transitionToDrifted(ctx, lease, proof, now)
		   return AssembleResp{
			  LeaseID: lease.ID,
			   State: lease.State,
			   DriftProof: &proof,
		   }, ErrDriftDetected
	   }
	  conserve, err := e.assembleAndConserve(ctx, lease, now)
	   if err != nil {
		  return AssembleResp{}, err
	   }
	   recheck, recheckProof, err := e.checkDrift(ctx, lease, now)
	   if err != nil {
		   return AssembleResp{}, err
	   }
	   if recheck {
		  lease, _ = e.transitionToDrifted(ctx, lease, recheckProof, now)
		  return AssembleResp{
			   LeaseID: lease.ID,
			   State:   lease.State,
			  DriftProof: &recheckProof,
		  }, ErrDriftDetected
	   }
	  if !conserve.Conserved {
		   proof := e.buildConserveFailProof(lease, conserve, now)
		   savedProof, _ := e.store.SaveDriftProof(ctx, proof)
		   lease,
			  _ = e.transitionToDrifted(ctx, lease, savedProof, now)
		  return AssembleResp{
			   LeaseID: lease.ID,
			  State:      lease.State,
			   DriftProof: &savedProof,
		   }, ErrConserveFailed
	   }
	   lease.SealedResult = &conserve
	  sealedState, fsmErr := e.fsmMachine.Apply(string(lease.State), LeaseEvSeal)
	  if fsmErr != nil {
		   return AssembleResp{},
			  ErrIllegalTransition
	  }
	  lease.State = LeaseState(sealedState)
	  lease.SealedAt = &now
	   lease.UpdatedAt = now
	  lease, err = e.store.SaveLease(ctx, lease)
	   if err != nil {
		   return AssembleResp{}, err
	   }
	  return AssembleResp{
		   LeaseID: lease.ID,
		   State:  lease.State,
		  Result: &conserve,
	  }, nil
  }

// ──────────────────────────────────────────
// RePin 漂移后重新 pin
// ──────────────────────────────────────────

  func (e *Engine) RePin(ctx context.Context, leaseID string) (RePinResp, error) {
	  if e == nil {
		  return RePinResp{}, ErrNilEngine
	   }
	   e.mu.Lock()
	   defer e.mu.Unlock()
	   lease, err := e.store.GetLease(ctx, leaseID)
	   if err != nil {
		  return RePinResp{}, err
	  }
	  now := e.clock()
	  if lease.ExpiresAt.Before(now) {
		  _ = e.transitionToExpired(ctx, lease, now)
		  return RePinResp{}, ErrLeaseExpired
	  }
	   if !CanRepin(lease.State) {
		  return RePinResp{}, ErrRePinRequired
	  }
	  if lease.RepinCount >= lease.MaxRepin {
		  return RePinResp{}, ErrRepinExceeded
	   }
	   observations, err := e.observeAllBooks(ctx, lease.UserID, lease.Asset)
	  if err != nil {
		  return RePinResp{}, err
	   }
	  newState,
		  fsmErr := e.fsmMachine.Apply(string(lease.State), LeaseEvRepin)
	   if fsmErr != nil {
		  return RePinResp{}, ErrIllegalTransition
	   }
	   snaps := make([]BookSnap, 0, len(observations))
	  for _, obs := range observations {
		   snaps = append(snaps, BookSnap{
			  Book:   obs.Book,
			  SnapID: uuid.NewString(),
			  ObserveGen: obs.ObserveGen,
			   Waterline: obs.Waterline,
			  Checksum: obs.Checksum,
		   })
	   }
	  lease.State = LeaseState(newState)
	   lease.BookSnaps = snaps
	  lease.SnapGen++
	   lease.RepinCount++
	  lease.PinnedAt = now
	  lease.DriftedAt = nil
	   lease.DriftProofID = ""
	   lease.AssembleStart = nil
	  lease.UpdatedAt = now
	  saved, err := e.store.SaveLease(ctx, lease)
	  if err != nil {
		  return RePinResp{}, err
	  }
	   return RePinResp{
		   LeaseID: saved.ID,
		  SnapGen: saved.SnapGen,
		   State:   saved.State,
		   BookSnaps: saved.BookSnaps,
		  ExpiresAt:  saved.ExpiresAt,
		  RepinCount: saved.RepinCount,
	   }, nil
   }

// ──────────────────────────────────────────
// Abort 手动中止
// ──────────────────────────────────────────

  func (e *Engine) Abort(ctx context.Context, leaseID, reason string) error {
	  if e == nil {
		   return ErrNilEngine
	   }
	   e.mu.Lock()
	   defer e.mu.Unlock()
	   lease,
		  err := e.store.GetLease(ctx, leaseID)
	  if err != nil {
		   return err
	  }
	   if IsTerminal(lease.State) {
		   return nil
	  }
	   now := e.clock()
	   newState, fsmErr := e.fsmMachine.Apply(string(lease.State), LeaseEvAbort)
	   if fsmErr != nil {
		   return ErrIllegalTransition
	   }
	  if reason != "" {
		   proof := BookSnapDriftProof{
			  ID:      uuid.NewString(),
			  LeaseID: lease.ID,
			   UserID:  lease.UserID,
			  Asset:   lease.Asset,
			   SnapGen: lease.SnapGen,
			  Kind:    DriftKindAbort,
			  Detail:  reason,
			  CreatedAt: now,
		   }
		   _, _ = e.store.SaveDriftProof(ctx, proof)
	  }
	  lease.State = LeaseState(newState)
	   lease.AbortedAt = &now
	  lease.UpdatedAt = now
	   _, err = e.store.SaveLease(ctx, lease)
	  return err
   }

// ──────────────────────────────────────────
// GetLease 查询租约
// ──────────────────────────────────────────

   func (e *Engine) GetLease(ctx context.Context, leaseID string) (BookSnapLease, error) {
	   if e == nil {
		   return BookSnapLease{}, ErrNilEngine
	   }
	   return e.store.GetLease(ctx, leaseID)
  }

// ──────────────────────────────────────────
// ListLeases 查询用户租约
// ──────────────────────────────────────────

   func (e *Engine) ListLeases(ctx context.Context, userID string, limit int) ([]BookSnapLease, error) {
	  if e == nil {
		  return nil, ErrNilEngine
	   }
	   if limit <= 0 {
		   limit = 50
	  }
	  return e.store.ListLeasesByUser(ctx, userID, limit)
  }

// ──────────────────────────────────────────
// ExportProofs 导出漂移证明
// ──────────────────────────────────────────

   func (e *Engine) ExportProofs(ctx context.Context, leaseID, userID string, limit int) ([]BookSnapDriftProof, error) {
	  if e == nil {
		   return nil, ErrNilEngine
	  }
	   if limit <= 0 {
		   limit = 50
	   }
	   if leaseID != "" {
		   return e.store.ListDriftProofsByLease(ctx, leaseID, limit)
	  }
	   if userID != "" {
		  return e.store.ListDriftProofsByUser(ctx, userID, limit)
	  }
	  return nil, ErrUserInvalid
  }

// ──────────────────────────────────────────
// Health 健康统计
// ──────────────────────────────────────────

  func (e *Engine) Health(ctx context.Context) (HealthResp, error) {
	  if e == nil {
		  return HealthResp{}, ErrNilEngine
	   }
	   active, _ := e.store.CountByState(ctx, LeasePinned)
	  assembling, _ := e.store.CountByState(ctx, LeaseAssembling)
	   sealed, _ := e.store.CountByState(ctx, LeaseSealed)
	  drifted, _ := e.store.CountByState(ctx, LeaseDrifted)
	  proofs, _ := e.store.CountDriftProofs(ctx)
	  return HealthResp{
		  ActiveLeases:  active + assembling,
		   SealedLeases:  sealed,
		  DriftedLeases: drifted,
		   TotalDriftProofs: proofs,
	   }, nil
   }

// ──────────────────────────────────────────
// 内部方法
// ──────────────────────────────────────────

  func (e *Engine) observeAllBooks(ctx context.Context, userID, asset string) ([]WaterlineObservation, error) {
	   if e.waterlineQ != nil {
		   return e.waterlineQ.ObserveWaterlines(ctx, userID, asset)
	   }
	  books := account.AllBooks()
	   out := make([]WaterlineObservation, 0, len(books))
	  for _, b := range books {
		  out = append(out, WaterlineObservation{
			   Book:       string(b),
			  Waterline:  "0",
			   ObserveGen: 1,
			   Checksum:   waterlineChecksum(userID, asset, string(b), "0"),
		   })
	   }
	  return out, nil
  }

   func (e *Engine) checkDrift(ctx context.Context, lease BookSnapLease, now time.Time) (bool, BookSnapDriftProof, error) {
	   current, err := e.observeAllBooks(ctx, lease.UserID, lease.Asset)
	  if err != nil {
		  return false, BookSnapDriftProof{}, err
	   }
	   snapMap := make(map[string]BookSnap, len(lease.BookSnaps))
	  for _, bs := range lease.BookSnaps {
		   snapMap[bs.Book] = bs
	   }
	   for _, obs := range current {
		   pinned, ok := snapMap[obs.Book]
		  if !ok {
			  continue
		  }
		  if obs.Waterline != pinned.Waterline {
			  proof := BookSnapDriftProof{
				   ID: uuid.NewString(),
				   LeaseID: lease.ID,
				   UserID:  lease.UserID,
				  Asset:   lease.Asset,
				  SnapGen: lease.SnapGen,
				  Kind:    DriftKindWaterline,
				  Book: obs.Book,
				  OldWaterline: pinned.Waterline,
				  NewWaterline: obs.Waterline,
				  OldGen:       pinned.ObserveGen,
				   NewGen:    obs.ObserveGen,
				  Detail:    fmt.Sprintf("科目 %s 水线从 %s 漂移至 %s", obs.Book, pinned.Waterline, obs.Waterline),
				  CreatedAt: now,
			   }
			   saved, _ := e.store.SaveDriftProof(ctx, proof)
			   return true, saved, nil
		   }
		   if obs.ObserveGen != pinned.ObserveGen {
			   proof := BookSnapDriftProof{
				  ID:      uuid.NewString(),
				  LeaseID: lease.ID,
				   UserID:  lease.UserID,
				   Asset:   lease.Asset,
				  SnapGen: lease.SnapGen,
				  Kind: DriftKindGenMismatch,
				   Book:   obs.Book,
				  OldGen: pinned.ObserveGen,
				  NewGen: obs.ObserveGen,
				  Detail: fmt.Sprintf("科目 %s 观测世代从 %d 变为 %d", obs.Book, pinned.ObserveGen, obs.ObserveGen),
				  CreatedAt: now,
			  }
			   saved, _ := e.store.SaveDriftProof(ctx, proof)
			   return true, saved, nil
		  }
		   if obs.Checksum != pinned.Checksum {
			  proof := BookSnapDriftProof{
				   ID: uuid.NewString(),
				  LeaseID: lease.ID,
				   UserID:  lease.UserID,
				   Asset:   lease.Asset,
				  SnapGen: lease.SnapGen,
				   Kind:    DriftKindCreditHop,
				   Book:    obs.Book,
				  Detail:  fmt.Sprintf("科目 %s 校验和变化，疑似跨科目信用跳转", obs.Book),
				   CreatedAt: now,
			  }
			  saved, _ := e.store.SaveDriftProof(ctx, proof)
			   return true, saved, nil
		  }
	   }
	  return false, BookSnapDriftProof{}, nil
   }

   func (e *Engine) assembleAndConserve(ctx context.Context, lease BookSnapLease, now time.Time) (MultiConserve, error) {
	  result := MultiConserve{
		  UserID:    lease.UserID,
		  Asset:     lease.Asset,
		   SnapGen:   lease.SnapGen,
		  CheckedAt: now,
		  SnapWindow: lease.SnapWindow,
		   LeaseID:    lease.ID,
	  }
	  totalDebit, _ := money.ParseBook("0")
	  totalCredit, _ := money.ParseBook("0")
	   for _, snap := range lease.BookSnaps {
		   bills, err := e.fetchBillsForBook(ctx, lease.UserID, lease.Asset, snap.Book, lease.SnapGen)
		  if err != nil {
			   return MultiConserve{}, err
		  }
		   bookDebit, _ := money.ParseBook("0")
		   bookCredit, _ := money.ParseBook("0")
		   count := 0
		   for _, b := range bills {
			   if d, err := money.Parse(orZero(b.Debit)); err == nil && d.IsPositive() {
				   bookDebit, _ = bookDebit.Add(d)
				   count++
			   }
			  if c, err := money.Parse(orZero(b.Credit)); err == nil && c.IsPositive() {
				  bookCredit, _ = bookCredit.Add(c)
				  count++
			  }
		  }
		  result.Books = append(result.Books, BookTotals{
			  Book:   snap.Book,
			   Debit:  bookDebit.String(),
			   Credit: bookCredit.String(),
			  Count: count,
		  })
		   totalDebit, _ = totalDebit.Add(bookDebit)
		  totalCredit,
			  _ = totalCredit.Add(bookCredit)
	  }
	  result.TotalDebit = totalDebit.String()
	  result.TotalCredit = totalCredit.String()
	   delta, err := totalDebit.Sub(totalCredit)
	  if err == nil && delta.IsZero() {
		   result.Conserved = true
	  } else {
		  result.Conserved = false
		   if err == nil {
			  result.Delta = delta.String()
		   } else {
			  result.Delta = "compute_error"
		  }
	   }
	  return result, nil
   }

   func (e *Engine) fetchBillsForBook(ctx context.Context, userID, asset, book string, snapGen int64) ([]BillEntry, error) {
	   if e.billProv != nil {
		  return e.billProv.FetchBills(ctx, userID, asset, book, snapGen)
	   }
	  return nil, nil
   }

   func (e *Engine) transitionToExpired(ctx context.Context, lease BookSnapLease, now time.Time) error {
	   newState, fsmErr := e.fsmMachine.Apply(string(lease.State), LeaseEvExpire)
	  if fsmErr != nil {
		   return ErrIllegalTransition
	  }
	   expired := now
	   lease.State = LeaseState(newState)
	  lease.AbortedAt = &expired
	  lease.UpdatedAt = now
	   proof := BookSnapDriftProof{
		  ID: uuid.NewString(),
		  LeaseID: lease.ID,
		   UserID:  lease.UserID,
		   Asset:   lease.Asset,
		   SnapGen:   lease.SnapGen,
		   Kind:      DriftKindExpiry,
		  Detail:    "快照租约已过期",
		   CreatedAt: now,
	   }
	   _, _ = e.store.SaveDriftProof(ctx, proof)
	   _, err := e.store.SaveLease(ctx, lease)
	   return err
   }

  func (e *Engine) transitionToDrifted(ctx context.Context, lease BookSnapLease, proof BookSnapDriftProof, now time.Time) (BookSnapLease, error) {
	  newState, fsmErr := e.fsmMachine.Apply(string(lease.State), LeaseEvDrift)
	   if fsmErr != nil {
		  return lease, ErrIllegalTransition
	  }
	   lease.State = LeaseState(newState)
	   lease.DriftedAt = &now
	  lease.DriftProofID = proof.ID
	   lease.UpdatedAt = now
	   saved, err := e.store.SaveLease(ctx, lease)
	  if err != nil {
		   return lease, err
	  }
	  return saved, nil
   }

  func (e *Engine) buildConserveFailProof(lease BookSnapLease, conserve MultiConserve, now time.Time) BookSnapDriftProof {
	   return BookSnapDriftProof{
		   ID:      uuid.NewString(),
		  LeaseID: lease.ID,
		   UserID:  lease.UserID,
		   Asset: lease.Asset,
		   SnapGen:      lease.SnapGen,
		   Kind:         DriftKindConserveFail,
		  DebitBefore:  conserve.TotalDebit,
		   CreditBefore: conserve.TotalCredit,
		   Delta:  conserve.Delta,
		   Detail: fmt.Sprintf("多科目守恒校验失败: debit=%s credit=%s delta=%s", conserve.TotalDebit, conserve.TotalCredit, conserve.Delta),
		  CreatedAt: now,
	   }
  }

   func (e *Engine) defaultWindow(userID, asset string) string {
	   now := e.clock()
	  return userID + "|" + asset + "|" + now.Truncate(time.Hour).Format(time.RFC3339)
  }

  func waterlineChecksum(userID, asset, book, waterline string) string {
	  h := sha256.New()
	  h.Write([]byte(userID))
	  h.Write([]byte{0})
	  h.Write([]byte(asset))
	  h.Write([]byte{0})
	   h.Write([]byte(book))
	  h.Write([]byte{0})
	  h.Write([]byte(waterline))
	  return hex.EncodeToString(h.Sum(nil))
   }

   func orZero(s string) string {
	   s = strings.TrimSpace(s)
	   if s == "" {
		   return "0"
	  }
	   return s
   }
