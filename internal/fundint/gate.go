package fundint

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// GateStore 解析快照与借记租约的持久化（由 intxfergate.MemStore / SQLStore 实现）。

type GateStore interface {
	// ResolveSnap CRUD
	InsertResolveSnap(ctx context.Context, s *ResolveSnap) error
	GetResolveSnapByIdem(ctx context.Context, idemKey string) (*ResolveSnap, error)
	UpdateResolveSnap(ctx context.Context, s *ResolveSnap) error
	// DebitLease CRUD
	InsertDebitLease(ctx context.Context,
		l *DebitLease) error
	GetDebitLeaseByIdem(ctx context.Context, idemKey string) (*DebitLease, error)
	UpdateDebitLease(ctx context.Context, l *DebitLease) error
	ListExpiredLeases(ctx context.Context, before time.Time, limit int) ([]*DebitLease, error)
	ListFreshSnapsByState(ctx context.Context, state ResolveSnapState, limit int) ([]*ResolveSnap, error)
}

// ResolveGate 解析世代闸主结构。

type ResolveGate struct {
	store GateStore
	snapTTL  time.Duration // 快照有效期，默认 5min
	leaseTTL time.Duration // 租约有效期，默认 30s
}

// NewResolveGate 创建解析世代闸。

   func NewResolveGate(store GateStore, opts ...GateOption) *ResolveGate {
	  g := &ResolveGate{
		  store: store,
		   snapTTL: 5 * time.Minute,
		   leaseTTL: 30 * time.Second,
	  }
	   for _, o := range opts {
		   o(g)
	  }
	  return g
  }

// GateOption 参数化配置。

type GateOption func(*ResolveGate)

// WithSnapTTL 覆盖快照有效期。

  func WithSnapTTL(d time.Duration) GateOption {
	  return func(g *ResolveGate) { g.snapTTL = d }
   }

// WithLeaseTTL 覆盖租约有效期。

   func WithLeaseTTL(d time.Duration) GateOption {
	   return func(g *ResolveGate) { g.leaseTTL = d }
  }

// SnapRequest 快照申请参数。
type SnapRequest struct {
	FromUserID string
	ToUserID   string
	Channel    string // 首个命中渠道
	Channels string // 全部参与渠道（逗号分隔）
	IdemKey string
	Asset string
	Amount string
}

// RecordSnap 记录一次解析世代快照。
// 同 idem_key 若已有 fresh 快照，直接返回（幂等）。
// 已有 expired/deviated/closed 快照时，创建新世代（gen+1）。

  func (g *ResolveGate) RecordSnap(ctx context.Context, req SnapRequest) (*ResolveSnap, error) {
	  now := time.Now().UTC()
	   // 检查是否已有快照
	   exist, err := g.store.GetResolveSnapByIdem(ctx, req.IdemKey)
	   if err != nil && !errors.Is(err, ErrSnapNotFound) {
		  return nil, fmt.Errorf("gate: get existing snap: %w", err)
	   }
	  var gen int64 = 1
	  if exist != nil {
		  if exist.State == ResolveSnapFresh && exist.ToUserID == req.ToUserID {
			   // 同结果幂等命中
			   return exist, nil
		  }
		   if exist.State == ResolveSnapFresh && exist.ToUserID != req.ToUserID {
			   // 同 idem_key 解析到不同用户，视为歧义
			  return nil, fmt.Errorf("%w: idem_key=%s prev=%s new=%s",
				  ErrAmbiguous, req.IdemKey, exist.ToUserID, req.ToUserID)
		  }
		   gen = exist.Gen + 1
	  }
	  snap := &ResolveSnap{
		   ID:         uuid.NewString(),
		   FromUserID: req.FromUserID,
		   ToUserID: req.ToUserID,
		   Channel:  req.Channel,
		   Channels: req.Channels,
		   IdemKey:  req.IdemKey,
		  Gen: gen,
		   Asset: req.Asset,
		   Amount: req.Amount,
		   State:      ResolveSnapFresh,
		  ResolvedAt: now,
		   ExpiresAt: now.Add(g.snapTTL),
		   CreatedAt: now,
		   UpdatedAt: now,
	  }
	   if err := g.store.InsertResolveSnap(ctx, snap); err != nil {
		  return nil, fmt.Errorf("gate: insert snap: %w", err)
	  }
	  return snap, nil
  }

// ValidateSnap 校验快照仍在有效期内可用。

  func (g *ResolveGate) ValidateSnap(ctx context.Context,
	   idemKey, expectedTo string, now time.Time) (*ResolveSnap, error) {
	  snap,
		  err := g.store.GetResolveSnapByIdem(ctx, idemKey)
	   if err != nil {
		  return nil, fmt.Errorf("gate: get snap: %w", err)
	   }
	  if err := snap.Valid(now); err != nil {
		  return nil, err
	  }
	   if !strings.EqualFold(snap.ToUserID, expectedTo) {
		   return nil, fmt.Errorf("%w: snap_to=%s actual=%s",
			   ErrAmbiguous, snap.ToUserID, expectedTo)
	   }
	  return snap, nil
   }

// AcquireLease 申请借记互斥租约。
// 同 idem_key 已有 held/committed 租约则返回 ConflictError；
// 已有 committed 且 LedgerID 匹配时幂等返回已有租约。
   func (g *ResolveGate) AcquireLease(ctx context.Context, snapID, idemKey, from, to, asset, amount string) (*DebitLease, error) {
	  now := time.Now().UTC()
	  exist, err := g.store.GetDebitLeaseByIdem(ctx, idemKey)
	  if err != nil && !errors.Is(err, ErrLeaseNotFound) {
		   return nil, fmt.Errorf("gate: get existing lease: %w", err)
	   }
	  if exist != nil {
		   switch exist.State {
		   case DebitLeaseHeld:
			  return nil, &ConflictError{LeaseID: exist.ID, IdemKey: idemKey}
		   case DebitLeaseCommitted:
			  // 幂等：已提交
			   return exist, nil
		   case DebitLeaseConflict:
			   return nil, &ConflictError{LeaseID: exist.ID, IdemKey: idemKey}
		  }
	   }
	  lease := &DebitLease{
		   ID:         uuid.NewString(),
		  IdemKey:    idemKey,
		  SnapID:     snapID,
		  FromUserID: from,
		   ToUserID: to,
		   Asset: asset,
		   Amount: amount,
		  State:  DebitLeaseHeld,
		  AcquiredAt: &now,
		  ExpiresAt: now.Add(g.leaseTTL),
		  CreatedAt: now,
		   UpdatedAt: now,
	   }
	   if err := g.store.InsertDebitLease(ctx, lease); err != nil {
		  return nil, fmt.Errorf("gate: insert lease: %w", err)
	  }
	  return lease, nil
   }

// CommitLease 将 held 租约提升为 committed，写入 ledger transfer ID。
  func (g *ResolveGate) CommitLease(ctx context.Context, leaseID, ledgerID string) error {
	  // 通过 GetDebitLeaseByIdem 无法按 ID 查，此处约定调用方传入已有 lease 对象后就地更新。
	   // 实际实现中 intxfergate.Store 会提供 GetDebitLeaseByID。
	   return g.commitLeaseByID(ctx, leaseID, ledgerID)
   }

  func (g *ResolveGate) commitLeaseByID(ctx context.Context, leaseID, ledgerID string) error {
	  // 此处通过 store 类型断言获取扩展接口（若存在），否则降级实现。
	  type byIDGetter interface {
		  GetDebitLeaseByID(ctx context.Context, id string) (*DebitLease, error)
	   }
	  bg, ok := g.store.(byIDGetter)
	   if !ok {
		   return fmt.Errorf("gate: store does not support GetDebitLeaseByID")
	  }
	   lease,
		  err := bg.GetDebitLeaseByID(ctx, leaseID)
	   if err != nil {
		   return fmt.Errorf("gate: get lease by id: %w", err)
	   }
	   if lease.State != DebitLeaseHeld {
		   return fmt.Errorf("gate: cannot commit lease state=%s", lease.State)
	   }
	   now := time.Now().UTC()
	  lease.State = DebitLeaseCommitted
	  lease.LedgerID = ledgerID
	   lease.CommittedAt = &now
	  lease.UpdatedAt = now
	  return g.store.UpdateDebitLease(ctx, lease)
  }

// ExpireOldLeases 由 patrol worker 定期调用：释放已过 TTL 的 held 租约。

   func (g *ResolveGate) ExpireOldLeases(ctx context.Context, now time.Time, limit int) (int, error) {
	   leases,
		  err := g.store.ListExpiredLeases(ctx, now, limit)
	  if err != nil {
		  return 0, fmt.Errorf("gate: list expired leases: %w", err)
	   }
	   n := 0
	   for _, l := range leases {
		  if l.State != DebitLeaseHeld {
			  continue
		   }
		   l.State = DebitLeaseReleased
		   l.UpdatedAt = now
		   if err := g.store.UpdateDebitLease(ctx, l); err != nil {
			   continue
		  }
		   n++
	   }
	  return n, nil
  }

// ExpireOldSnaps 由 patrol worker 定期调用：标记已过 TTL 的 fresh 快照为 expired。

  func (g *ResolveGate) ExpireOldSnaps(ctx context.Context, now time.Time, limit int) (int, error) {
	  snaps, err := g.store.ListFreshSnapsByState(ctx, ResolveSnapFresh, limit)
	  if err != nil {
		   return 0, fmt.Errorf("gate: list fresh snaps: %w", err)
	  }
	  n := 0
	   for _, s := range snaps {
		  if !now.After(s.ExpiresAt) {
			  continue
		   }
		   s.State = ResolveSnapExpired
		  s.UpdatedAt = now
		  if err := g.store.UpdateResolveSnap(ctx, s); err != nil {
			  continue
		   }
		   n++
	   }
	   return n, nil
   }

// ErrSnapNotFound 快照不存在。

var ErrSnapNotFound = fmt.Errorf("resolve snap not found")

// ErrLeaseNotFound 租约不存在。
var ErrLeaseNotFound = fmt.Errorf("debit lease not found")
