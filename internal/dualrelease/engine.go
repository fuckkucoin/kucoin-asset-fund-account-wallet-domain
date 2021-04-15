package dualrelease

import (
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/persist"
)

// TicketFacts 只读票证事实；由调用方从 dualctl.Book 读取后注入，本包不写 dualctl 状态机。

type TicketFacts struct {
	// TicketID dualctl 请求 ID（即双控票证主键）。
	TicketID string
	// RequestID 关联业务请求（出金/划转等），可与 TicketID 相同。
	RequestID string
	// EpochAt 当前有效 epoch；Execute 时需与 ReleaseLease.Gen 一致。
	EpochAt int64
	// TTL 票证有效期（绝对时刻）。
	TTL time.Time
	// OpenedBy 开单人。
	OpenedBy string
}

// FundObservation 资金动作只读观察；由调用方从资金域注入，本包不写资金 FSM。

type FundObservation struct {
	// FundActionID 关联资金动作 ID（如出金单、划转单）。
	FundActionID string
	// Amount 数量（仅用于证明签名的摘要字段，不做业务计算）。
	Amount string
	// ObservedAt 资金动作观察时刻。
	ObservedAt time.Time
}

// Engine 双控放行核心引擎。
type Engine struct {
	store Store
	clock func() time.Time
	// leaseTTL 放行互斥租约有效期；超时后 reclaim worker 可回收。
	leaseTTL time.Duration
}

// NewEngine 初始化引擎。leaseTTL<=0 时使用默认值 5 分钟。
  func NewEngine(store Store, leaseTTL time.Duration) *Engine {
	   if leaseTTL <= 0 {
		   leaseTTL = 5 * time.Minute
	   }
	  return &Engine{store: store,
		   clock: func() time.Time { return time.Now().UTC() }, leaseTTL: leaseTTL}
  }

// WithClock 注入时钟（单调时钟替换）。
   func (e *Engine) WithClock(c func() time.Time) *Engine {
	   if e != nil && c != nil {
		   e.clock = c
	   }
	  return e
   }

  // ──────────────────────────────────────────────────
   // OpenCase 创建放行案例（ready 初态）
   // ──────────────────────────────────────────────────
   // OpenCase 为已达票的 dualctl 请求开一条放行案例（ReleaseCase）。
   // 调用方必须先确认 dualctl.Book.Gap/Execute 已通过。
   // 幂等：同 requestID 已存在且非终结态则直接返回现有案例。

   func (e *Engine) OpenCase(facts TicketFacts, actor string) (*ReleaseCase, error) {
	  if err := actorOK(actor); err != nil {
		   return nil, err
	  }
	   if facts.TicketID == "" || facts.RequestID == "" {
		  return nil, fmt.Errorf("dualrelease: ticket facts incomplete")
	  }
	  // 幂等：已存在非终结案例直接返回。
	  if existing, err := e.store.GetCase(facts.RequestID); err == nil {
		   if !existing.State.Closed() {
			   return existing, nil
		  }
		  return nil, ErrCaseClosed
	  }
	   now := e.clock()
	   rc := &ReleaseCase{
		   ID:        uuid.NewString(),
		  RequestID: facts.RequestID,
		   TicketID:  facts.TicketID,
		   Gen:       0,
		   State:     StateReady,
		  Actor:     actor,
		   CreatedAt: now,
		  UpdatedAt: now,
	  }
	  rc.Fence = persist.Fence{Epoch: 1,
		   Token: persist.NewToken()}
	  if err := e.store.PutCase(rc); err != nil {
		   return nil, err
	  }
	   return rc, nil
   }

// ──────────────────────────────────────────────────
// AcquireLease 抢占放行世代互斥租约（ready→unlocking）
// ──────────────────────────────────────────────────
// AcquireLease 尝试抢占 requestID 的互斥租约，并将世代（gen）递增。
// 并发两个 worker 调用时，只有一个能 CAS 成功，另一个返回 ErrCaseLocked。
// 成功后将 ReleaseCase 状态推进到 unlocking。

  func (e *Engine) AcquireLease(requestID, holder string) (*ReleaseLease, error) {
	   if err := actorOK(holder); err != nil {
		   return nil, err
	   }
	  rc, err := e.store.GetCase(requestID)
	   if err != nil {
		   return nil, err
	  }
	   if rc.State.Closed() {
		  return nil, ErrCaseClosed
	  }
	  if rc.State != StateReady {
		  return nil, ErrBadState
	   }
	   now := e.clock()
	   // 检查是否已存在有效租约（未过期且 holder 不同）。
	   if existing, lerr := e.store.GetLease(requestID); lerr == nil {
		   if now.Before(existing.ExpiresAt) && existing.Holder != holder {
			   return nil, ErrCaseLocked
		  }
	  }
	   newGen := rc.Gen + 1
	   l := &ReleaseLease{
		   ID: uuid.NewString(),
		  RequestID: requestID,
		   Holder:     holder,
		   Gen:        newGen,
		  State:      StateUnlocking,
		  LeaseToken: persist.NewToken(),
		  AcquiredAt: now,
		  ExpiresAt: now.Add(e.leaseTTL),
		  Fence:     persist.Fence{Epoch: 1, Token: persist.NewToken()},
		   CreatedAt: now,
		  UpdatedAt: now,
	   }
	   if err := e.store.PutLease(l); err != nil {
		   return nil, err
	  }
	   // 推进案例状态到 unlocking，并绑定世代。
	   rc.Gen = newGen
	  rc.State = StateUnlocking
	  rc.UpdatedAt = now
	   rc.Fence.Epoch++
	   rc.Fence.Token = persist.NewToken()
	  if err := e.store.PutCase(rc); err != nil {
		   return nil, err
	  }
	  return l, nil
   }

// ──────────────────────────────────────────────────
// ConfirmUnlock Unlock 成功回调（unlocking→executing）
// ──────────────────────────────────────────────────
// ConfirmUnlock 在 kms.Unlocker.Unlock 成功后调用，将案例推进到 executing。
// gen 必须与当前租约世代吻合，否则 ErrStaleGen。
  func (e *Engine) ConfirmUnlock(requestID string, gen int64, leaseToken string) (*ReleaseCase, error) {
	  rc, err := e.store.GetCase(requestID)
	  if err != nil {
		  return nil, err
	   }
	   if rc.State.Closed() {
		   return nil, ErrCaseClosed
	   }
	  if rc.State != StateUnlocking {
		  return nil, fmt.Errorf("%w: expected unlocking, got %s", ErrBadState, rc.State)
	   }
	   if rc.Gen != gen {
		   return nil, ErrStaleGen
	  }
	   // 核对租约 token。
	   l, lerr := e.store.GetLease(requestID)
	  if lerr != nil {
		   return nil, fmt.Errorf("dualrelease: lease missing on ConfirmUnlock: %w", lerr)
	   }
	   if l.LeaseToken != leaseToken {
		  return nil, ErrStaleGen
	  }
	  now := e.clock()
	  rc.State = StateExecuting
	   rc.UpdatedAt = now
	  rc.Fence.Epoch++
	   rc.Fence.Token = persist.NewToken()
	  if err := e.store.PutCase(rc); err != nil {
		   return nil, err
	   }
	   return rc, nil
  }

// ──────────────────────────────────────────────────
// RevertUnlock Unlock 失败回调（unlocking→ready）
// ──────────────────────────────────────────────────
// RevertUnlock 在 kms.Unlocker.Unlock 失败后调用，状态回退到 ready，
// 释放互斥租约，确保后续 Execute 被阻断直到重新 AcquireLease。
  func (e *Engine) RevertUnlock(requestID string, gen int64) (*ReleaseCase, error) {
	  rc, err := e.store.GetCase(requestID)
	  if err != nil {
		  return nil, err
	   }
	   if rc.State != StateUnlocking {
		   return nil, fmt.Errorf("%w: expected unlocking, got %s", ErrBadState, rc.State)
	   }
	   if rc.Gen != gen {
		   return nil, ErrStaleGen
	  }
	  now := e.clock()
	  // 回退到 ready，世代保持（防止旧世代 Execute 通过）。
	   rc.State = StateReady
	  rc.UpdatedAt = now
	  rc.Fence.Epoch++
	   rc.Fence.Token = persist.NewToken()
	  if err := e.store.PutCase(rc); err != nil {
		   return nil, err
	   }
	  // 使租约立即过期（设置 ExpiresAt 为过去）。
	   if l, lerr := e.store.GetLease(requestID); lerr == nil {
		  l.ExpiresAt = now.Add(-time.Second)
		   l.UpdatedAt = now
		   _ = e.store.PutLease(l)
	   }
	   return rc, nil
  }

// ──────────────────────────────────────────────────
// CheckTicket 票证老化检查
// ──────────────────────────────────────────────────
// CheckTicket 在 Execute 路径前检查票证是否仍在有效期内。
// 超期 → 写入 TicketExpiry 审计 → 将案例标 expired → 返回 ErrTicketExpired。

  func (e *Engine) CheckTicket(facts TicketFacts, actor string) error {
	  now := e.clock()
	   // TTL 检查。
	   if !now.Before(facts.TTL) {
		   exp := &TicketExpiry{
			   ID:        uuid.NewString(),
			   TicketID:  facts.TicketID,
			   RequestID: facts.RequestID,
			   Reason:    ReasonTTLExceeded,
			   EpochAt:   facts.EpochAt,
			  TTLSecs:   int(facts.TTL.Sub(now).Seconds()),
			  ExpiredAt: now,
			   Actor: actor,
			  AuditHash: ComputeAuditHash(facts.TicketID, facts.RequestID, ReasonTTLExceeded, facts.EpochAt, now),
			  CreatedAt: now,
		   }
		  _ = e.store.AppendExpiry(exp)
		  // 将案例标为 expired（尽力，不影响错误语义）。
		  if rc, cerr := e.store.GetCase(facts.RequestID); cerr == nil && !rc.State.Closed() {
			   rc.State = StateExpired
			   rc.UpdatedAt = now
			  rc.Fence.Epoch++
			  rc.Fence.Token = persist.NewToken()
			   _ = e.store.PutCase(rc)
		   }
		  return fmt.Errorf("%w: ticket %s ttl=%s", ErrTicketExpired, facts.TicketID, facts.TTL)
	   }
	  return nil
   }

// CheckEpoch 在 Execute 路径前校验 epoch 是否与世代匹配。
// epoch 不匹配 → 写入 TicketExpiry（EpochMismatch）→ ErrTicketExpired。

   func (e *Engine) CheckEpoch(facts TicketFacts, gen int64, actor string) error {
	   if facts.EpochAt != gen {
		  now := e.clock()
		  exp := &TicketExpiry{
			  ID: uuid.NewString(),
			   TicketID: facts.TicketID,
			   RequestID: facts.RequestID,
			   Reason:    ReasonEpochMismatch,
			  EpochAt:   facts.EpochAt,
			  TTLSecs:   0,
			   ExpiredAt: now,
			   Actor:     actor,
			   AuditHash: ComputeAuditHash(facts.TicketID, facts.RequestID, ReasonEpochMismatch, facts.EpochAt, now),
			   CreatedAt: now,
		   }
		   _ = e.store.AppendExpiry(exp)
		   return fmt.Errorf("%w: ticket epoch %d != gen %d", ErrTicketExpired, facts.EpochAt, gen)
	  }
	   return nil
  }

// ──────────────────────────────────────────────────
// Execute 放行执行（executing→效果证明）
// ──────────────────────────────────────────────────
// ExecuteParams Execute 入参。
type ExecuteParams struct {
	RequestID string
	Actor     string
	// Gen 调用方持有的世代号；与 ReleaseCase.Gen 不符时 ErrStaleGen。
	Gen         int64
	LeaseToken  string
	TicketFacts TicketFacts
	FundObs FundObservation
}

// Execute 尝试完成放行执行，写入 EffectProof。
// 前置：ConfirmUnlock 已成功（State==executing）。
// 后置：EffectProof 入库 → MarkExecuted 将 State→executed。
// 失败路径：
// - gen 不符 → ErrStaleGen（另一并发 Execute 已赢得世代）
// - 票证过期 → ErrTicketExpired + 审计
// - 效果证明写库失败 → 不标 executed，返回错误
   func (e *Engine) Execute(p ExecuteParams) (*EffectProof, *ReleaseCase, error) {
	   if err := actorOK(p.Actor); err != nil {
		   return nil, nil, err
	  }
	   rc, err := e.store.GetCase(p.RequestID)
	   if err != nil {
		  return nil, nil, err
	   }
	  if rc.State.Closed() {
		  return nil, rc, ErrCaseClosed
	   }
	   if rc.State != StateExecuting {
		   return nil, rc, fmt.Errorf("%w: expected executing, got %s", ErrBadState, rc.State)
	  }
	  // 世代 CAS：gen 不符代表另一并发 Execute 已完成，当前调用为旧世代。
	   if rc.Gen != p.Gen {
		   return nil, rc, ErrStaleGen
	  }
	   // 票证过期检查（TTL + epoch）。
	   if err := e.CheckTicket(p.TicketFacts, p.Actor); err != nil {
		  return nil, rc, err
	   }
	  // 构建效果证明。
	   now := e.clock()
	  proofHash := ComputeProofHash(p.RequestID,
		  p.TicketFacts.TicketID, p.FundObs.FundActionID, p.Gen, p.Actor)
	   proof := &EffectProof{
		   ID:        uuid.NewString(),
		   RequestID: p.RequestID,
		  TicketID: p.TicketFacts.TicketID,
		  Gen:          p.Gen,
		   FundActionID: p.FundObs.FundActionID,
		  Actor:        p.Actor,
		  ProofHash: proofHash,
		  SignedAt: now,
		  Fence:     persist.Fence{Epoch: 1, Token: persist.NewToken()},
		  CreatedAt: now,
	   }
	  if err := e.store.PutProof(proof); err != nil {
		   return nil, rc, fmt.Errorf("dualrelease: effect proof write failed: %w", err)
	   }
	  // 标记 executed：必须在 EffectProof 写成功之后。
	  rc.State = StateExecuted
	  rc.FundActionID = p.FundObs.FundActionID
	   rc.EffectProofID = proof.ID
	  rc.UpdatedAt = now
	  rc.Fence.Epoch++
	  rc.Fence.Token = persist.NewToken()
	   if err := e.store.PutCase(rc); err != nil {
		  // 证明已写，但 case 未翻；需人工或 worker 补偿一致。
		   return proof, rc, fmt.Errorf("dualrelease: case update failed after proof write: %w", err)
	  }
	   return proof, rc, nil
   }

// ──────────────────────────────────────────────────
// ForceAbort 强制中止（双人确认）
// ──────────────────────────────────────────────────
// ForceAbort 双人强制中止放行案例；只有 ready/unlocking/executing 状态可中止。
// 需两个不同 actor 连署：第一次调用记录申请 actor，第二次核对第二人不同后落库。
// 资金侧：本函数不产生任何资金副作用，调用方负责回滚占用（只读资金动作）。

   func (e *Engine) ForceAbort(requestID, actor1, actor2, reason string) (*ReleaseCase, error) {
	   if err := actorOK(actor1); err != nil {
		  return nil, err
	  }
	  if err := actorOK(actor2); err != nil {
		  return nil, err
	   }
	   if strings.EqualFold(actor1, actor2) {
		  return nil, fmt.Errorf("dualrelease: force abort requires two distinct actors")
	   }
	   rc,
		  err := e.store.GetCase(requestID)
	  if err != nil {
		   return nil, err
	  }
	   if rc.State.Closed() {
		   return nil, ErrCaseClosed
	  }
	   now := e.clock()
	  rc.State = StateAborted
	  rc.AbortReason = fmt.Sprintf("force_abort by %s+%s: %s", actor1, actor2, reason)
	  rc.UpdatedAt = now
	  rc.Fence.Epoch++
	   rc.Fence.Token = persist.NewToken()
	  if err := e.store.PutCase(rc); err != nil {
		   return nil, err
	  }
	   // 回收租约。
	   if l, lerr := e.store.GetLease(requestID); lerr == nil {
		  l.ExpiresAt = now.Add(-time.Second)
		  l.UpdatedAt = now
		   _ = e.store.PutLease(l)
	   }
	   return rc, nil
  }

// ──────────────────────────────────────────────────
// GetProof 查询效果证明
// ──────────────────────────────────────────────────
// GetProof 按 requestID + gen 查询效果证明。
   func (e *Engine) GetProof(requestID string, gen int64) (*EffectProof, error) {
	   return e.store.GetProof(requestID, gen)
   }

// GetCase 查询放行案例。

   func (e *Engine) GetCase(requestID string) (*ReleaseCase, error) {
	   return e.store.GetCase(requestID)
   }

// ──────────────────────────────────────────────────
// MarkExpired 票证超时后将案例标记为 expired
// ──────────────────────────────────────────────────
// MarkExpired 由 ticket aging worker 调用；将过期案例批量标 expired。
   func (e *Engine) MarkExpired(cases []*ReleaseCase) []error {
	  now := e.clock()
	  errs := make([]error, 0)
	   for _, rc := range cases {
		   if rc.State.Closed() {
			  continue
		  }
		  rc.State = StateExpired
		  rc.UpdatedAt = now
		   rc.Fence.Epoch++
		   rc.Fence.Token = persist.NewToken()
		   if err := e.store.PutCase(rc); err != nil {
			  errs = append(errs, fmt.Errorf("dualrelease: mark expired %s: %w", rc.RequestID, err))
		   }
	  }
	  return errs
  }
