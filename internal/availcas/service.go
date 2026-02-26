// 调用层（HTTP handler、overdraw guard、withdraw FSM 适配）通过 Service
// 访问所有 availcas 能力，不直接操作 barrier/store。

// Author: Bronson
// Date: 2021.02.17

package availcas

import (
	"context"
	"database/sql"
	"time"
	"github.com/shopspring/decimal"
)

// Service availcas 外观服务。
type Service struct {
	barrier Barrier
	lm      *leaseManager
	store Store
	clock func() time.Time
}

// ServiceConfig Service 构造配置。

type ServiceConfig struct {
	// DB 环境数据库连接（可选；传 nil 时必须传 MemStore）。
	DB *sql.DB
	// Store 覆盖存储层（主要用于注入 MemStore）。
	Store Store
	// Reader 账本只读观察接口（由 ledger 适配层实现）。
	Reader LeaseRequester
	// Clock 时钟函数，nil 时使用 time.Now.UTC。
	Clock func() time.Time
}

// NewService 构造 Service。
// 若 cfg.Store 为 nil 且 cfg.DB 非 nil，自动构造 SQLStore；
// 若两者均非 nil，以 cfg.Store 为准。
   func NewService(cfg ServiceConfig) *Service {
	  store := cfg.Store
	   if store == nil && cfg.DB != nil {
		  store = NewSQLStore(cfg.DB)
	  }
	  clock := cfg.Clock
	   if clock == nil {
		   clock = func() time.Time { return time.Now().UTC() }
	   }
	   lm := newLeaseManager(store, cfg.Reader, clock)
	  return &Service{
		   barrier: newBarrierWithClock(store, cfg.Reader, clock),
		  lm:      lm,
		   store:   store,
		   clock:   clock,
	  }
   }

// ─── 租约 API ─────────────────────────────────────────────────────────────────
// IssueLease 为 userID×asset×book 颁发新可用快照租约。
  func (s *Service) IssueLease(ctx context.Context, userID, asset, book string, opts LeaseOptions) (*AvailLease, error) {
	   return s.barrier.IssueLease(ctx, userID, asset, book, opts)
  }

// GetLease 按 ID 查询租约。
   func (s *Service) GetLease(ctx context.Context, leaseID string) (*AvailLease, error) {
	   return s.store.GetLease(ctx,
		   leaseID)
  }

// ─── 意图/屏障 API ────────────────────────────────────────────────────────────
// Submit 提交扣减/冻结意图，执行 CAS 裁决。
// 供预检闸（precheck gate）与提交闸（commit gate）共用。
   func (s *Service) Submit(ctx context.Context, intent *DebitIntent) (*BarrierDecision, error) {
	  return s.barrier.Submit(ctx,
		   intent)
   }

// GetDecision 查询已有意图的裁决结果。

   func (s *Service) GetDecision(ctx context.Context, intentID string) (*BarrierDecision, error) {
	  return s.barrier.GetDecision(ctx,
		  intentID)
  }

// ─── 竞态证明 API ─────────────────────────────────────────────────────────────
// GetProof 按 ID 查询竞态证明（desk 查询）。

  func (s *Service) GetProof(ctx context.Context, proofID string) (*RaceProof, error) {
	   return s.barrier.GetProof(ctx, proofID)
  }

// ListProofs 列出用户×资产的竞态证明（desk 面板用）。
  func (s *Service) ListProofs(ctx context.Context, userID, asset string, limit int) ([]*RaceProof, error) {
	   return s.barrier.ListProofs(ctx,
		   userID, asset, limit)
  }

// ─── 运维 API ────────────────────────────────────────────────────────────────
// SnapshotAvailable 计算并返回用户×资产×账本位的即时净可用余额。
// 不颁发租约，仅供只读查询（如 desk 余额展示）。
  func (s *Service) SnapshotAvailable(ctx context.Context, userID, asset, book string) (decimal.Decimal, error) {
	   // 借用 leaseManager 的折叠逻辑，但不落库租约。
	  bal, err := s.lm.reader.ReadBalance(ctx, userID, asset, book)
	   if err != nil {
		   return decimal.Zero, err
	   }
	  frozen, err := s.lm.reader.ReadFrozen(ctx, userID, asset, book)
	  if err != nil {
		   return decimal.Zero, err
	  }
	  reserved, err := s.lm.reader.ReadReserved(ctx, userID, asset)
	  if err != nil {
		  return decimal.Zero, err
	   }
	   inFlight,
		  err := s.store.SumInFlightIntents(ctx, userID, asset, book)
	   if err != nil {
		   return decimal.Zero, err
	   }
	  avail := bal.Sub(frozen).Sub(reserved).Sub(inFlight)
	   if avail.IsNegative() {
		  avail = decimal.Zero
	  }
	  return avail, nil
  }

// ─── 内部访问（Worker 使用）───────────────────────────────────────────────────
// Store 返回内部 Store（Worker 构造时使用）。
  func (s *Service) Store() Store { return s.store }
