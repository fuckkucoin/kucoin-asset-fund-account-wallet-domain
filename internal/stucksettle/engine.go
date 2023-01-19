// 功能：回收效果世代闸核心引擎。
// - AcquireLease：为每次 RecoverStuck 尝试申请互斥租约；同 transfer_id 仅一个 Acting 租约。
// - CommitRecover：提交回收结果、固化 EffectProof；证明缺失时拒绝标记 Recovered。
// - BlockDup：重复 Acting 请求转为 DupRecoverCase，资金动作立即中止。
// - AdvanceMutex 在 AcquireLease 前强制检查（Hold 态拒绝旁路）。
// 不变量：
// - 只有持有有效 Acting 租约且 EffectProof 通过校验，StuckCase 才能进入 Recovered。
// - DupRecoverCase 一旦产生，第二次租约立即进入 DupBlocked（终态），不发起资金动作。

// 作者：Bronson
// 日期：2021.11.22

package stucksettle

import (
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
)

const (
	defaultLeaseTTLSeconds = 120
	maxLeaseTTLSeconds     = 600
)

// Store 引擎存储接口（内存或 SQL 实现均可挂载）。
type Store interface {
	// 世代操作
	GetOrCreateGen(transferID string, now time.Time) (*StuckGeneration, error)
	BumpGen(transferID string, now time.Time) (*StuckGeneration, error)
	GetGen(transferID string) (*StuckGeneration, error)
	// 租约操作
	PutLease(l *RecoverLease) error
	GetLease(leaseID string) (*RecoverLease, error)
	ActiveLeaseByTransfer(transferID string) (*RecoverLease, error) // 返回 Acting 租约
	// 双结案件操作
	PutDupCase(c *DupRecoverCase) error
	GetDupCase(dupCaseID string) (*DupRecoverCase, error)
	DupCasesByTransfer(transferID string) ([]DupRecoverCase, error)
	// 证明操作（append-only）
	AppendProof(p *EffectProof) error
	GetProof(proofID string) (*EffectProof, error)
	ProofByLease(leaseID string) (*EffectProof,
		error)
}

// Engine 回收效果世代闸引擎。
type Engine struct {
	mu sync.Mutex
	store Store
	mutex *MutexStore
	clock func() time.Time
}

// NewEngine 构造引擎；store 和 mutex 均为必填。
  func NewEngine(store Store, mutex *MutexStore) *Engine {
	   return &Engine{
		   store: store,
		  mutex: mutex,
		  clock: func() time.Time { return time.Now().UTC() },
	   }
  }

// WithClock 注入时钟（用）。
  func (e *Engine) WithClock(fn func() time.Time) *Engine {
	  if fn != nil {
		   e.clock = fn
	  }
	   return e
   }

   func (e *Engine) now() time.Time { return e.clock() }

// AcquireLease 为一次 RecoverStuck 尝试申请互斥租约。
// 流程：
// 1. 检查 AdvanceMutex（Hold 态或 Advance 持锁时返回 ErrAdvanceLocked/ErrHoldBypass）。
// 2. GetOrCreate StuckGeneration，BumpGen 获取新世代号。
// 3. 检查当前是否有 Acting 租约：
// a. 无 Acting 租约 → 创建 Open 租约并立即转入 Acting。
// b. 有 Acting 租约 → 产生 DupRecoverCase，新租约进入 DupBlocked，返回 ErrDupBlocked。

  func (e *Engine) AcquireLease(req AcquireLeaseReq, isHoldState bool) (*RecoverLease, error) {
	   if e == nil || e.store == nil {
		  return nil, ErrUnavailable
	  }
	  trID := strings.TrimSpace(req.TransferID)
	  holderKey := strings.TrimSpace(req.HolderKey)
	   if trID == "" || holderKey == "" {
		  return nil, ErrInvalidInput
	   }
	   now := e.now()
	   // 检查 Advance 互斥
	   if err := e.mutex.CheckRecoverAllowed(trID, isHoldState, now); err != nil {
		   return nil, err
	   }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	  // 先看是否已有同 holderKey 的活跃租约（幂等）
	   if existing, err := e.store.ActiveLeaseByTransfer(trID); err == nil && existing != nil {
		  if existing.HolderKey == holderKey {
			  return existing, nil // 幂等返回
		   }
		  // 已有不同 holderKey 的 Acting 租约 → 双结
		   return e.createDupBlockedLease(existing,
			  trID, holderKey, now)
	  }
	   // BumpGen
	   gen, err := e.store.BumpGen(trID, now)
	   if err != nil {
		   return nil, err
	  }
	  ttl := req.TTLSeconds
	   if ttl <= 0 {
		  ttl = defaultLeaseTTLSeconds
	   }
	   if ttl > maxLeaseTTLSeconds {
		   ttl = maxLeaseTTLSeconds
	  }
	   lease := &RecoverLease{
		   LeaseID: uuid.NewString(),
		   TransferID: trID,
		   Generation: gen.CurrentGen,
		  State:      LeaseActing,
		   HolderKey: holderKey,
		   OpenedAt:  now,
		  ActingAt:  now,
		  ExpiresAt: now.Add(time.Duration(ttl) * time.Second),
		   CreatedAt: now,
		  UpdatedAt: now,
	   }
	  if err := e.store.PutLease(lease); err != nil {
		  return nil, err
	  }
	  return lease, nil
   }

// createDupBlockedLease 创建被双结检测拦截的租约，并产生 DupRecoverCase。
// 调用前必须持有 e.mu。
   func (e *Engine) createDupBlockedLease(first *RecoverLease, trID, holderKey string, now time.Time) (*RecoverLease, error) {
	  gen, err := e.store.GetGen(trID)
	   if err != nil {
		   return nil, err
	   }
	   dupLease := &RecoverLease{
		   LeaseID: uuid.NewString(),
		  TransferID: trID,
		  Generation: gen.CurrentGen,
		  State:      LeaseDupBlocked,
		  HolderKey: holderKey,
		  OpenedAt:  now,
		  BlockedAt: now,
		   ExpiresAt: now,
		   BlockReason: "dup_recover: active lease exists",
		  CreatedAt: now,
		  UpdatedAt: now,
	   }
	  if err := e.store.PutLease(dupLease); err != nil {
		  return nil, err
	  }
	  dupCase := &DupRecoverCase{
		   DupCaseID: uuid.NewString(),
		  TransferID: trID,
		   Generation:   gen.CurrentGen,
		  FirstLeaseID: first.LeaseID,
		   DupLeaseID:   dupLease.LeaseID,
		   State: DupCaseOpen,
		  DetectedAt: now,
		  CreatedAt:  now,
		  UpdatedAt:  now,
	  }
	  _ = e.store.PutDupCase(dupCase)
	   return dupLease, ErrDupBlocked
   }

// CommitRecover 提交回收结果，固化 EffectProof，更新租约为 Recovered。
// 约束：
// - 租约必须处于 Acting 状态（非终态）。
// - 必须构造并写入有效 EffectProof；proof 缺失或无效时返回 ErrProofMissing。
// - 证明写入后租约才能转为 Recovered。

  func (e *Engine) CommitRecover(req CommitRecoverReq) (*EffectProof, error) {
	   if e == nil || e.store == nil {
		  return nil, ErrUnavailable
	  }
	   leaseID := strings.TrimSpace(req.LeaseID)
	  if leaseID == "" {
		   return nil, ErrInvalidInput
	  }
	   e.mu.Lock()
	   defer e.mu.Unlock()
	  lease, err := e.store.GetLease(leaseID)
	   if err != nil || lease == nil {
		  return nil, ErrLeaseTerminal
	  }
	   if LeaseIsTerminal(lease.State) {
		   return nil, ErrLeaseTerminal
	  }
	  if lease.State != LeaseActing {
		  return nil, ErrLeaseTerminal
	  }
	  now := e.now()
	  if now.After(lease.ExpiresAt) {
		   lease.State = LeaseExpired
		  lease.ExpiredAt = now
		  lease.UpdatedAt = now
		  _ = e.store.PutLease(lease)
		  return nil, ErrLeaseExpired
	  }
	  proof, err := BuildEffectProof(lease, req.Action, req.LedgerRef, req.IssuedBy, now)
	  if err != nil {
		   return nil, ErrProofMissing
	  }
	   if !VerifyProof(proof) {
		   return nil, ErrProofInvalid
	   }
	  if err := e.store.AppendProof(proof); err != nil {
		  return nil, err
	   }
	   lease.State = LeaseRecovered
	  lease.RecoveredAt = now
	   lease.EffectProofID = proof.EffectProofID
	  lease.UpdatedAt = now
	   if err := e.store.PutLease(lease); err != nil {
		  return nil, err
	  }
	  return proof, nil
   }

// ExpireLease 将已超时的 Acting 租约强制设为 Expired。
// 由 worker 定期调用；不产生资金动作。

  func (e *Engine) ExpireLease(leaseID string) error {
	   if e == nil || e.store == nil {
		   return ErrUnavailable
	  }
	   e.mu.Lock()
	   defer e.mu.Unlock()
	  lease, err := e.store.GetLease(leaseID)
	  if err != nil || lease == nil {
		   return nil
	   }
	   if LeaseIsTerminal(lease.State) {
		  return nil
	  }
	  now := e.now()
	  if now.Before(lease.ExpiresAt) {
		   return nil // 未超时，不操作
	   }
	  lease.State = LeaseExpired
	   lease.ExpiredAt = now
	  lease.UpdatedAt = now
	   return e.store.PutLease(lease)
  }

// GetEffectProof 查询 transfer 最新世代的效果证明。
// 先查 Acting 租约（回收进行中），再查 Recovered 租约（已完成）。
// 返回 ErrProofMissing 时调用方不得将 StuckCase 标记为 Recovered。
  func (e *Engine) GetEffectProof(transferID string) (*EffectProof, error) {
	   if e == nil || e.store == nil {
		  return nil, ErrUnavailable
	  }
	   trID := strings.TrimSpace(transferID)
	  if trID == "" {
		   return nil, ErrInvalidInput
	   }
	  // 先找 Acting 租约
	  lease, _ := e.store.ActiveLeaseByTransfer(trID)
	   // 若无 Acting 租约，通过 ProofByTransfer 从证明表直接查最新世代证明
	   if lease == nil || lease.EffectProofID == "" {
		   // 尝试通过 MemStore/SQLStore 反查 Recovered 租约
		  proof, err := e.proofByTransfer(trID)
		  if err != nil || proof == nil {
			  return nil, ErrProofMissing
		  }
		   if !VerifyProof(proof) {
			   return nil, ErrProofInvalid
		   }
		   return proof, nil
	   }
	   proof, err := e.store.GetProof(lease.EffectProofID)
	  if err != nil || proof == nil {
		   return nil, ErrProofMissing
	  }
	   if !VerifyProof(proof) {
		  return nil, ErrProofInvalid
	   }
	   return proof, nil
   }

// proofByTransfer 通过 transfer_id 在证明表中查最新一条证明。
// 内存实现直接遍历；SQL 实现走 ProofByLease（需调用方先找到 Recovered 租约 ID）。

  func (e *Engine) proofByTransfer(transferID string) (*EffectProof, error) {
	  if ms, ok := e.store.(*MemStore); ok {
		   ms.mu.Lock()
		  defer ms.mu.Unlock()
		  var latest *EffectProof
		  for _, p := range ms.proofs {
			  if p.TransferID == transferID {
				  if latest == nil || p.IssuedAt.After(latest.IssuedAt) {
					  cp := *p
					   latest = &cp
				   }
			   }
		   }
		  if latest == nil {
			   return nil, ErrProofMissing
		  }
		   return latest, nil
	  }
	   // SQL 路径：通过 transfer_id 索引查询
	   if ss, ok := e.store.(*SQLStore); ok {
		   return ss.proofByTransferSQL(transferID)
	   }
	  return nil, ErrProofMissing
  }

// GetOrCreateGen 查询或初始化世代记录（查询路径，不自动 Bump）。

  func (e *Engine) GetOrCreateGen(transferID string) (*StuckGeneration, error) {
	  if e == nil || e.store == nil {
		  return nil, ErrUnavailable
	   }
	   return e.store.GetOrCreateGen(strings.TrimSpace(transferID), e.now())
   }

// DupCasesByTransfer 返回 transfer 的全部双结案件。
  func (e *Engine) DupCasesByTransfer(transferID string) ([]DupRecoverCase, error) {
	  if e == nil || e.store == nil {
		  return nil, ErrUnavailable
	   }
	  return e.store.DupCasesByTransfer(strings.TrimSpace(transferID))
   }

// ConfirmDupCase 双人确认关闭双结案件。

  func (e *Engine) ConfirmDupCase(req ConfirmDupCaseReq) (*DupRecoverCase, error) {
	   if e == nil || e.store == nil {
		   return nil, ErrUnavailable
	  }
	   if strings.TrimSpace(req.DupCaseID) == "" {
		   return nil, ErrInvalidInput
	   }
	  if strings.TrimSpace(req.ConfirmedBy) == "" || strings.TrimSpace(req.CoConfirmedBy) == "" {
		  return nil, ErrInvalidActor
	  }
	  if req.ConfirmedBy == req.CoConfirmedBy {
		  return nil, ErrInvalidActor // 双人必须不同
	   }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	   c, err := e.store.GetDupCase(req.DupCaseID)
	   if err != nil || c == nil {
		   return nil, ErrCaseNotFound
	   }
	  if DupCaseIsTerminal(c.State) {
		  return nil, ErrCaseAlreadyClosed
	  }
	  if err := DupCaseTransition(c.State, DupCaseClosed); err != nil {
		  return nil, err
	   }
	   now := e.now()
	   c.State = DupCaseClosed
	   c.ConfirmedBy = req.ConfirmedBy
	  c.CoConfirmedBy = req.CoConfirmedBy
	   c.ClosedAt = now
	   c.CloseNote = strings.TrimSpace(req.Note)
	   c.UpdatedAt = now
	  if err := e.store.PutDupCase(c); err != nil {
		   return nil, err
	   }
	   return c, nil
  }

// ExportProof 导出效果证明包（供审计）。
  func (e *Engine) ExportProof(req ProofExportReq) (*ProofExport, error) {
	  if e == nil || e.store == nil {
		  return nil, ErrUnavailable
	  }
	   now := e.now()
	   var proof *EffectProof
	  var lease *RecoverLease
	   var gen *StuckGeneration
	   var err error
	   if req.LeaseID != "" {
		   lease, err = e.store.GetLease(strings.TrimSpace(req.LeaseID))
		   if err != nil || lease == nil {
			  return nil, ErrLeaseTerminal
		  }
		   if lease.EffectProofID != "" {
			  proof, _ = e.store.GetProof(lease.EffectProofID)
		   }
	  } else if req.TransferID != "" {
		   proof, err = e.GetEffectProof(req.TransferID)
		   if err != nil {
			  return nil, err
		   }
		   lease,
			  _ = e.store.GetLease(proof.LeaseID)
	  }
	   if proof == nil {
		   return nil, ErrProofMissing
	  }
	   if lease != nil {
		  gen, _ = e.store.GetGen(lease.TransferID)
	  }
	   export := &ProofExport{ExportedAt: now}
	   export.Proof = CopyProof(*proof)
	   if lease != nil {
		  export.Lease = CopyLease(*lease)
	   }
	   if gen != nil {
		  export.Generation = CopyGen(*gen)
	  }
	  return export, nil
   }
