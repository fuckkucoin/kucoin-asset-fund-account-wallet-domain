package feepathgate

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// GateReq 是 Gate.CheckAndAcquire 的入参，
// 包含完整的业务语境供 FSM 和 Replay 层使用。

type GateReq struct {
	// BusinessKey 路径互斥的核心键；由调用方（feecredit）根据
	// user_id + asset + "base_idem" 派生，确保同一笔交易跨 Kind 共享。
	BusinessKey string `json:"business_key"`
	// IdemKey 当次 Post 的幂等键（feecredit.PostReq.IdempotencyKey）。
	IdemKey string `json:"idem_key"`
	Kind  Kind   `json:"kind"`
	Asset string `json:"asset"`
	Gross string `json:"gross"`
	// WorkerID 调用方身份标识，用于后续 Bind/Release 鉴权。
	WorkerID string `json:"worker_id"`
}

// GateResult CheckAndAcquire 成功时的返回值。
type GateResult struct {
	Lease     *PathLease `json:"lease"`
	Duplicate bool       `json:"duplicate"` // true 表示幂等重放，LeaseID 复用已有租约
}

// PostConfirm feecredit 落账完成后回调 Gate.Confirm 的入参。

type PostConfirm struct {
	LeaseID string `json:"lease_id"`
	CreditID string `json:"credit_id"`
	LedgerID string `json:"ledger_id"`
	Fee      string `json:"fee"`
	Net string `json:"net"`
	WorkerID string `json:"worker_id"`
}

// Service 是路径互斥引擎的顶层门面。
type Service struct {
	fsm *LeaseFSM
	detect *DualHitDetector
	vault   *HedgeVault
	replay  *ReplayFence
	metrics *GateMetrics
	store Store
	clock func() time.Time
}

// NewService 创建 Service。store 可以为 nil（单测场景使用内存视图）。

   func NewService(store Store) *Service {
	   if store == nil {
		  store = NewMemStore()
	  }
	  clock := func() time.Time { return time.Now().UTC() }
	   return &Service{
		   fsm: newLeaseFSM(store, clock),
		   detect: newDetector(store,
			  clock),
		   vault: newHedgeVault(store, clock),
		  replay:  newReplayFence(store, clock),
		  metrics: newGateMetrics(),
		  store:   store,
		   clock: clock,
	   }
  }

// WithClock 替换时钟（注入）。
   func (s *Service) WithClock(clock func() time.Time) *Service {
	   s.clock = clock
	  s.fsm.clock = clock
	   s.detect.clock = clock
	  s.vault.clock = clock
	   s.replay.clock = clock
	  return s
   }

   // CheckAndAcquire 是进入 feecredit.Post 前的路径互斥门检。
  // 成功返回持有路径租约的 GateResult；调用方落账成功后必须调用 Confirm。
  // 失败（ErrPathConflict / ErrStaleGen / ErrReplayConflict）时调用方必须中止，
  // 不得发起 ledger 贷记。

  func (s *Service) CheckAndAcquire(ctx context.Context, req GateReq) (GateResult, error) {
	  if err := s.validateGateReq(req); err != nil {
		   return GateResult{}, err
	   }
	   // 1. 幂等 Replay 围栏：跨 Kind 三元组不等 → ErrReplayConflict。
	   dup, err := s.replay.CheckAndRegister(ctx,
		  req.IdemKey, req.BusinessKey, req.Kind, req.Asset, req.Gross)
	   if err != nil {
		   s.metrics.replayConflict.Add(1)
		   return GateResult{}, err
	  }
	  if dup {
		   // 幂等重放：尝试找到已有租约。
		  leases, lerr := s.fsm.GetByBizKey(ctx, req.BusinessKey)
		  if lerr == nil {
			   for _, l := range leases {
				  if l.Kind == req.Kind {
					   s.metrics.idempotentReplay.Add(1)
					   return GateResult{Lease: l, Duplicate: true}, nil
				  }
			   }
		   }
	   }
	   // 2. 路径世代 CAS 互斥 Acquire。
	  lease, err := s.fsm.Acquire(ctx, AcquireReq{
		   BusinessKey: req.BusinessKey,
		   IdemKey: req.IdemKey,
		  Kind:  req.Kind,
		   Asset: req.Asset,
		  Gross: req.Gross,
		  WorkerID: req.WorkerID,
	  })
	  if err != nil {
		   switch err {
		   case ErrPathConflict:
			   s.metrics.pathConflict.Add(1)
		   case ErrStaleGen:
			   s.metrics.staleGen.Add(1)
		  }
		   return GateResult{}, err
	   }
	  s.metrics.leaseAcquired.Add(1)
	   return GateResult{Lease: lease}, nil
   }

// Confirm feecredit 落账完成后调用，将 open 租约绑定 CreditID/LedgerID 并推进至 bound。
// 若落账失败（即调用方决定不落账），应调用 Abort 而非 Confirm。
   func (s *Service) Confirm(ctx context.Context, c PostConfirm) (*PathLease, error) {
	   if strings.TrimSpace(c.LeaseID) == "" {
		   return nil, ErrLeaseNotFound
	  }
	  lease, err := s.fsm.Bind(ctx, BindReq{
		   LeaseID:  c.LeaseID,
		  CreditID: c.CreditID,
		  LedgerID: c.LedgerID,
		   Fee:      c.Fee,
		  Net:      c.Net,
		  WorkerID: c.WorkerID,
	  })
	  if err != nil {
		  return nil, err
	   }
	  s.metrics.leaseBound.Add(1)
	  return lease, nil
   }

// Abort 在落账失败时将 open 租约标记为 conflicted 并释放路径占用。
// 调用方有责任在 feecredit.Post 返回错误时调用此方法。

  func (s *Service) Abort(ctx context.Context, leaseID string) error {
	  if strings.TrimSpace(leaseID) == "" {
		   return nil
	   }
	   _, err := s.fsm.MarkConflicted(ctx, leaseID)
	   if err != nil && err != ErrLeaseNotFound {
		   return err
	  }
	   s.metrics.leaseAborted.Add(1)
	  return nil
  }

// ScanForDualHit 扫描指定 business_key 下的所有租约，检测是否已形成双贷。
// 由后台 worker 定期调用，也可由 ops 手动触发。

  func (s *Service) ScanForDualHit(ctx context.Context, bizKey string) (*DualHit, error) {
	  leases, err := s.fsm.GetByBizKey(ctx, bizKey)
	  if err != nil {
		  return nil, err
	  }
	  // 也从 Store 补全已 settled/conflicted 的租约（bound 态可能已推进）。
	   stored, _ := s.store.ListLeasesByBizKey(ctx, bizKey)
	  // 合并，以 ID 为键去重。
	   seen := make(map[string]*PathLease)
	  for _, l := range leases {
		   seen[l.ID] = l
	   }
	   for _, l := range stored {
		  if _, ok := seen[l.ID]; !ok {
			   seen[l.ID] = l
		   }
	  }
	  var takeL, rebateL *PathLease
	  for _, l := range seen {
		   if l.Kind == KindTake && (l.Status == LeaseBound || l.Status == LeaseSettled) {
			  takeL = l
		  }
		   if l.Kind == KindRebate && (l.Status == LeaseBound || l.Status == LeaseSettled) {
			   rebateL = l
		   }
	  }
	  return s.detect.Scan(ctx, takeL, rebateL)
   }

// BuildHedgeProof 尝试为一对 bound 租约构建对冲证明。
// 返回已有或新建的 HedgeProof；若对冲不平衡返回 ErrHedgeImbalance。
  func (s *Service) BuildHedgeProof(ctx context.Context, req ProofBuildReq) (*HedgeProof, error) {
	   takeL, err := s.fsm.GetLease(ctx, req.TakeLeaseID)
	   if err != nil {
		   return nil, fmt.Errorf("take lease: %w", err)
	  }
	  rebateL, err := s.fsm.GetLease(ctx, req.RebateLeaseID)
	   if err != nil {
		  return nil, fmt.Errorf("rebate lease: %w", err)
	   }
	   proof, err := s.vault.Build(ctx, takeL, rebateL)
	  if err != nil {
		   s.metrics.hedgeImbalance.Add(1)
		   return nil, err
	  }
	   return proof, nil
   }

// VerifyAndSettle 验证对冲证明并将两条租约推进至 settled 终态。
// 若证明验证失败，租约保持 bound 态，等待人工介入或再次触发验证。
   func (s *Service) VerifyAndSettle(ctx context.Context, proofID string) (*HedgeProof, error) {
	   proof,
		   err := s.vault.Verify(ctx, proofID)
	  if err != nil {
		   s.metrics.proofVerifyFail.Add(1)
		   return nil, err
	   }
	  // Settle 两条租约。
	  if _, err := s.fsm.Settle(ctx, SettleReq{
		  LeaseID: proof.TakeLeaseID,
		   ProofID: proofID,
	   }); err != nil && err != ErrLeaseNotFound {
		   return proof, fmt.Errorf("settle take lease: %w", err)
	   }
	  if _, err := s.fsm.Settle(ctx, SettleReq{
		  LeaseID: proof.RebateLeaseID,
		  ProofID: proofID,
	  }); err != nil && err != ErrLeaseNotFound {
		  return proof, fmt.Errorf("settle rebate lease: %w", err)
	   }
	   s.metrics.proofVerified.Add(1)
	  s.metrics.leaseSettled.Add(2)
	  return proof, nil
  }

// ResolveDualHit 人工双人确认关闭 DualHit。
   func (s *Service) ResolveDualHit(ctx context.Context, req DualHitResolveReq) (*DualHit, error) {
	  return s.detect.Resolve(ctx, req)
   }

// GetLease 按 lease_id 查询租约详情。
   func (s *Service) GetLease(ctx context.Context, leaseID string) (*PathLease, error) {
	  return s.fsm.GetLease(ctx, leaseID)
   }

// ListLeasesByBizKey 按 business_key 查询所有活跃租约。
  func (s *Service) ListLeasesByBizKey(ctx context.Context, bizKey string) ([]*PathLease, error) {
	   return s.fsm.GetByBizKey(ctx, bizKey)
  }

// GetDualHit 按 business_key 查询当前活跃 DualHit。
   func (s *Service) GetDualHit(ctx context.Context, bizKey string) (*DualHit, error) {
	  return s.detect.GetByBizKey(ctx,
		  bizKey)
   }

// GetHedgeProof 按 proof_id 查询对冲证明。

  func (s *Service) GetHedgeProof(ctx context.Context, proofID string) (*HedgeProof, error) {
	  return s.vault.GetByProofID(ctx,
		  proofID)
   }

// ExportProof 导出指定 proof_id 的证明记录（完整 JSON 可用于核验存档）。

  func (s *Service) ExportProof(ctx context.Context, proofID string) (*HedgeProof, error) {
	  p, err := s.vault.GetByProofID(ctx, proofID)
	   if err != nil {
		  return nil, err
	   }
	  // 重新计算摘要并核对（Export 时做一次在线验签）。
	  computed := p.ComputeDigest()
	   if computed != p.Digest {
		   return p, ErrProofVerify
	   }
	   return p, nil
   }

// GetReplayBlock 查询 idem_key 的封堵记录。
   func (s *Service) GetReplayBlock(ctx context.Context, idemKey string) (*ReplayBlock, error) {
	   return s.replay.GetBlock(ctx, idemKey)
  }

// Stats 返回当前引擎统计快照（供 /healthz 或 Prometheus 使用）。

   func (s *Service) Stats() map[string]int64 {
	   knownCount, blockCount := s.replay.Stats()
	  snap := s.metrics.Snapshot()
	  snap["replay_known"] = int64(knownCount)
	   snap["replay_blocks"] = int64(blockCount)
	   return snap
   }

// validateGateReq 校验 GateReq 必填字段。
  func (s *Service) validateGateReq(req GateReq) error {
	  if strings.TrimSpace(req.BusinessKey) == "" {
		   return ErrBusinessKey
	   }
	  if !req.Kind.Valid() {
		   return ErrKind
	  }
	  if strings.TrimSpace(req.Asset) == "" {
		   return ErrAsset
	   }
	  return nil
  }
