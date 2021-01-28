package swapthaw

import (
	"fmt"
	"time"
)

// Service 闪兑冻额解冻互斥引擎服务。

type Service struct {
	gen   *GenerationManager
	mutex *ThawMutex
	detector *DoubleReleaseDetector
	leaseTTL time.Duration
	clock func() time.Time
}

// NewService 构造服务（内存持久化，适用于与单机模式）。
// leaseTTL 为 0 时 FreezeLease 不过期。
  func NewService(leaseTTL time.Duration) *Service {
	  genStore := NewMemGenerationStore()
	  leaseStore := NewMemLeaseStore()
	  bindStore := NewMemBindStore()
	   caseStore := NewMemThawCaseStore()
	  return &Service{
		  gen: NewGenerationManager(genStore),
		   mutex:    NewThawMutex(leaseStore, bindStore),
		   detector: NewDoubleReleaseDetector(caseStore),
		   leaseTTL: leaseTTL,
		   clock:    func() time.Time { return time.Now().UTC() },
	   }
  }

// NewServiceWith 用外部组件构造服务（模式，注入 SQL 实现）。
   func NewServiceWith(
	   genStore GenerationStore,
	   leaseStore LeaseStore,
	   bindStore BindStore,
	   caseStore ThawCaseStore,
	  leaseTTL time.Duration,
   ) *Service {
	   return &Service{
		   gen:   NewGenerationManager(genStore),
		  mutex: NewThawMutex(leaseStore, bindStore),
		   detector: NewDoubleReleaseDetector(caseStore),
		   leaseTTL: leaseTTL,
		  clock:    func() time.Time { return time.Now().UTC() },
	   }
  }

   func (svc *Service) now() time.Time {
	  if svc.clock != nil {
		   return svc.clock()
	  }
	   return time.Now().UTC()
  }

// ──────────────────────────────────────────────
// 冻额路径（Execute 调用）
// ──────────────────────────────────────────────
// GuardNewFreeze 检查用户是否允许发起新冻额。
// 存在未关闭 ThawCase 时返回 ErrThawCaseOpen，调用方须拒绝 Execute 请求。
// 本方法在 Execute 进入 InTx 前调用，不要求在事务内。

  func (svc *Service) GuardNewFreeze(userID string) error {
	  if userID == "" {
		  return fmt.Errorf("swapthaw: GuardNewFreeze: empty userID")
	  }
	   blocked, err := svc.gen.UserHasOpenThawCase(userID)
	   if err != nil {
		  return fmt.Errorf("swapthaw: GuardNewFreeze: %w", err)
	   }
	  if blocked {
		  return ErrThawCaseOpen
	  }
	  return nil
   }

// AcquireLeaseForOrder 在 freeze journal 同事务内调用：
// 1. 读取订单当前世代。
// 2. CAS 抬升世代（与 freeze journal 原子）。
// 3. 创建 FreezeLease，绑定世代与令牌。
// 返回的 FreezeLease.ID（freeze_lease_id）须随 swap 订单一起存档，供 failUnfreeze/settle 携带。

   func (svc *Service) AcquireLeaseForOrder(orderID, userID, asset, frozenAmount string) (*FreezeLease, error) {
	  gen, err := svc.gen.Current(orderID)
	  if err != nil {
		  return nil, fmt.Errorf("swapthaw: AcquireLeaseForOrder.CurrentGen: %w", err)
	  }
	   // 先创建 Lease（获得 leaseID），再 CAS 绑定世代。
	  req := AcquireLeaseReq{
		   OrderID:      orderID,
		   UserID:       userID,
		  Asset:        asset,
		   FrozenAmount: frozenAmount,
		  TTL: svc.leaseTTL,
	  }
	  lease, err := svc.mutex.AcquireLease(req, gen.Generation+1)
	   if err != nil {
		  return nil, err
	  }
	   // CAS 抬升世代，绑定新 Lease。
	   _, err = svc.gen.BumpGeneration(orderID, userID, lease.ID, gen.Generation)
	   if err != nil {
		   return nil, fmt.Errorf("swapthaw: AcquireLeaseForOrder.BumpGeneration: %w", err)
	   }
	  return lease, nil
   }

// ──────────────────────────────────────────────
// 终态声明路径（settle / failUnfreeze 调用）
// ──────────────────────────────────────────────
// ClaimSettleTerminal 声明 settle 终态腿。
// 流程：
// 1. 调用 ThawMutex.ClaimTerminal（互斥争抢）。
// 成功：返回 SettleBind，调用方接着账本操作。
// ErrThawMutexDenied：另一腿（failUnfreeze）已胜出，返回 ErrThawMutexDenied。
// ErrDoubleRelease：Lease 已终态（settle 后再次 settle 或 failUnfreeze 已 settle 后再 settle）：
// 升 ThawCase，返回 ErrDoubleRelease。
// 2. 成功后清除订单活跃 Lease（世代 active_lease_id 置空）。
  func (svc *Service) ClaimSettleTerminal(req ClaimTerminalReq) (*SettleBind, *ThawCase, error) {
	   req.Leg = BindLegSettle
	  return svc.claimTerminalInternal(req)
  }

// ClaimUnfreezeTerminal 声明 failUnfreeze 终态腿。
// 与 ClaimSettleTerminal 逻辑对称：Leg=BindLegFailUnfreeze。
// 返回 ErrThawMutexDenied 表示 settle 已胜出（幂等拒绝）；
// 返回 ErrDoubleRelease 表示 settle 已完成但 failUnfreeze 再次尝试（升 ThawCase）。
   func (svc *Service) ClaimUnfreezeTerminal(req ClaimTerminalReq) (*SettleBind, *ThawCase, error) {
	  req.Leg = BindLegFailUnfreeze
	   return svc.claimTerminalInternal(req)
  }

   func (svc *Service) claimTerminalInternal(req ClaimTerminalReq) (*SettleBind, *ThawCase, error) {
	   bind, lease, err := svc.mutex.ClaimTerminal(req)
	  if err == nil {
		  // 胜出，清除活跃 Lease。
		   _ = svc.gen.ClearActiveLease(req.OrderID)
		  return bind, nil, nil
	   }
	   if err == ErrDoubleRelease {
		  // 双释事故：升 ThawCase。
		  assetStr := ""
		   amtStr := ""
		  if lease != nil {
			  assetStr = lease.Asset
			  amtStr = lease.FrozenAmount
			  // Lease 进入 blocked 终态。
			  now := svc.now()
			   _ = TransitLeaseWithLeg(lease, LeaseStatusBlocked, req.Leg, now)
			  _ = svc.mutex.leases.UpdateLease(lease)
		   }
		   ev := DoubleReleaseEvent{
			  OrderID: req.OrderID,
			  UserID: extractUserID(lease),
			  LeaseID:     req.LeaseID,
			   Asset:       assetStr,
			   Kind:        ThawCaseKindDoubleRelease,
			  IncomingLeg: req.Leg,
			  ExistingLeg: extractExistingLeg(lease),
			   DetectedAmount: amtStr,
			  FrozenSnapshot:    req.FrozenSnapshot,
			  AvailableSnapshot: req.AvailableSnapshot,
		  }
		  tc, detectErr := svc.detector.Detect(ev)
		   if detectErr != nil {
			   return nil, nil, fmt.Errorf("swapthaw: claimTerminalInternal.Detect: %w; original: %w", detectErr, err)
		   }
		   // 注册到 GenerationManager 以阻断用户新冻额。
		   if ms, ok := svc.gen.store.(*MemGenerationStore); ok {
			   ms.RegisterOpenCase(ev.UserID, tc.ID)
		  }
		  return nil, tc, ErrDoubleRelease
	   }
	  return nil, nil, err
   }

// ──────────────────────────────────────────────
// ThawCase 证明与收口
// ──────────────────────────────────────────────
// ProveThawCase 固化 ThawCase 证明（open→proven）。
// ProofPayload 须包含可核对的事故摘要（金额、快照哈希等）。
   func (svc *Service) ProveThawCase(req ProveThawCaseReq) (*ThawCase, error) {
	   tc, err := svc.detector.ProveCase(req)
	  if err != nil {
		   return nil, err
	  }
	  return tc, nil
   }

// CloseThawCase 收口 ThawCase（proven→closed），并自动解除用户阻断。
// 须确认案件已处于 proven 状态（ProveThawCase 已成功）。
  func (svc *Service) CloseThawCase(req CloseThawCaseReq) (*ThawCase, error) {
	   tc, err := svc.detector.CloseCase(req)
	   if err != nil {
		  return nil, err
	  }
	   // 解除 MemGenerationStore 中的用户阻断（仅内存模式有效，模式通过 SQL 查询）。
	   if ms, ok := svc.gen.store.(*MemGenerationStore); ok {
		   ms.RemoveOpenCase(tc.UserID,
			   tc.ID)
	   }
	  return tc, nil
   }

// ──────────────────────────────────────────────
// 查询 API
// ──────────────────────────────────────────────
// QueryGeneration 查询订单冻额世代。

  func (svc *Service) QueryGeneration(orderID string) (*GenerationQueryResp, error) {
	  gen, err := svc.gen.Current(orderID)
	   if err != nil {
		  return nil, err
	  }
	  blocked, err := svc.gen.UserHasOpenThawCase(gen.UserID)
	   if err != nil {
		  return nil, err
	  }
	  return &GenerationQueryResp{
		  OrderID:         orderID,
		   Generation:      gen.Generation,
		  ActiveLeaseID:   gen.ActiveLeaseID,
		  HasOpenThawCase: blocked,
		  UpdatedAt:       gen.UpdatedAt,
	   }, nil
  }

// GetLease 按 ID 查询 FreezeLease（供他包只读引用）。
  func (svc *Service) GetLease(leaseID string) (*FreezeLease, error) {
	   return svc.mutex.leases.GetLease(leaseID)
  }

// GetLeaseByOrder 按订单 ID 查询活跃 FreezeLease（供他包只读引用）。
   func (svc *Service) GetLeaseByOrder(orderID string) (*FreezeLease, error) {
	  return svc.mutex.leases.GetLeaseByOrder(orderID)
   }

// GetThawCase 按 ID 查询 ThawCase（thaw_proof_id，供他包只读引用）。
   func (svc *Service) GetThawCase(caseID string) (*ThawCase, error) {
	   return svc.detector.GetCase(caseID)
  }

// GetThawCaseByOrder 按订单 ID 查询最新 ThawCase。
  func (svc *Service) GetThawCaseByOrder(orderID string) (*ThawCase, error) {
	  return svc.detector.GetCaseByOrder(orderID)
   }

// ListOpenThawCases 列出全部未关闭 ThawCase（Worker 巡检入口）。
   func (svc *Service) ListOpenThawCases(limit int) ([]*ThawCase, error) {
	  return svc.detector.ListOpenCases(limit)
   }

// ListOpenThawCasesByUser 列出指定用户未关闭 ThawCase。

  func (svc *Service) ListOpenThawCasesByUser(userID string, limit int) ([]*ThawCase, error) {
	   return svc.detector.ListOpenByUser(userID, limit)
   }

// ──────────────────────────────────────────────
// 内部辅助
// ──────────────────────────────────────────────

   func extractUserID(lease *FreezeLease) string {
	   if lease == nil {
		   return ""
	  }
	  return lease.UserID
   }

   func extractExistingLeg(lease *FreezeLease) BindLeg {
	  if lease == nil {
		  return ""
	  }
	   return lease.WinnerLeg
   }
