package hotbreach

import (
	"context"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/persist"
)

// Service hotbreach 统一服务。
type Service struct {
	store *Store
	lm    *LeaseManager
	detector *BreachDetector
	guard *DrainGuard
	patrol   *BreachPatrol
	clearing *BreachClearing
	clock    func() time.Time
}

// NewService 构造 Service。
// kv 为 nil 时使用内存 KV（/单机）。
// hot 为 nil 时跳过热库存快照读取（仅逻辑）。

   func NewService(kv persist.KV, hot *hotinv.Book, ledSt ledger.Store) *Service {
	   store := NewStore(kv)
	  clock := func() time.Time { return time.Now().UTC() }
	  lm := NewLeaseManager(store, hot, ledSt)
	   det := NewBreachDetector(store, hot, ledSt)
	   guard := NewDrainGuard(store, hot, ledSt)
	  patrol := NewBreachPatrol(store,
		   hot, ledSt)
	  clearing := NewBreachClearing(store)
	   return &Service{
		  store: store,
		  lm:       lm,
		   detector: det,
		  guard: guard,
		  patrol:   patrol,
		   clearing: clearing,
		   clock:    clock,
	   }
  }

// WithClock 注入可控时钟（用）。
  func (s *Service) WithClock(c func() time.Time) *Service {
	  if s == nil || c == nil {
		   return s
	  }
	   s.clock = c
	   s.store.WithClock(c)
	   s.lm.WithClock(c)
	  s.detector.WithClock(c)
	   s.guard.WithClock(c)
	  s.patrol.WithClock(c)
	  s.clearing.WithClock(c)
	  return s
   }

  // ─────────────────────────────────────────────
   // 租约操作
   // ─────────────────────────────────────────────
  // IssueLease 签发预检世代租约（PreSubmit 通过后调用）。
   func (s *Service) IssueLease(ctx context.Context, req IssueLeaseReq) (PreLease, error) {
	   if s == nil {
		   return PreLease{}, fmt.Errorf("hotbreach: service not initialized")
	   }
	  return s.lm.IssueLease(ctx, req)
   }

// ConsumeLease 消费租约（出金/归集实际预扣前）。

  func (s *Service) ConsumeLease(ctx context.Context, req ConsumeLeaseReq) (PreLease, error) {
	  if s == nil {
		   return PreLease{}, fmt.Errorf("hotbreach: service not initialized")
	   }
	   return s.lm.ConsumeLease(ctx, req)
   }

// RevokeLease 管理员撤销租约。
   func (s *Service) RevokeLease(ctx context.Context, leaseID, reason string) error {
	  if s == nil {
		   return fmt.Errorf("hotbreach: service not initialized")
	   }
	   return s.lm.RevokeLease(ctx, leaseID, reason)
  }

// GetLease 按 ID 查询租约状态。
  func (s *Service) GetLease(ctx context.Context, leaseID string) (LeaseStatusResp, error) {
	   if s == nil {
		  return LeaseStatusResp{}, fmt.Errorf("hotbreach: service not initialized")
	  }
	  lease,
		   err := s.store.GetLease(ctx, leaseID)
	  if err != nil {
		  return LeaseStatusResp{}, err
	  }
	  resp := LeaseStatusResp{Lease: &lease}
	   // 检查是否被击穿挡写
	  blocked, breachID, cerr := s.store.HasActiveBreachForAsset(ctx, lease.Asset)
	  if cerr == nil && blocked {
		   resp.BlockedBy = breachID
	  }
	  return resp, nil
   }

// ListLeases 查询某 asset 的租约列表（ops）。

  func (s *Service) ListLeases(ctx context.Context, asset string, limit int) ([]PreLease, error) {
	   if s == nil {
		  return nil, fmt.Errorf("hotbreach: service not initialized")
	   }
	   return s.store.ListLeasesByAsset(ctx, asset, limit)
  }

// ExpireLeases 过期回收 worker 调用入口。
   func (s *Service) ExpireLeases(ctx context.Context) (int, error) {
	   if s == nil {
		   return 0, nil
	  }
	   return s.lm.ExpireLeases(ctx)
   }

// ─────────────────────────────────────────────
// 挡写守卫
// ─────────────────────────────────────────────
// Guard 对出金/归集预扣调用点实施挡写检查。
// 返回 nil 表示允许；否则为拒绝原因（ErrBreachGated / ErrNoLease / ErrFenceConflict 等）。

   func (s *Service) Guard(ctx context.Context, req GuardReq) error {
	   if s == nil {
		  return fmt.Errorf("hotbreach: service not initialized")
	   }
	  return s.guard.Guard(ctx, req)
   }

// ─────────────────────────────────────────────
// 击穿检测与查询
// ─────────────────────────────────────────────
// Detect 主动检测某 asset 是否存在击穿。

  func (s *Service) Detect(ctx context.Context, asset, leaseID, concurrentParty string) (DetectResult, error) {
	  if s == nil {
		   return DetectResult{}, fmt.Errorf("hotbreach: service not initialized")
	   }
	   return s.detector.Detect(ctx, asset, leaseID, concurrentParty)
  }

// GetBreach 按 ID 查询击穿案例。

  func (s *Service) GetBreach(ctx context.Context, breachID string) (BreachCase, error) {
	  if s == nil {
		  return BreachCase{}, fmt.Errorf("hotbreach: service not initialized")
	   }
	   return s.store.GetBreach(ctx, breachID)
   }

// QueryBreaches 查询某 asset 的击穿列表（含证明）。

  func (s *Service) QueryBreaches(ctx context.Context, asset string, withProofs bool, limit int) (BreachQueryResp, error) {
	   if s == nil {
		  return BreachQueryResp{}, fmt.Errorf("hotbreach: service not initialized")
	  }
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  cases, err := s.store.ListBreachesByAsset(ctx, asset, limit)
	   if err != nil {
		   return BreachQueryResp{}, err
	   }
	   resp := BreachQueryResp{Cases: cases, Total: len(cases)}
	  if withProofs {
		   for _, bc := range cases {
			   proofs, perr := s.store.ListProofsByBreach(ctx, bc.ID, 10)
			   if perr == nil {
				  resp.Proofs = append(resp.Proofs, proofs...)
			  }
		   }
	  }
	   return resp, nil
   }

// ─────────────────────────────────────────────
// 巡检与解除
// ─────────────────────────────────────────────
// Patrol 一轮击穿巡检（open→gated 推进）。
  func (s *Service) Patrol(ctx context.Context) (PatrolReport, error) {
	  if s == nil {
		  return PatrolReport{}, nil
	  }
	  return s.patrol.Patrol(ctx)
  }

// ClearBreach 四眼解除击穿挡写。

  func (s *Service) ClearBreach(ctx context.Context, req ClearBreachReq) (BreachCase, error) {
	  if s == nil {
		   return BreachCase{}, fmt.Errorf("hotbreach: service not initialized")
	  }
	   return s.clearing.ClearBreach(ctx, req)
  }

// GetProof 按 ID 查询击穿证明。
   func (s *Service) GetProof(ctx context.Context, proofID string) (BreachProof, error) {
	  if s == nil {
		   return BreachProof{}, fmt.Errorf("hotbreach: service not initialized")
	   }
	   return s.store.GetProof(ctx, proofID)
  }

// InjectBreachCase 直接注入击穿案例（ops 手动建档 / 辅助）。
// 注入后状态为 open；调用 Patrol 可推进至 gated。
   func (s *Service) InjectBreachCase(ctx context.Context, asset, leaseID, concurrentParty string) (BreachCase, error) {
	  if s == nil {
		  return BreachCase{}, fmt.Errorf("hotbreach: service not initialized")
	  }
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  if asset == "" {
		  return BreachCase{}, fmt.Errorf("%w: asset required", ErrInvalidInput)
	  }
	   now := s.clock()
	   cpHash := hashParty(concurrentParty)
	  bc := BreachCase{
		  ID: generateID(),
		   Asset:               asset,
		   LeaseID:             leaseID,
		  ConcurrentParty:     concurrentParty,
		   ConcurrentPartyHash: cpHash,
		  State:               BreachOpen,
		   DetectedAt: now,
	   }
	  if err := s.store.SaveBreach(ctx, bc); err != nil {
		  return BreachCase{}, fmt.Errorf("hotbreach: inject breach: %w", err)
	   }
	  return bc, nil
  }
