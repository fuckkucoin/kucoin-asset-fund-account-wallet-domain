// batchhopgate 顶层服务聚合——组合账户批量 hop 部分失败残留与续跳互斥收口引擎门面。
// Service 将四个子系统整合为统一对外接口：
// BatchResidualFSM — BatchResidualCase 状态机（open/proven/resuming/closed/blocked）
// ResumeLeaseFSM — ResumeLease 续跳互斥世代租约（open/bound/done/expired）
// OccupyProofVault — HopOccupyProof 部分守恒占用证明
// BatchGenEngine — BatchGeneration CAS 世代计数器
// 核心调用流（BatchHop 失败路径，由 portfolio.BatchHop 在 item i 失败时调用）：
// 1. OpenResidual(req) → BatchGenEngine.AcquireGeneration（CAS 世代+1）
// 2. OpenResidual → BatchResidualFSM.OpenResidual（落库残留案）
// 3. OpenResidual → OccupyProofVault.Build（构建部分守恒证明）
// 4. OpenResidual → BatchResidualFSM.Prove（open→proven）
// 5. OpenResidual → 返回 BatchResidualCase + GenToken 给调用方
// 核心调用流（续跳路径，须携带 GenToken）：
// 1. RequestResume(req) → ValidateToken → 检查残留案状态（须为 proven）
// 2. RequestResume → ResumeLeaseFSM.Acquire（申请互斥续跳租约）
// 3. RequestResume → BatchResidualFSM.BindLease（proven→resuming）
// 4. ExecuteResume(req) → ResumeLeaseFSM.Bind（open→bound）
// 5. ExecuteResume → 逐项执行续跳 hop（跳过已成功索引，防二次扣入）
// 6. ExecuteResume → ResumeLeaseFSM.Done（bound→done）
// 7. ExecuteResume → BatchResidualFSM.Close（resuming→closed）
// 8. ExecuteResume → BatchGenEngine.ReleaseGeneration（active→idle）
// 并行互斥保证：
// 同用户同资产存在 open/proven/resuming 残留时，OpenResidual 返回 ErrConcurrentBatch，
// 拒绝 portfolio.BatchHop 开启新批次。
// 与其他包的隔离：
// 本包不读写 acctpol.PolicyVersion；不写 xferloop；不写 wdbatch；
// 不改写 ledger Apply/fence 内核；portfolio BookLedger 只读消费。
// 产出标识（供其他包只读）：
// batch_residual_id — BatchResidualCase.ID
// resume_lease_id — ResumeLease.ID

// Author: ColeHale
// Date: 2021.02.24

package batchhopgate

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Service 是 batchhopgate 引擎的顶层门面。

type Service struct {
	residualFSM *BatchResidualFSM
	leaseFSM   *ResumeLeaseFSM
	proofVault *OccupyProofVault
	genEngine *BatchGenEngine
	metrics   *GateMetrics
	store     Store
	clock     func() time.Time
}

// NewService 构造 Service。store 为 nil 时使用内存存储（仅用）。

   func NewService(store Store) *Service {
	  if store == nil {
		  store = NewMemStore()
	   }
	  clk := func() time.Time { return time.Now().UTC() }
	   return &Service{
		  residualFSM: NewResidualFSM(store),
		  leaseFSM:   NewResumeLeaseFSM(store),
		  proofVault: NewOccupyProofVault(store),
		   genEngine: NewBatchGenEngine(store),
		  metrics: newGateMetrics(),
		   store:   store,
		   clock:   clk,
	   }
  }

   func (svc *Service) WithClock(c func() time.Time) *Service {
	  if svc != nil && c != nil {
		   svc.clock = c
		   svc.residualFSM.WithClock(c)
		  svc.leaseFSM.WithClock(c)
		   svc.proofVault.WithClock(c)
		   svc.genEngine.clock = c
	   }
	   return svc
  }

   // ─────────────────────────────────────────────────────────────────────────────
  // OpenResidual — 落库残留案（由 portfolio.BatchHop 在 item i 失败时调用）
  // ─────────────────────────────────────────────────────────────────────────────
  // OpenResidualReq 完整参数，由 portfolio 层组装后传入。
   type OpenResidualFullReq struct {
	   UserID string
	  Asset      string
	  TotalItems int
	   FailIndex     int
	  SucceededHops []HopSummary
	   FailedReason  string
	  BeforeRoot  string
	   PartialRoot string
	  BuiltBy     string
  }

  // OpenResidualResult 包含落库残留案、占用证明和世代令牌。

  type OpenResidualResult struct {
	  Residual *BatchResidualCase
	   Proof *HopOccupyProof
	   Token GenToken
  }

   // OpenResidual 是 BatchHop 部分失败时的核心落库入口。
  // 执行顺序：
  // 1. 检查并发互斥（FindOpenResidual）
  // 2. CAS 递增世代（BumpGeneration）
   // 3. 落库残留案（BatchResidualFSM.OpenResidual）
  // 4. 构建占用证明（OccupyProofVault.Build）
  // 5. 推进残留案至 proven（BatchResidualFSM.Prove）
  // 6. 颁发世代令牌

   func (svc *Service) OpenResidual(ctx context.Context, req OpenResidualFullReq) (*OpenResidualResult, error) {
	   if svc == nil {
		   return nil, ErrUnavailable
	   }
	   if strings.TrimSpace(req.UserID) == "" {
		   return nil, ErrUserInvalid
	   }
	   if strings.TrimSpace(req.Asset) == "" {
		   return nil, ErrAssetInvalid
	   }
	   // 并发互斥检测。
	  if err := svc.residualFSM.CheckConcurrent(ctx, req.UserID, req.Asset); err != nil {
		   svc.metrics.residualBlocked.Inc()
		   return nil, err
	   }
	  // 读取当前世代（用于 CAS）。
	   genRec, err := svc.store.GetGeneration(ctx, req.UserID, req.Asset)
	  if err != nil {
		  return nil, err
	   }
	   newGen, err := svc.store.BumpGeneration(ctx, req.UserID, req.Asset, genRec.Generation)
	   if err != nil {
		   svc.metrics.genConflict.Inc()
		   return nil, err
	   }
	   // 落库残留案。
	  residual, err := svc.residualFSM.OpenResidual(ctx, OpenResidualReq{
		   UserID:     req.UserID,
		   Asset:      req.Asset,
		  Generation: newGen,
		   TotalItems: req.TotalItems,
		  FailIndex:     req.FailIndex,
		  SucceededHops: req.SucceededHops,
		   FailedReason:  req.FailedReason,
		  BeforeRoot:  req.BeforeRoot,
		   PartialRoot: req.PartialRoot,
	  })
	  if err != nil {
		   // 落库失败须释放世代，防止死锁。
		   _ = svc.store.ReleaseGeneration(ctx, req.UserID, req.Asset, newGen)
		   return nil, err
	   }
	   svc.metrics.residualOpened.Inc()
	   // 构建占用证明。
	  totals := BuildSubjectTotals(req.SucceededHops)
	  conserveOK := ConserveCheck(req.SucceededHops)
	  proof, err := svc.proofVault.Build(ctx, BuildReq{
		   ResidualCaseID: residual.ID,
		   UserID: req.UserID,
		   Asset:  req.Asset,
		   BeforeRoot: req.BeforeRoot,
		  PartialRoot: req.PartialRoot,
		  SucceededHops: req.SucceededHops,
		   SubjectTotals: totals,
		  ConserveOK:    conserveOK,
		   BuiltBy:       req.BuiltBy,
	  })
	   if err != nil {
		  // 证明构建失败；残留案保持 open 状态，等待 worker 重建。
		   svc.metrics.proofBuildFailed.Inc()
		   token := svc.leaseFSM.IssueToken(residual)
		  return &OpenResidualResult{Residual: residual, Token: token}, nil
	   }
	  svc.metrics.proofBuilt.Inc()
	   // 推进残留案至 proven。
	  residual, err = svc.residualFSM.Prove(ctx, residual.ID, proof.ID)
	   if err != nil {
		   return &OpenResidualResult{Residual: residual, Proof: proof}, nil
	   }
	  svc.metrics.residualProven.Inc()
	  token := svc.leaseFSM.IssueToken(residual)
	  return &OpenResidualResult{Residual: residual, Proof: proof, Token: token}, nil
  }

// ─────────────────────────────────────────────────────────────────────────────
// RequestResume — 申请续跳互斥租约
// ─────────────────────────────────────────────────────────────────────────────
// RequestResumeReq 申请续跳租约的参数。
type RequestResumeReq struct {
	ResidualCaseID string
	Token          GenToken
	ResumeFrom     int
	ConfirmedBy    string
	Note string
}

// RequestResume 验证世代令牌后申请续跳互斥租约，推进残留案至 resuming。
  func (svc *Service) RequestResume(ctx context.Context, req RequestResumeReq) (*ResumeLease, error) {
	  if svc == nil {
		   return nil, ErrUnavailable
	   }
	   residual, err := svc.store.GetResidual(ctx, req.ResidualCaseID)
	  if err != nil {
		   return nil, ErrResidualNotFound
	  }
	  // 验证世代令牌。
	  if err := svc.leaseFSM.ValidateToken(req.Token, residual); err != nil {
		  svc.metrics.tokenRejected.Inc()
		   return nil, err
	   }
	   if residual.Status != ResidualProven {
		   // 已进入 resuming 且租约未过期：二次申请走互斥冲突，禁止并行续跳。
		   existing, findErr := svc.store.FindActiveLease(ctx, req.ResidualCaseID)
		  if findErr == nil && existing != nil && !svc.clock().After(existing.ExpiresAt) {
			   svc.metrics.leaseConflict.Inc()
			  return nil, ErrLeaseConflict
		  }
		  return nil, fmt.Errorf("%w: current=%s, need proven", ErrResidualStatusInvalid, residual.Status)
	  }
	  if req.ResumeFrom < residual.FailIndex {
		  return nil, ErrResumeIndexInvalid
	   }
	  // 申请续跳互斥租约。
	  lease, err := svc.leaseFSM.Acquire(ctx, AcquireReq{
		  ResidualCaseID: req.ResidualCaseID,
		   UserID:         residual.UserID,
		  Asset: residual.Asset,
		  Generation: residual.Generation,
		   ResumeFrom: req.ResumeFrom,
		   ConfirmedBy: req.ConfirmedBy,
		   Note:        req.Note,
	   })
	   if err != nil {
		  svc.metrics.leaseConflict.Inc()
		  return nil, err
	  }
	   svc.metrics.leaseOpened.Inc()
	  // 推进残留案至 resuming。
	  _, err = svc.residualFSM.BindLease(ctx, req.ResidualCaseID, lease.ID)
	  if err != nil {
		   return lease, err
	  }
	   return lease, nil
  }

// ─────────────────────────────────────────────────────────────────────────────
// ExecuteResume — 执行续跳（幂等）
// ─────────────────────────────────────────────────────────────────────────────
// ExecuteResume 在持有有效 ResumeLease 的前提下，逐项执行续跳 hop。
// 幂等语义：
// - 对 ResumeItems 中 Index < FailIndex 的项：直接跳过（已成功，不得二次执行）。
// - 对 Index == BoundIdem 且参数一致：跳过（幂等重复）。
// - BoundIdem 冲突（相同 idem 但 Index 不同）：返回 ErrResumeConflict。
// HopExecutor 为调用方注入的单笔 hop 执行函数，签名需与 portfolio.Service.HopIdem 兼容。
// 若为 nil，ExecuteResume 仅验证权限和状态，不实际执行（供和预检使用）。

   func (svc *Service) ExecuteResume(ctx context.Context, req ResumeRequest, hopExec HopExecutor) (*ResumeResult, error) {
	  if svc == nil {
		  return nil, ErrUnavailable
	  }
	   residual, err := svc.store.GetResidual(ctx, req.ResidualCaseID)
	   if err != nil {
		   return nil, ErrResidualNotFound
	   }
	  if residual.Status != ResidualResuming {
		   return nil, fmt.Errorf("%w: current=%s, need resuming", ErrResidualStatusInvalid, residual.Status)
	   }
	  lease, err := svc.store.GetLease(ctx, req.LeaseID)
	   if err != nil {
		   return nil, ErrLeaseNotFound
	   }
	  if lease.ResidualCaseID != req.ResidualCaseID {
		  return nil, ErrLeaseConflict
	  }
	  now := svc.clock()
	  if lease.Status == LeaseExpired || (lease.Status == LeaseOpen && now.After(lease.ExpiresAt)) {
		  svc.metrics.leaseExpired.Inc()
		  return nil, ErrLeaseExpired
	   }
	   // 绑定续跳 idem 键（幂等）。
	  boundIdem := req.LeaseID + "|resume"
	  _, err = svc.leaseFSM.Bind(ctx, lease.ID, boundIdem)
	  if err != nil {
		   return nil, err
	  }
	   // 构建已成功 hop 索引集合，防止二次执行。
	  succeededIdx := map[int]bool{}
	   for _, h := range residual.SucceededHops {
		  succeededIdx[h.Index] = true
	  }
	   result := &ResumeResult{
		   ResidualCaseID: req.ResidualCaseID,
		   LeaseID: req.LeaseID,
	  }
	   for _, item := range req.ResumeItems {
		  if succeededIdx[item.Index] {
			   // 已成功，跳过（防二次扣入）。
			   result.Skipped = append(result.Skipped, HopSummary{
				   Index: item.Index,
				   From: item.From, To: item.To,
				  Asset: item.Asset, Amount: item.Amount, Idem: item.Idem,
			  })
			   continue
		   }
		   if item.Index < residual.FailIndex {
			  return nil, fmt.Errorf("%w: item index %d < fail_index %d",
				   ErrResumeIndexInvalid,
				  item.Index, residual.FailIndex)
		  }
		  if hopExec != nil {
			  if err := hopExec(ctx, item); err != nil {
				   return result, fmt.Errorf("batchhopgate: resume hop failed at item %d: %w", item.Index, err)
			  }
		   }
		   result.Executed = append(result.Executed, HopSummary{
			  Index: item.Index, From: item.From, To: item.To,
			  Asset: item.Asset, Amount: item.Amount, Idem: item.Idem,
		  })
		  svc.metrics.resumeHopDone.Inc()
	  }
	   // 续跳完成：推进租约和残留案至终态。
	   if _, err := svc.leaseFSM.Done(ctx, lease.ID); err != nil {
		   return result, err
	   }
	   svc.metrics.leaseDone.Inc()
	  if _, err := svc.residualFSM.Close(ctx, req.ResidualCaseID, req.ConfirmedBy, req.Note); err != nil {
		  return result, err
	   }
	   svc.metrics.residualClosed.Inc()
	   // 释放 BatchGeneration 世代锁。
	   _ = svc.store.ReleaseGeneration(ctx,
		   residual.UserID, residual.Asset, residual.Generation)
	   result.NewStatus = ResidualClosed
	  return result, nil
   }

// HopExecutor 单笔续跳 hop 执行函数（由 portfolio 层注入）。
type HopExecutor func(ctx context.Context, item ResumeItem) error

// ─────────────────────────────────────────────────────────────────────────────
// GetResidual / ListResiduals — 残留案查询
// ─────────────────────────────────────────────────────────────────────────────
// GetResidual 按 case_id 查询残留案（只读）。
   func (svc *Service) GetResidual(ctx context.Context, caseID string) (*BatchResidualCase, error) {
	   if svc == nil {
		   return nil, ErrUnavailable
	  }
	   return svc.store.GetResidual(ctx, caseID)
  }

// ListResiduals 返回用户所有残留案（只读）。

  func (svc *Service) ListResiduals(ctx context.Context, userID string, limit int) ([]*BatchResidualCase, error) {
	   if svc == nil {
		   return nil, ErrUnavailable
	  }
	   return svc.store.ListResidualsByUser(ctx, userID, limit)
  }

// GetProof 按 proof_id 查询占用证明（只读）。
  func (svc *Service) GetProof(ctx context.Context, proofID string) (*HopOccupyProof, error) {
	  if svc == nil {
		   return nil, ErrUnavailable
	   }
	   return svc.store.GetProof(ctx,
		  proofID)
  }

// ExportProof 导出占用证明 JSON（供审计接口）。
  func (svc *Service) ExportProof(ctx context.Context, proofID string) ([]byte, error) {
	  if svc == nil {
		  return nil, ErrUnavailable
	   }
	   return svc.proofVault.Export(ctx,
		  proofID)
   }

// VerifyProof 验证占用证明 Digest 完整性。
  func (svc *Service) VerifyProof(ctx context.Context, proofID string) (*HopOccupyProof, error) {
	  if svc == nil {
		   return nil, ErrUnavailable
	   }
	  return svc.proofVault.Verify(ctx, proofID)
  }

// ConfirmClose 四眼确认收口残留案。

   func (svc *Service) ConfirmClose(ctx context.Context, caseID, confirmerID, note string) (*BatchResidualCase, error) {
	  if svc == nil {
		   return nil, ErrUnavailable
	  }
	  c, err := svc.residualFSM.ConfirmClose(ctx, caseID, confirmerID, note)
	   if err != nil {
		  return nil, err
	   }
	  if c.Status == ResidualClosed {
		  svc.metrics.residualClosed.Inc()
		  if c.UserID != "" && c.Asset != "" {
			  _ = svc.store.ReleaseGeneration(ctx, c.UserID, c.Asset, c.Generation)
		  }
	   }
	   return c, nil
  }

// IssueToken 为已有残留案颁发世代令牌（供调用方在续跳时携带）。

  func (svc *Service) IssueToken(ctx context.Context, caseID string) (GenToken, error) {
	  if svc == nil {
		  return GenToken{},
			  ErrUnavailable
	   }
	   residual, err := svc.store.GetResidual(ctx, caseID)
	  if err != nil {
		  return GenToken{}, ErrResidualNotFound
	   }
	   return svc.leaseFSM.IssueToken(residual),
		   nil
   }

// GetGeneration 查询用户资产当前世代状态（只读）。
  func (svc *Service) GetGeneration(ctx context.Context, userID, asset string) (*BatchGeneration, error) {
	  if svc == nil {
		   return nil, ErrUnavailable
	  }
	   return svc.store.GetGeneration(ctx,
		  userID, asset)
   }

// BlockResidual 管理员强制阻塞残留案（审核拒绝）。

  func (svc *Service) BlockResidual(ctx context.Context, caseID, reason string) (*BatchResidualCase, error) {
	   if svc == nil {
		  return nil, ErrUnavailable
	   }
	   c, err := svc.residualFSM.Block(ctx, caseID, reason)
	   if err != nil {
		  return nil, err
	   }
	  if c.UserID != "" && c.Asset != "" {
		   _ = svc.store.ReleaseGeneration(ctx, c.UserID, c.Asset, c.Generation)
	  }
	   return c, nil
  }

// CheckConcurrent 检查是否存在阻塞并行 BatchHop 的未闭合残留案（供 portfolio.BatchHop 预检）。
  func (svc *Service) CheckConcurrent(ctx context.Context, userID, asset string) error {
	  if svc == nil {
		  return ErrUnavailable
	   }
	   return svc.residualFSM.CheckConcurrent(ctx, userID, asset)
  }
