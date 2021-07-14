package hmacwritelease

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/hmacfence"
)

// Engine HMAC 写路径租约绑定漂移封堵引擎。
// 功能：集中管理租约签发、写路径复验、Halt 翻转吊销与漂移证明签发。

type Engine struct {
	fence *hmacfence.Service
	ring *hmacfence.KeyRing
	leases LeaseRepo
	nonces NonceSeenRepo
	proofs DriftProofRepo
	gens   HaltGenRepo
	clock  func() time.Time
	mu     sync.Mutex
}

// NewEngine 构造引擎实例。
// 参数 fence 和 ring 为只读引用；Engine 不修改 hmacfence.Service 内部状态。

   func NewEngine(
	  fence *hmacfence.Service,
	  ring *hmacfence.KeyRing,
	   leases LeaseRepo,
	   nonces NonceSeenRepo,
	   proofs DriftProofRepo,
	  gens HaltGenRepo,
  ) *Engine {
	   return &Engine{
		  fence: fence,
		   ring:  ring,
		   leases: leases,
		  nonces: nonces,
		   proofs: proofs,
		  gens:   gens,
		   clock:  func() time.Time { return time.Now().UTC() },
	  }
  }

// WithClock 注入时钟；仅场景使用。
   func (e *Engine) WithClock(c func() time.Time) *Engine {
	   if c != nil {
		  e.clock = c
	   }
	   return e
  }

   // ─────────────────────────────────────────────────────────────────
  // VerifyAndLease：验签 + 签发持久租约
  // ─────────────────────────────────────────────────────────────────
  // VerifyAndLease 执行 HMAC 验签并签发写路径租约。
   // 功能：
   // 1. 从 KeyRing 取出 secret，调用 hmacfence.Service.Verify 执行内存级验签。
   // 2. 查询持久 nonce 防重放仓（hmac_nonce_seen）；若已存在则返回 ErrNonceReplay，
   // 且不签发新租约（内存已记录 nonce 不代表持久已记录）。
  // 3. 读取当前 HaltGeneration；若 Halted=true 则返回 ErrHalted。
   // 4. 构造 HmacWriteLease（含 key_id/nonce/ts/method_path/halt_gen/TTL）并落库。
  // 5. 构造 HmacNonceSeen 并写入持久防重放仓（ExpireAt = now + 2*skew）。
  // 风险点：
   // 步骤 4 与步骤 5 之间若宕机，租约已存在但 nonce 未落持久仓；
  // 重试时 hmacfence.Service.Verify 会返回 ErrReplay（内存已记录），
  // 本函数会在步骤 1 提前返回，不重复签发——设计上可接受此一次性孤立租约。
   func (e *Engine) VerifyAndLease(ctx context.Context, in VerifyAndLeaseInput) (*VerifyAndLeaseResult, error) {
	   if strings.TrimSpace(in.KeyID) == "" || strings.TrimSpace(in.Nonce) == "" {
		   return nil, fmt.Errorf("%w: key_id and nonce required", ErrInvalidInput)
	   }
	  // 步骤 1：hmacfence 内存级验签（含内存 nonce 防重放）。
	   if err := e.fence.VerifyRing(e.ring, in.KeyID, in.TS, in.Nonce, in.Method, in.Path, in.Sig, in.Body); err != nil {
		  return nil, err
	  }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	  now := e.clock()
	   // 步骤 2：持久 nonce 防重放检查。
	  seenID := NoneSeenIDFor(in.KeyID, in.Nonce)
	  if _, err := e.nonces.GetNonceSeen(ctx, seenID); err == nil {
		   // nonce 在持久仓中已存在，视为重放。
		   return nil, ErrNonceReplay
	  }
	  // 步骤 3：Halt 状态复验。
	   gen,
		   err := e.gens.GetHaltGen(ctx)
	   if err != nil {
		  return nil, fmt.Errorf("hmacwritelease.VerifyAndLease: load halt gen: %w", err)
	  }
	   if gen.Halted {
		   return nil, ErrHalted
	   }
	   // 步骤 4：签发 HmacWriteLease。
	   ttl := in.TTL
	  if ttl <= 0 {
		  ttl = DefaultLeaseTTL
	   }
	   leaseID := uuid.NewString()
	  lease := &HmacWriteLease{
		  LeaseID: leaseID,
		  KeyID: in.KeyID,
		  Nonce: in.Nonce,
		   TS: tsToUnix(in.TS),
		   MethodPath: MethodPathFor(in.Method,
			  in.Path),
		   HaltGen:  gen.Generation,
		  State:    LeaseIssued,
		   IssuedAt: now,
		   ExpireAt: now.Add(ttl),
	   }
	  if err := e.leases.SaveLease(ctx, lease); err != nil {
		   return nil, fmt.Errorf("hmacwritelease.VerifyAndLease: save lease: %w", err)
	  }
	   // 步骤 5：持久化 nonce 防重放记录。
	  nonceSeen := &HmacNonceSeen{
		   SeenID: seenID,
		  KeyID:  in.KeyID,
		  Nonce: in.Nonce,
		  LeaseID: leaseID,
		  SeenAt:  now,
		  ExpireAt: now.Add(2 * e.fence.Skew()),
	   }
	   if err := e.nonces.SaveNonceSeen(ctx, nonceSeen); err != nil {
		   return nil, fmt.Errorf("hmacwritelease.VerifyAndLease: save nonce seen: %w", err)
	  }
	   recordLeaseIssuedMetric(in.KeyID)
	   return &VerifyAndLeaseResult{
		   Lease:   lease,
		   HaltGen: gen.Generation,
	  }, nil
  }

// ─────────────────────────────────────────────────────────────────
// WriteCheck：资金写路径复验
// ─────────────────────────────────────────────────────────────────
// WriteCheck 验证资金写提交是否持有有效租约并通过 Halt 世代复验。
// 功能：
// 1. 若 LeaseID 为空，直接 fail-closed 并落 HmacWriteDriftProof（DriftNoLease）。
// 2. 按 LeaseID 查询 HmacWriteLease；不存在则 fail-closed（DriftNoLease）。
// 3. 若租约已吊销，返回 ErrLeaseRevoked 并落 HmacWriteDriftProof（DriftRevoked）。
// 4. 若租约已过 TTL（ExpireAt），返回 ErrLeaseExpired 并落证明（DriftExpired）。
// 5. 读取当前 HaltGeneration.Generation；若 lease.HaltGen != currentGen，
// 返回 ErrHaltGenMismatch 并落证明（DriftHaltGenMismatch）。
// 6. 全部通过后将租约流转到 LeaseBound，更新 BoundAt。
// 风险点：
// 拒绝路径必须先落 HmacWriteDriftProof 再返回 error；禁止静默拒绝。
// WriteCheck 不消费（consume）租约；消费由调用方在写提交落库后调用 ConsumeLease。
  func (e *Engine) WriteCheck(ctx context.Context, in WriteCheckInput) (*WriteCheckResult, error) {
	   now := e.clock()
	  // 步骤 1：无租约 fail-closed。
	   if strings.TrimSpace(in.LeaseID) == "" {
		   pID, _ := e.recordDriftProof(ctx, "", "", "", in.WriteScope, DriftNoLease,
			   "no hmac_write_lease_id provided", 0, now)
		  recordWriteBlockedMetric(in.WriteScope,
			   string(DriftNoLease))
		  return &WriteCheckResult{
			  Allowed:      false,
			   DriftProofID: pID,
			   Reason: DriftNoLease,
			   Detail: "no hmac_write_lease_id provided",
		   }, ErrNoLeaseForWrite
	   }
	  // 步骤 2：查询租约。
	  lease,
		   err := e.leases.GetLease(ctx, in.LeaseID)
	   if err != nil {
		   pID, _ := e.recordDriftProof(ctx, in.LeaseID, "", "", in.WriteScope, DriftNoLease,
			  "lease not found: "+err.Error(), 0, now)
		  recordWriteBlockedMetric(in.WriteScope, string(DriftNoLease))
		   return &WriteCheckResult{
			   Allowed: false,
			  DriftProofID: pID,
			  Reason: DriftNoLease,
			  Detail: "lease not found",
		  }, ErrLeaseNotFound
	  }
	  // 步骤 3：吊销检查。
	   if lease.State == LeaseRevoked {
		  pID, _ := e.recordDriftProof(ctx, lease.LeaseID, lease.KeyID, lease.Nonce, in.WriteScope,
			   DriftRevoked, "lease revoked: "+lease.RevokeNote, lease.HaltGen, now)
		   recordWriteBlockedMetric(in.WriteScope, string(DriftRevoked))
		  return &WriteCheckResult{
			   Allowed: false,
			   Lease:   lease,
			  DriftProofID: pID,
			   Reason: DriftRevoked,
			   Detail: "lease revoked: " + lease.RevokeNote,
		   }, ErrLeaseRevoked
	  }
	   // 步骤 4：过期检查。
	   if !now.Before(lease.ExpireAt) || lease.State == LeaseExpired {
		  pID, _ := e.recordDriftProof(ctx, lease.LeaseID, lease.KeyID, lease.Nonce, in.WriteScope,
			  DriftExpired,
			   fmt.Sprintf("lease expired at %s", lease.ExpireAt.UTC().Format(time.RFC3339)),
			  lease.HaltGen, now)
		  recordWriteBlockedMetric(in.WriteScope, string(DriftExpired))
		  return &WriteCheckResult{
			   Allowed: false,
			  Lease:        lease,
			  DriftProofID: pID,
			  Reason:       DriftExpired,
			  Detail:       fmt.Sprintf("lease expired at %s", lease.ExpireAt.UTC().Format(time.RFC3339)),
		  }, ErrLeaseExpired
	  }
	  // 步骤 5：已消费检查。
	  if lease.State == LeaseConsumed {
		   pID, _ := e.recordDriftProof(ctx, lease.LeaseID, lease.KeyID, lease.Nonce, in.WriteScope,
			  DriftRevoked, "lease already consumed", lease.HaltGen, now)
		   recordWriteBlockedMetric(in.WriteScope, string(DriftRevoked))
		  return &WriteCheckResult{
			  Allowed: false,
			   Lease: lease,
			  DriftProofID: pID,
			  Reason: DriftRevoked,
			   Detail: "lease already consumed",
		   }, ErrLeaseConsumed
	   }
	   // 步骤 6：Halt 世代复验。
	  gen, gErr := e.gens.GetHaltGen(ctx)
	   if gErr != nil {
		   return nil, fmt.Errorf("hmacwritelease.WriteCheck: load halt gen: %w", gErr)
	  }
	  if lease.HaltGen != gen.Generation {
		   detail := fmt.Sprintf("halt_gen mismatch: lease=%d current=%d", lease.HaltGen, gen.Generation)
		  pID,
			   _ := e.recordDriftProof(ctx, lease.LeaseID, lease.KeyID, lease.Nonce, in.WriteScope,
			  DriftHaltGenMismatch, detail, gen.Generation, now)
		  recordWriteBlockedMetric(in.WriteScope, string(DriftHaltGenMismatch))
		   return &WriteCheckResult{
			  Allowed: false,
			   Lease:        lease,
			   DriftProofID: pID,
			   Reason:       DriftHaltGenMismatch,
			  Detail:       detail,
		   }, ErrHaltGenMismatch
	  }
	   // 步骤 7：全部通过，流转到 LeaseBound。
	   t := now
	  lease.BoundAt = &t
	  if tErr := Transition(lease, LeaseBound); tErr != nil {
		   return nil, fmt.Errorf("hmacwritelease.WriteCheck: transition to bound: %w", tErr)
	   }
	  if sErr := e.leases.SaveLease(ctx, lease); sErr != nil {
		  return nil, fmt.Errorf("hmacwritelease.WriteCheck: save lease bound: %w", sErr)
	   }
	  recordWriteAllowedMetric(in.WriteScope)
	  return &WriteCheckResult{
		   Allowed: true,
		   Lease: lease,
	   }, nil
  }

// ConsumeLease 将已通过 WriteCheck 的租约流转到 LeaseConsumed（资金写提交落库后调用）。
// 功能：幂等——已 consumed 的租约直接返回 nil。
// 风险点：必须在资金写入数据库成功后才调用，防止写失败但租约已消费导致资金保护失效。

  func (e *Engine) ConsumeLease(ctx context.Context, leaseID string) error {
	  if strings.TrimSpace(leaseID) == "" {
		  return fmt.Errorf("%w: leaseID required", ErrInvalidInput)
	  }
	   lease, err := e.leases.GetLease(ctx, leaseID)
	  if err != nil {
		   return err
	  }
	   if lease.State == LeaseConsumed {
		   return nil
	  }
	  now := e.clock()
	   t := now
	  lease.ConsumedAt = &t
	  if tErr := Transition(lease, LeaseConsumed); tErr != nil {
		  return tErr
	   }
	  return e.leases.SaveLease(ctx,
		  lease)
   }

// ─────────────────────────────────────────────────────────────────
// Halt 翻转与批量吊销
// ─────────────────────────────────────────────────────────────────
// SetHalt 设置 Halt 状态并（若开启）递增世代、批量吊销旧租约。
// 功能：
// 1. 读取当前 HaltGeneration。
// 2. 若 on=true：递增 Generation，设置 Halted=true，落库。
// 3. 调用 RevokeByHaltGen 批量吊销旧世代租约（Generation < newGen）。
// 4. 若 on=false：设置 Halted=false，落库（不改变 Generation）。
// 重点：SetHalt(true) 后，所有持旧 halt_gen 的 HmacWriteLease 须在下次
// WriteCheck 时因世代不符被拒绝并落 HmacWriteDriftProof。
// 批量吊销确保在 worker 下次扫描前已持久标记失效。
   func (e *Engine) SetHalt(ctx context.Context, on bool) (int64, error) {
	  e.mu.Lock()
	   defer e.mu.Unlock()
	  now := e.clock()
	  gen, err := e.gens.GetHaltGen(ctx)
	   if err != nil {
		   return 0, fmt.Errorf("hmacwritelease.SetHalt: load halt gen: %w", err)
	  }
	  if on {
		  gen.Generation++
		  gen.Halted = true
		   gen.UpdatedAt = now
		   if sErr := e.gens.SaveHaltGen(ctx, gen); sErr != nil {
			   return 0, fmt.Errorf("hmacwritelease.SetHalt: save halt gen: %w", sErr)
		  }
		  // 批量吊销旧世代租约（异步 worker 会接着扫描；此处先处理第一批）。
		   _, _ = e.revokeByHaltGenLocked(ctx, gen.Generation, 200, now, "halt flip: generation "+fmt.Sprintf("%d", gen.Generation))
		   recordHaltFlipMetric(gen.Generation)
		   return gen.Generation, nil
	   }
	   // on=false：解除 Halt，不改变 Generation（已翻转世代不回退）。
	   gen.Halted = false
	   gen.UpdatedAt = now
	  if sErr := e.gens.SaveHaltGen(ctx, gen); sErr != nil {
		  return 0, fmt.Errorf("hmacwritelease.SetHalt: save halt gen (off): %w", sErr)
	   }
	   return gen.Generation, nil
  }

// RevokeByHaltGen 批量吊销 HaltGen < currentGen 的非终态租约。
// 功能：由 HaltRevokeWorker 周期调用；幂等，已终态租约跳过。
   func (e *Engine) RevokeByHaltGen(ctx context.Context, batchSize int) (int, error) {
	   e.mu.Lock()
	   defer e.mu.Unlock()
	  now := e.clock()
	  gen, err := e.gens.GetHaltGen(ctx)
	  if err != nil {
		  return 0, fmt.Errorf("hmacwritelease.RevokeByHaltGen: load halt gen: %w", err)
	  }
	   return e.revokeByHaltGenLocked(ctx, gen.Generation, batchSize, now, "halt revoke worker")
  }

// revokeByHaltGenLocked 在 Engine.mu 锁内批量吊销旧世代租约。
  func (e *Engine) revokeByHaltGenLocked(ctx context.Context, currentGen int64, batchSize int, now time.Time, note string) (int, error) {
	   leases,
		   err := e.leases.ListByHaltGenLT(ctx, currentGen, batchSize)
	   if err != nil {
		  return 0, err
	  }
	  revoked := 0
	   for _, l := range leases {
		   if l.State.IsTerminal() {
			   continue
		  }
		  t := now
		  l.RevokedAt = &t
		  l.RevokeNote = note
		  if tErr := Transition(l, LeaseRevoked); tErr != nil {
			  continue
		  }
		  if sErr := e.leases.SaveLease(ctx, l); sErr != nil {
			   continue
		  }
		   revoked++
		   recordLeaseRevokedMetric(l.KeyID)
	  }
	  return revoked, nil
   }

// ─────────────────────────────────────────────────────────────────
// 过期扫描
// ─────────────────────────────────────────────────────────────────
// ExpireStaleLeases 将 ExpireAt 已过且状态为 issued/bound 的租约流转到 LeaseExpired。
// 功能：由 LeaseExpiryWorker 周期调用；幂等。

   func (e *Engine) ExpireStaleLeases(ctx context.Context, batchSize int) (int, error) {
	   now := e.clock()
	  leases, err := e.leases.ListIssuedExpired(ctx, batchSize)
	  if err != nil {
		   return 0, err
	  }
	   expired := 0
	   for _, l := range leases {
		  if l.State.IsTerminal() {
			  continue
		  }
		  if now.Before(l.ExpireAt) {
			   continue
		  }
		  t := now
		  _ = t
		   if tErr := Transition(l, LeaseExpired); tErr != nil {
			   continue
		   }
		  if sErr := e.leases.SaveLease(ctx, l); sErr != nil {
			  continue
		  }
		  expired++
		  recordLeaseExpiredMetric(l.KeyID)
	   }
	   return expired, nil
  }

// ─────────────────────────────────────────────────────────────────
// 只读查询（供他包引用）
// ─────────────────────────────────────────────────────────────────
// GetLease 返回指定 LeaseID 的 HmacWriteLease（只读）。
  func (e *Engine) GetLease(ctx context.Context, leaseID string) (*HmacWriteLease, error) {
	   return e.leases.GetLease(ctx, leaseID)
  }

// GetDriftProof 返回指定 ProofID 的 HmacWriteDriftProof（只读）。

  func (e *Engine) GetDriftProof(ctx context.Context, proofID string) (*HmacWriteDriftProof, error) {
	   return e.proofs.GetProof(ctx, proofID)
   }

// ListDriftProofsByLeaseID 返回指定租约的漂移证明列表（只读，供审计导出）。
   func (e *Engine) ListDriftProofsByLeaseID(ctx context.Context, leaseID string, limit int) ([]*HmacWriteDriftProof, error) {
	   return e.proofs.ListByLeaseID(ctx, leaseID, limit)
  }

// ListDriftProofsByReason 返回指定原因的漂移证明列表（只读，供审计导出）。

  func (e *Engine) ListDriftProofsByReason(ctx context.Context, reason DriftReason, limit int) ([]*HmacWriteDriftProof, error) {
	  return e.proofs.ListByReason(ctx,
		   reason, limit)
   }

// GetCurrentHaltGen 返回当前 Halt 世代号（只读）。

   func (e *Engine) GetCurrentHaltGen(ctx context.Context) (*HaltGeneration, error) {
	   return e.gens.GetHaltGen(ctx)
  }

// ─────────────────────────────────────────────────────────────────
// 内部辅助
// ─────────────────────────────────────────────────────────────────
// recordDriftProof 构造并落库 HmacWriteDriftProof；失败时仅记录不阻断主流程。
// 返回 ProofID；落库失败时返回空串。

   func (e *Engine) recordDriftProof(
	  ctx context.Context,
	  leaseID, keyID, nonce, writeScope string,
	   reason DriftReason,
	  detail string,
	   haltGen int64,
	   now time.Time,
   ) (string, error) {
	   p := &HmacWriteDriftProof{
		   ProofID: uuid.NewString(),
		  LeaseID: leaseID,
		  KeyID: keyID,
		  Nonce: nonce,
		  WriteScope: writeScope,
		  Reason:     reason,
		   Detail:     detail,
		   AttemptedAt: now,
		   HaltGen:     haltGen,
	   }
	   p.Digest = p.ComputeDigest()
	  if err := e.proofs.SaveProof(ctx, p); err != nil {
		   return "", err
	  }
	  recordDriftProofMetric(string(reason))
	   return p.ProofID, nil
  }

// tsToUnix 将字符串时间戳转为 int64 Unix 秒；转换失败返回 0。
   func tsToUnix(ts string) int64 {
	   var v int64
	  _, _ = fmt.Sscanf(strings.TrimSpace(ts), "%d", &v)
	   return v
   }
