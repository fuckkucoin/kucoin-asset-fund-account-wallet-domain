package guardlease

import (
	"context"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/pkg/money"
)

// Service 守恒闸快照世代租约服务。
type Service struct {
	mu sync.RWMutex
	store    Store
	halted   bool
	clock    func() time.Time
	leaseTTL time.Duration
	// blockSet 记录当前被挡写的 user×asset 集合（由 fail ConserveProof 触发）。
	blockSet map[string]struct{}
}

// New 创建服务实例（Store 可稍后注入）。
  func New(store Store) *Service {
	   return &Service{
		  store: store,
		  clock:    func() time.Time { return time.Now().UTC() },
		   leaseTTL: DefaultLeaseTTL,
		  blockSet: make(map[string]struct{}),
	  }
   }

// WithTTL 覆盖租约 TTL（用）。
  func (s *Service) WithTTL(d time.Duration) *Service {
	   if s != nil && d > 0 {
		  s.leaseTTL = d
	   }
	  return s
   }

// WithClock 注入时钟（用）。
  func (s *Service) WithClock(fn func() time.Time) *Service {
	  if s != nil && fn != nil {
		  s.clock = fn
	  }
	   return s
  }

  func (s *Service) now() time.Time {
	   if s.clock != nil {
		   return s.clock()
	  }
	  return time.Now().UTC()
   }

  func blockKey(userID, asset string) string {
	  return strings.ToLower(strings.TrimSpace(userID)) + "|" + strings.ToUpper(strings.TrimSpace(asset))
  }

// ---- Halt / Block ----
// SetHalt 全局紧急 halt（在 fail ConserveProof 时可自动触发）。
  func (s *Service) SetHalt(on bool) {
	   s.mu.Lock()
	  s.halted = on
	   s.mu.Unlock()
   }

// Halted 返回全局 halt 状态。
  func (s *Service) Halted() bool {
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	   return s.halted
  }

// BlockWrite 对指定 user×asset 挡写（fail ConserveProof 升级时调用）。
   func (s *Service) BlockWrite(userID, asset string) {
	  k := blockKey(userID, asset)
	   s.mu.Lock()
	   s.blockSet[k] = struct{}{}
	   s.mu.Unlock()
   }

// UnblockWrite 四眼解除后取消 user×asset 挡写。

   func (s *Service) UnblockWrite(userID, asset string) {
	   k := blockKey(userID, asset)
	  s.mu.Lock()
	   delete(s.blockSet, k)
	   s.mu.Unlock()
  }

// IsBlocked 检查 user×asset 是否处于挡写状态。
   func (s *Service) IsBlocked(userID, asset string) bool {
	  k := blockKey(userID, asset)
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	  _, ok := s.blockSet[k]
	   return ok || s.halted
   }

// ---- Lease Issuance ----
// IssueLease Gate/Snapshot 完成后发放世代租约。
// 每次调用递增该 user×asset 世代计数，发放短 TTL 租约并持久化。

  func (s *Service) IssueLease(ctx context.Context, req IssueLeaseRequest) (SnapLease, error) {
	  if s == nil || s.store == nil {
		   return SnapLease{},
			  ErrUnavailable
	  }
	   if s.Halted() {
		  return SnapLease{}, ErrHalted
	  }
	  uid, err := validate.UserID(req.UserID)
	   if err != nil {
		   return SnapLease{}, ErrInvalidUser
	  }
	   as, err := validate.Asset(req.Asset)
	  if err != nil {
		   return SnapLease{}, ErrInvalidAsset
	  }
	   actor,
		  err := validate.Actor(req.ActorID)
	   if err != nil {
		  return SnapLease{}, ErrInvalidActor
	  }
	  hash := strings.TrimSpace(req.SnapshotHash)
	  if hash == "" {
		  return SnapLease{},
			   ErrSnapHashMismatch
	  }
	   ttl := s.leaseTTL
	  if req.TTL > 0 {
		  ttl = req.TTL
	   }
	   gen, err := s.store.IncrGeneration(ctx, uid, as)
	  if err != nil {
		   return SnapLease{}, err
	  }
	  now := s.now()
	   lease := SnapLease{
		  LeaseID: uuid.NewString(),
		   Generation: gen,
		  UserID: uid,
		   Asset:        as,
		   SnapshotHash: hash,
		  State:        LeaseOpen,
		  IssuedAt:     now,
		  ExpiresAt:    now.Add(ttl),
		   ActorID:      actor,
	   }
	  if err := s.store.InsertLease(ctx, lease); err != nil {
		   return SnapLease{}, err
	  }
	   return lease, nil
   }

// GetLease 查询租约。

   func (s *Service) GetLease(ctx context.Context, leaseID string) (SnapLease, error) {
	  if s == nil || s.store == nil {
		   return SnapLease{}, ErrUnavailable
	  }
	  return s.store.GetLease(ctx, strings.TrimSpace(leaseID))
   }

// ---- Conserve Lease Binding ----
// BindLease Conserve 调用前核验并绑定租约。
// 规则：
// 1. 租约必须存在且为 open 状态；
// 2. 租约必须未超期（fail-closed：过期即拒）；
// 3. 快照哈希必须与租约记录一致；
// 4. user×asset 必须匹配；
// 5. 租约期间若存在尚未清除的活账写 FenceRequest（未携带租约的并发写），
// 升 ErrStaleSnap——调用方需重新 Gate/Snapshot；
// 6. 核验通过后将租约推进为 bound 状态。

   func (s *Service) BindLease(ctx context.Context, req BindLeaseRequest) (SnapLease, error) {
	   if s == nil || s.store == nil {
		   return SnapLease{}, ErrUnavailable
	   }
	  if s.Halted() {
		   return SnapLease{}, ErrHalted
	  }
	  uid, err := validate.UserID(req.UserID)
	  if err != nil {
		  return SnapLease{}, ErrInvalidUser
	   }
	  as, err := validate.Asset(req.Asset)
	   if err != nil {
		   return SnapLease{},
			  ErrInvalidAsset
	   }
	  lease, err := s.store.GetLease(ctx, strings.TrimSpace(req.LeaseID))
	   if err != nil {
		   return SnapLease{}, ErrLeaseNotFound
	  }
	  if lease.UserID != uid || lease.Asset != as {
		   return SnapLease{}, ErrLeaseNotFound
	  }
	   if lease.State == LeaseConsumed {
		   return SnapLease{}, ErrLeaseConsumed
	  }
	   if lease.State == LeaseExpired || lease.IsExpired(s.now()) {
		   return SnapLease{}, ErrLeaseExpired
	  }
	  if lease.State != LeaseOpen {
		  return SnapLease{}, ErrLeaseInvalidState
	   }
	  if lease.SnapshotHash != strings.TrimSpace(req.SnapshotHash) {
		   return SnapLease{}, ErrSnapHashMismatch
	  }
	  // 检查是否存在租约窗口内未清除的活账并发写。
	   hasFence,
		   err := s.store.HasActiveFence(ctx, uid, as, lease.LeaseID)
	   if err != nil {
		  return SnapLease{}, err
	   }
	  if hasFence {
		   return SnapLease{}, ErrStaleSnap
	  }
	  now := s.now()
	  extra := map[string]any{"bound_at": now}
	  if err := s.store.UpdateLeaseState(ctx, lease.LeaseID, LeaseOpen, LeaseBound, extra); err != nil {
		  return SnapLease{}, err
	   }
	   lease.State = LeaseBound
	  lease.BoundAt = &now
	   return lease, nil
  }

// ConsumeLease 守恒证明记录后，将 bound 租约推进为 consumed，并清除排队写。
   func (s *Service) ConsumeLease(ctx context.Context, leaseID string) error {
	  if s == nil || s.store == nil {
		  return ErrUnavailable
	  }
	  now := s.now()
	   extra := map[string]any{"consumed_at": now}
	  if err := s.store.UpdateLeaseState(ctx, leaseID, LeaseBound, LeaseConsumed, extra); err != nil {
		  return err
	   }
	   return s.store.ClearFence(ctx, leaseID)
  }

// ---- Live-Write Fence ----
// RegisterFence 在租约窗口内，活账写（credit/debit）在提交前必须调用此方法。
// 若 user×asset 当前有未过期的 open 租约且该写不携带租约，则排队（记录 FenceRequest）。
// 调用方（账本层）在写完账后调用 AckFence（传 lease_id=消费的租约）清除排队状态；
// 若此次写已经携带了租约（来自合法 Conserve 流程的写），传 leaseID 标注归属。
// 规则简化：
// - 携带 leaseID 的写：验证租约 open/bound 且未超期，登记到 leaseID 上；
// - 不携带 leaseID 的写：若当前 user×asset 没有 open 租约，直接放行不记录；
// 若有 open 租约，记录 FenceRequest（后续 BindLease 会检测到并升 ErrStaleSnap）。

   func (s *Service) RegisterFence(ctx context.Context, userID, asset, kind, amount, leaseID string) error {
	  if s == nil || s.store == nil {
		  return ErrUnavailable
	   }
	  if s.Halted() {
		  return ErrHalted
	  }
	  if s.IsBlocked(userID, asset) {
		  return ErrConserveFail
	  }
	   uid, err := validate.UserID(userID)
	  if err != nil {
		   return ErrInvalidUser
	   }
	   as, err := validate.Asset(asset)
	   if err != nil {
		   return ErrInvalidAsset
	   }
	   leaseID = strings.TrimSpace(leaseID)
	  if leaseID == "" {
		   // 无租约写：检查 user×asset 是否有活跃 open 租约。
		  leases, err := s.store.ListActiveLeases(ctx, 1)
		   if err != nil {
			   return err
		  }
		  hasActive := false
		  var activeLease SnapLease
		  for _, l := range leases {
			  if l.UserID == uid && l.Asset == as && !l.IsExpired(s.now()) {
				  hasActive = true
				   activeLease = l
				  break
			   }
		  }
		  if !hasActive {
			   return nil // 无租约窗口，直接放行
		  }
		   // 有活跃租约：记录无租约写（后续 BindLease 检测到 ErrStaleSnap）
		   f := FenceRequest{
			  FenceID: uuid.NewString(),
			  LeaseID: activeLease.LeaseID,
			   UserID: uid,
			  Asset:  as,
			   Kind:     strings.TrimSpace(kind),
			  Amount:   strings.TrimSpace(amount),
			  QueuedAt: s.now(),
		  }
		  return s.store.InsertFence(ctx, f)
	  }
	  // 携带 leaseID 的写：仅验证租约合法性，不记录排队（不属于并发冲突）
	  lease, err := s.store.GetLease(ctx, leaseID)
	   if err != nil {
		  return ErrLeaseNotFound
	   }
	  if lease.UserID != uid || lease.Asset != as {
		   return ErrLeaseNotFound
	   }
	   if lease.State == LeaseConsumed {
		  return ErrLeaseConsumed
	  }
	   if lease.State == LeaseExpired || lease.IsExpired(s.now()) {
		   return ErrLeaseExpired
	  }
	   // 携带有效租约的写属于授权写，不插入 FenceRequest，直接放行。
	  // BindLease 时 HasActiveFence 只检查无租约的并发写，不会误报 ErrStaleSnap。
	  return nil
  }

// ---- Partial Amount Guard ----
// CheckPartialAmt 升格后的部分批次守恒：计数 + 金额双重核验。
// 计数不平衡直接返回错误；计数平衡但金额失衡则记录 PartialImbalance 并返回 ErrPartialImbalance。

  func (s *Service) CheckPartialAmt(ctx context.Context, req CheckPartialAmtRequest) (*PartialImbalance, error) {
	   if s == nil || s.store == nil {
		   return nil, ErrUnavailable
	  }
	   // 计数守恒检查（与原 CheckPartial 语义一致）
	   if req.Total < 0 || req.Submitted < 0 || req.Skipped < 0 || req.Failed < 0 {
		   return nil, ErrPartialImbalance
	   }
	  if req.Submitted+req.Skipped+req.Failed != req.Total {
		   return nil, ErrPartialImbalance
	  }
	   // 金额守恒检查
	   submittedAmt, err := money.ParseBook(orZero(req.SubmittedAmt))
	   if err != nil {
		  return nil, ErrInvalidAmount
	  }
	   skippedAmt, err := money.ParseBook(orZero(req.SkippedAmt))
	   if err != nil {
		  return nil, ErrInvalidAmount
	  }
	   failedAmt, err := money.ParseBook(orZero(req.FailedAmt))
	   if err != nil {
		   return nil, ErrInvalidAmount
	   }
	  expectedTotal, err := money.ParseBook(orZero(req.ExpectedTotal))
	   if err != nil {
		  return nil, ErrInvalidAmount
	   }
	   // actualTotal = submittedAmt + skippedAmt + failedAmt
	  t1, err := submittedAmt.Add(skippedAmt)
	  if err != nil {
		  return nil, ErrInvalidAmount
	  }
	   actualTotal, err := t1.Add(failedAmt)
	  if err != nil {
		  return nil, ErrInvalidAmount
	   }
	  if err := money.Conserve(expectedTotal, actualTotal); err != nil {
		  // 金额失衡：记录 PartialImbalance
		   // delta 取绝对差（actualTotal 可能小于 expectedTotal，Sub 失败则用反向）
		  deltaAmt, derr := actualTotal.Sub(expectedTotal)
		  if derr != nil {
			  deltaAmt, _ = expectedTotal.Sub(actualTotal)
		  }
		  uid,
			  _ := validate.UserID(req.UserID)
		  as, _ := validate.Asset(req.Asset)
		  im := PartialImbalance{
			   ImbalanceID: uuid.NewString(),
			   BatchID: strings.TrimSpace(req.BatchID),
			   UserID: uid,
			   Asset:         as,
			  ExpectedTotal: expectedTotal.String(),
			  ActualTotal:   actualTotal.String(),
			   Delta:         deltaAmt.String(),
			   State:         ImbalanceOpen,
			  DetectedAt: s.now(),
		   }
		   if serr := s.store.InsertImbalance(ctx, im); serr != nil {
			   return nil, serr
		   }
		  return &im, ErrPartialImbalance
	  }
	  return nil, nil
   }

// ---- Conserve Proof ----
// RecordProof 记录守恒证明（before/after Bundle 核对）。
// - 守恒通过：proof 状态设为 ok，消费租约；
// - 守恒失败：proof 状态设为 fail，触发 user×asset 挡写，记录升级时间。
  func (s *Service) RecordProof(ctx context.Context, req ConserveProofRequest) (ConserveProof, error) {
	  if s == nil || s.store == nil {
		   return ConserveProof{}, ErrUnavailable
	   }
	  if s.Halted() {
		  return ConserveProof{}, ErrHalted
	   }
	   uid, err := validate.UserID(req.UserID)
	   if err != nil {
		  return ConserveProof{}, ErrInvalidUser
	  }
	   as, err := validate.Asset(req.Asset)
	  if err != nil {
		   return ConserveProof{}, ErrInvalidAsset
	  }
	   beforeAmt, err := money.ParseBook(orZero(req.BeforeTotal))
	  if err != nil {
		   return ConserveProof{}, ErrInvalidAmount
	   }
	  afterAmt, err := money.ParseBook(orZero(req.AfterTotal))
	  if err != nil {
		  return ConserveProof{}, ErrInvalidAmount
	   }
	  now := s.now()
	   proof := ConserveProof{
		  ProofID: uuid.NewString(),
		   LeaseID: strings.TrimSpace(req.LeaseID),
		   UserID: uid,
		   Asset:       as,
		   BeforeTotal: beforeAmt.String(),
		  AfterTotal: afterAmt.String(),
		  SnapHash:   strings.TrimSpace(req.SnapHash),
		   State: ProofPending,
		   CreatedAt: now,
	   }
	   consErr := money.Conserve(beforeAmt, afterAmt)
	  if consErr != nil {
		  proof.State = ProofFail
		   proof.FailReason = consErr.Error()
		   esc := now
		   proof.EscalatedAt = &esc
		  if err := s.store.InsertProof(ctx, proof); err != nil {
			  return proof, err
		   }
		   // 挡写升级
		  s.BlockWrite(uid, as)
		   return proof, ErrConserveFail
	   }
	   proof.State = ProofOK
	   proof.ResolvedAt = &now
	  if err := s.store.InsertProof(ctx, proof); err != nil {
		   return proof, err
	   }
	  // 消费租约
	   if proof.LeaseID != "" {
		   _ = s.ConsumeLease(ctx, proof.LeaseID)
	  }
	   return proof, nil
   }

// FourEyesClear 四眼解除守恒失败挡写。
// 需要两个不同 actor，且 proof 处于 fail 状态。
   func (s *Service) FourEyesClear(ctx context.Context, req FourEyesClearRequest) (ConserveProof, error) {
	  if s == nil || s.store == nil {
		  return ConserveProof{}, ErrUnavailable
	  }
	  a1, err := validate.Actor(req.Actor1)
	  if err != nil {
		   return ConserveProof{}, ErrInvalidActor
	  }
	  a2,
		   err := validate.Actor(req.Actor2)
	  if err != nil {
		  return ConserveProof{}, ErrInvalidActor
	  }
	  if strings.EqualFold(a1, a2) {
		   return ConserveProof{}, ErrFourEyesRequired
	   }
	   proof, err := s.store.GetProof(ctx, strings.TrimSpace(req.ProofID))
	   if err != nil {
		   return ConserveProof{}, ErrProofNotFound
	  }
	   if proof.State != ProofFail && proof.State != ProofHalted {
		   return ConserveProof{}, ErrLeaseInvalidState
	  }
	   now := s.now()
	   extra := map[string]any{
		   "resolved_at": now,
		  "cleared_by": a1 + "+" + a2,
	   }
	   if err := s.store.UpdateProofState(ctx, proof.ProofID, proof.State, ProofOK, extra); err != nil {
		  return ConserveProof{}, err
	  }
	  proof.State = ProofOK
	  proof.ResolvedAt = &now
	   proof.ClearedBy = a1 + "+" + a2
	   // 解除挡写
	   s.UnblockWrite(proof.UserID, proof.Asset)
	  return proof, nil
  }

// ---- Imbalance Management ----
// ProveImbalance 双人确认失衡，推进到 proven 状态。
   func (s *Service) ProveImbalance(ctx context.Context, imbalanceID, actor1, actor2 string) (PartialImbalance, error) {
	   if s == nil || s.store == nil {
		  return PartialImbalance{}, ErrUnavailable
	  }
	  a1, err := validate.Actor(actor1)
	  if err != nil {
		   return PartialImbalance{}, ErrInvalidActor
	   }
	   a2, err := validate.Actor(actor2)
	  if err != nil {
		  return PartialImbalance{}, ErrInvalidActor
	  }
	   if strings.EqualFold(a1, a2) {
		  return PartialImbalance{}, ErrFourEyesRequired
	   }
	   im,
		   err := s.store.GetImbalance(ctx, strings.TrimSpace(imbalanceID))
	   if err != nil {
		   return PartialImbalance{}, ErrImbalanceNotFound
	   }
	   if im.State != ImbalanceOpen {
		  return PartialImbalance{}, ErrLeaseInvalidState
	   }
	  now := s.now()
	  extra := map[string]any{
		   "proven_at": now,
		  "actor1":    a1,
		  "actor2":    a2,
	   }
	  if err := s.store.UpdateImbalanceState(ctx, im.ImbalanceID, ImbalanceOpen, ImbalanceProven, extra); err != nil {
		  return PartialImbalance{}, err
	  }
	   im.State = ImbalanceProven
	   im.ProvenAt = &now
	  im.Actor1 = a1
	  im.Actor2 = a2
	   return im, nil
   }

// CloseImbalance 关闭已处理的失衡记录。

   func (s *Service) CloseImbalance(ctx context.Context, imbalanceID, actor string) (PartialImbalance, error) {
	  if s == nil || s.store == nil {
		   return PartialImbalance{}, ErrUnavailable
	   }
	  if _, err := validate.Actor(actor); err != nil {
		   return PartialImbalance{}, ErrInvalidActor
	   }
	   im, err := s.store.GetImbalance(ctx, strings.TrimSpace(imbalanceID))
	   if err != nil {
		  return PartialImbalance{},
			  ErrImbalanceNotFound
	   }
	   if im.State != ImbalanceProven {
		  return PartialImbalance{}, ErrLeaseInvalidState
	   }
	   now := s.now()
	   extra := map[string]any{"closed_at": now}
	   if err := s.store.UpdateImbalanceState(ctx, im.ImbalanceID, ImbalanceProven, ImbalanceClosed, extra); err != nil {
		   return PartialImbalance{}, err
	  }
	   im.State = ImbalanceClosed
	   im.ClosedAt = &now
	  return im, nil
   }

// ---- Queries ----
// ListActiveLeases 查询活跃租约列表（供 ops API 使用）。
  func (s *Service) ListActiveLeases(ctx context.Context, limit int) ([]SnapLease, error) {
	  if s == nil || s.store == nil {
		  return nil, ErrUnavailable
	   }
	  if limit <= 0 {
		  limit = 20
	   }
	   return s.store.ListActiveLeases(ctx,
		  limit)
  }

// ListOpenImbalances 查询 open 失衡列表。

  func (s *Service) ListOpenImbalances(ctx context.Context, limit int) ([]PartialImbalance, error) {
	  if s == nil || s.store == nil {
		  return nil, ErrUnavailable
	   }
	   if limit <= 0 {
		   limit = 20
	   }
	   return s.store.ListOpenImbalances(ctx, limit)
  }

// ListFailedProofs 查询 fail 守恒证明列表（等待四眼解除）。

   func (s *Service) ListFailedProofs(ctx context.Context, limit int) ([]ConserveProof, error) {
	   if s == nil || s.store == nil {
		   return nil, ErrUnavailable
	  }
	  if limit <= 0 {
		  limit = 20
	  }
	  return s.store.ListFailedProofs(ctx, limit)
  }

// ---- Utility ----

  func orZero(s string) string {
	  if strings.TrimSpace(s) == "" {
		  return "0"
	   }
	  return s
   }
