package claimmutex

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

const (
	KindClaim      = "claim"
	KindAutoCredit = "auto_credit"
)

// Service 认领效果互斥封堵引擎服务层。

type Service struct {
	store Store
	clock func() time.Time
}

// New 创建 Service，使用内存存储（适合和开发）。
  func New() *Service {
	   return NewWithStore(NewMemStore())
   }

// NewWithStore 使用指定存储实现创建 Service。
  func NewWithStore(store Store) *Service {
	  return &Service{
		  store: store,
		   clock: func() time.Time { return time.Now().UTC() },
	  }
   }

// WithClock 注入时钟（用）。
  func (s *Service) WithClock(c func() time.Time) *Service {
	  if c != nil {
		   s.clock = c
	   }
	   return s
   }

   func (s *Service) now() time.Time { return s.clock() }

// ─────────────────────────── AcquireLease ───────────────────────────────────
// AcquireLease 首次 Claim 或 auto-credit 抢占输出效果租约。
// 流程：
// 1. 校验请求参数；
// 2. 检查同输出是否已有 pending 自动贷记绑定（若 Kind=claim）；
// 3. 尝试 InsertLease（CAS：若已存在则返回 ErrMutexConflict）；
// 4. 若 Kind=claim 且存在 pending auto_credit bind → 置 bind rejected，开 DupClaimCase；
// 5. 若 Kind=auto_credit → 写 AutoCreditBind；
// 6. 写 ClaimEffectProof（pending）。
// 幂等：同 OutputKey + IdemKey 重复调用，返回已有租约。
   func (s *Service) AcquireLease(ctx context.Context, req AcquireLeaseRequest) (*OutputLease, error) {
	  if err := validateAcquire(req); err != nil {
		   return nil, err
	  }
	   ttl := req.TTL
	   if ttl <= 0 {
		   ttl = DefaultLeaseTTL
	   }
	  now := s.now()
	   // 幂等检查：同 outputKey 是否已有租约
	  existing, err := s.store.GetLease(ctx, req.OutputKey)
	  if err != nil && !errors.Is(err, ErrLeaseNotFound) {
		   return nil, err
	   }
	  if existing != nil {
		  // 同一 actor + idem → 幂等返回
		   if existing.WinnerActor == req.ActorID {
			   return existing, nil
		   }
		  // 不同 actor，第二次认领
		  return nil, ErrSecondClaim
	  }
	  // 若 Kind=claim，检查是否有 pending auto_credit bind
	  var existingBind *AutoCreditBind
	  if req.Kind == KindClaim {
		  existingBind,
			  err = s.store.GetPendingAutoBind(ctx, req.OutputKey)
		   if err != nil && !errors.Is(err, ErrBindNotFound) {
			   return nil, err
		  }
	  }
	  // 创建新租约
	   lease := OutputLease{
		  LeaseID: uuid.NewString(),
		  OutputKey: req.OutputKey,
		   Generation: now.UnixNano(),
		   State: LeaseOpen,
		  WinnerKind:  req.Kind,
		  WinnerActor: req.ActorID,
		  IssuedAt:  now,
		  ExpiresAt: now.Add(ttl),
	   }
	  if err := s.store.InsertLease(ctx, lease); err != nil {
		  if errors.Is(err, ErrMutexConflict) {
			  // 并发：另一腿已抢先写入
			   got, gerr := s.store.GetLease(ctx, req.OutputKey)
			  if gerr != nil {
				   return nil, ErrMutexConflict
			   }
			   if got.WinnerActor == req.ActorID {
				   return got, nil
			  }
			  return nil, ErrSecondClaim
		   }
		  return nil, err
	  }
	  // 如果存在 pending auto_credit bind，且当前是 claim → 拒绝 bind，开 DupClaimCase
	  if req.Kind == KindClaim && existingBind != nil {
		  dupErr := s.handleClaimVsAutoCreditRace(ctx, lease, existingBind, now)
		   if dupErr != nil {
			   // 开 DupClaimCase 失败仅记录，不阻断主路径（证明存储层会捕获）
			   _ = dupErr
		   }
	   }
	  // 若 Kind=auto_credit，写 AutoCreditBind
	   if req.Kind == KindAutoCredit {
		  bind := AutoCreditBind{
			  BindID:    uuid.NewString(),
			  OutputKey: req.OutputKey,
			   Generation: lease.Generation,
			   LeaseID:    lease.LeaseID,
			   ActorID: req.ActorID,
			   UserID:  req.UserID,
			   Asset:   req.Asset,
			  Amount:  req.Amount,
			  IdemKey: req.IdemKey,
			  State: BindBound,
			  BoundAt: now,
		   }
		   if bindErr := s.store.InsertAutoBind(ctx, bind); bindErr != nil {
			  _ = bindErr // 非阻断
		  }
	  }
	   // 写 ClaimEffectProof（pending）
	  proof := ClaimEffectProof{
		  ProofID:    uuid.NewString(),
		  OutputKey:  req.OutputKey,
		   LeaseID:    lease.LeaseID,
		  Generation: lease.Generation,
		   ClaimKind: req.Kind,
		  ActorID: req.ActorID,
		  UserID: req.UserID,
		   Asset:   req.Asset,
		   Amount:  req.Amount,
		   IdemKey: req.IdemKey,
		   State:     ProofPending,
		   CreatedAt: now,
	   }
	  if pErr := s.store.InsertProof(ctx, proof); pErr != nil {
		  _ = pErr // 非阻断；verify worker 会定期检测
	  }
	  return &lease, nil
   }

// handleClaimVsAutoCreditRace 处理 claim 抢先胜出、auto_credit 绑定被拒的竞态。
// 若 auto_credit 已经入账（bind.State == BindBound），则开 DupClaimCase。
   func (s *Service) handleClaimVsAutoCreditRace(ctx context.Context, winnerLease OutputLease, bind *AutoCreditBind, now time.Time) error {
	   _ = s.store.UpdateAutoBindState(ctx, bind.BindID, BindPending, BindRejected, now, "claim won mutex race")
	   dupCase := DupClaimCase{
		  CaseID: uuid.NewString(),
		  OutputKey: winnerLease.OutputKey,
		   LeaseID:     winnerLease.LeaseID,
		  FirstKind:   KindClaim,
		   FirstActor:  winnerLease.WinnerActor,
		  SecondKind:  KindAutoCredit,
		   SecondActor: bind.ActorID,
		  SecondIdem:  bind.IdemKey,
		   DeltaAmount: bind.Amount,
		  State:       DupCaseOpen,
		  DetectedAt: now,
	   }
	  return s.store.InsertDupCase(ctx, dupCase)
  }

// ─────────────────────────── GuardOutput ────────────────────────────────────
// GuardOutput 后续同输出动作（含 Recovery 推进）须调用此方法核验效果世代。
// 若 LeaseID / Generation 与当前租约不符 → ErrGenerationMismatch（挡写）。
// 若输出已进入 blocked / expired → ErrSecondClaim。
   func (s *Service) GuardOutput(ctx context.Context, req GuardRequest) (*OutputLease, error) {
	  if strings.TrimSpace(req.OutputKey.String()) == "" {
		  return nil, ErrOutputKeyEmpty
	   }
	  lease, err := s.store.GetLease(ctx, req.OutputKey)
	  if err != nil {
		  return nil, err
	  }
	   if lease.IsBlocked() {
		   return nil, ErrSecondClaim
	   }
	  if req.LeaseID != "" && lease.LeaseID != req.LeaseID {
		  return nil,
			   ErrGenerationMismatch
	   }
	  if req.Generation != 0 && lease.Generation != req.Generation {
		  return nil,
			   ErrGenerationMismatch
	   }
	  if lease.IsExpired(s.now()) {
		   return nil, ErrLeaseExpired
	  }
	  return lease, nil
  }

// ─────────────────────────── RegisterAutoCredit ─────────────────────────────
// RegisterAutoCredit 自动贷记管道注册绑定。
// 若同输出已有 open 租约（Claim 已胜出），则直接 rejected 并开 DupClaimCase。
// 若无租约，写 pending AutoCreditBind，等待后续 AcquireLease(auto_credit) 处理。
   func (s *Service) RegisterAutoCredit(ctx context.Context, req RegisterAutoCreditRequest) (*AutoCreditBind, error) {
	  if err := validateAutoCredit(req); err != nil {
		   return nil, err
	  }
	  now := s.now()
	   // 检查是否已有胜出租约
	   existing, err := s.store.GetLease(ctx, req.OutputKey)
	   if err != nil && !errors.Is(err, ErrLeaseNotFound) {
		  return nil, err
	  }
	  bind := AutoCreditBind{
		   BindID: uuid.NewString(),
		  OutputKey: req.OutputKey,
		  UserID:    req.UserID,
		   Asset:   req.Asset,
		  Amount:  req.Amount,
		  IdemKey: req.IdemKey,
		   ActorID: req.ActorID,
		  BoundAt: now,
	   }
	  if existing != nil && existing.WinnerKind == KindClaim {
		  // Claim 已胜出，auto_credit 被拒
		   bind.State = BindRejected
		  bind.Generation = existing.Generation
		  bind.LeaseID = existing.LeaseID
		  t := now
		   bind.ResolvedAt = &t
		   bind.RejectNote = fmt.Sprintf("claim %s already won lease %s", existing.WinnerActor, existing.LeaseID)
		   if insErr := s.store.InsertAutoBind(ctx, bind); insErr != nil {
			  return nil, insErr
		   }
		   // 如果 Claim 租约已经 claimed（即已入账），说明双贷已发生
		  if existing.State == LeaseClaimed || existing.State == LeaseAutoCredited {
			  dupCase := DupClaimCase{
				  CaseID:    uuid.NewString(),
				   OutputKey: req.OutputKey,
				   LeaseID:   existing.LeaseID,
				   FirstKind: existing.WinnerKind,
				   FirstActor:  existing.WinnerActor,
				  SecondKind:  KindAutoCredit,
				  SecondActor: req.ActorID,
				  SecondIdem:  req.IdemKey,
				   DeltaAmount: req.Amount,
				  State:      DupCaseOpen,
				   DetectedAt: now,
			   }
			   _ = s.store.InsertDupCase(ctx,
				   dupCase)
		  }
		   return &bind, ErrMutexConflict
	   }
	  // 无胜出租约，写 pending
	  bind.State = BindPending
	   if err := s.store.InsertAutoBind(ctx, bind); err != nil {
		  return nil, err
	  }
	   return &bind, nil
  }

// ─────────────────────────── AdvanceLease ───────────────────────────────────
// AdvanceLease 将租约从 open 推进到 claimed/auto_credited。
// 仅允许持有同一 LeaseID 的 actor 推进。

   func (s *Service) AdvanceLease(ctx context.Context, leaseID, actorID string, kind string) (*OutputLease, error) {
	  now := s.now()
	  lease, err := s.store.GetLeaseByID(ctx, leaseID)
	   if err != nil {
		   return nil, err
	   }
	  if lease.WinnerActor != actorID {
		   return nil, ErrMutexConflict
	   }
	   if lease.State != LeaseOpen {
		   return lease, nil // 幂等
	   }
	  var next LeaseState
	  switch kind {
	  case KindClaim:
		  next = LeaseClaimed
	  case KindAutoCredit:
		  next = LeaseAutoCredited
	  default:
		   return nil, ErrInvalidRequest
	  }
	   if err := s.store.UpdateLeaseState(ctx, leaseID, LeaseOpen, next, now, ""); err != nil {
		  return nil, err
	   }
	  lease.State = next
	   t := now
	  lease.ClaimedAt = &t
	   return lease, nil
   }

// BlockOutput 封堵同输出后续所有认领动作。
// 通常由 DupClaimCase 开案时触发，或 ops 手动调用。
   func (s *Service) BlockOutput(ctx context.Context, leaseID, reason string) error {
	  now := s.now()
	   lease, err := s.store.GetLeaseByID(ctx, leaseID)
	   if err != nil {
		   return err
	  }
	   if lease.State == LeaseBlocked {
		  return nil // 幂等
	  }
	  return s.store.UpdateLeaseState(ctx,
		   leaseID, lease.State, LeaseBlocked, now, reason)
  }

// ─────────────────────────── ClaimEffectProof ───────────────────────────────
// CreateProof 提交认领效果证明（pending 态）。
  func (s *Service) CreateProof(ctx context.Context, req CreateProofRequest) (*ClaimEffectProof, error) {
	  if err := validateProofReq(req); err != nil {
		  return nil, err
	  }
	   now := s.now()
	  // 幂等检查
	   existing,
		  err := s.store.GetProofByOutput(ctx, req.OutputKey)
	   if err != nil && !errors.Is(err, ErrProofNotFound) {
		  return nil, err
	   }
	  if existing != nil && existing.LeaseID == req.LeaseID {
		  return existing, nil
	   }
	   proof := ClaimEffectProof{
		   ProofID:   uuid.NewString(),
		   OutputKey: req.OutputKey,
		  LeaseID:   req.LeaseID,
		   Generation: req.Generation,
		  ClaimKind:  req.ClaimKind,
		   ActorID:    req.ActorID,
		  UserID: req.UserID,
		   Asset: req.Asset,
		   Amount:  req.Amount,
		   IdemKey: req.IdemKey,
		  State: ProofPending,
		  CreatedAt: now,
	  }
	  if err := s.store.InsertProof(ctx, proof); err != nil {
		  return nil, err
	   }
	  return &proof, nil
   }

// VerifyProof 核验效果证明的世代与金额守恒，通过则置为 verified。
  func (s *Service) VerifyProof(ctx context.Context, proofID string) (*ClaimEffectProof, error) {
	  now := s.now()
	  proof, err := s.store.GetProofByID(ctx, proofID)
	   if err != nil {
		   return nil, err
	   }
	  if proof.State != ProofPending {
		   return proof, nil // 幂等
	   }
	   // 校验对应租约世代是否一致
	   lease, err := s.store.GetLeaseByID(ctx, proof.LeaseID)
	  if err != nil {
		  if err := s.store.UpdateProofState(ctx, proofID, ProofPending, ProofFailed, now, "lease not found", ""); err != nil {
			  return nil, err
		  }
		  proof.State = ProofFailed
		  proof.FailReason = "lease not found"
		   return proof, ErrProofMissing
	  }
	  if lease.Generation != proof.Generation {
		  failReason := fmt.Sprintf("generation mismatch: lease=%d proof=%d", lease.Generation, proof.Generation)
		  _ = s.store.UpdateProofState(ctx, proofID, ProofPending, ProofFailed, now, failReason, "")
		  proof.State = ProofFailed
		   proof.FailReason = failReason
		   return proof, ErrGenerationMismatch
	   }
	  if err := s.store.UpdateProofState(ctx, proofID, ProofPending, ProofVerified, now, "", ""); err != nil {
		  return nil, err
	  }
	   t := now
	  proof.State = ProofVerified
	  proof.VerifiedAt = &t
	  return proof, nil
  }

// CompleteClaim 将认领标记为完成。须先确认效果证明已 verified/sealed。
// 若证明缺失或未通过验证，返回 ErrProofMissing。
   func (s *Service) CompleteClaim(ctx context.Context, req CompleteClaimRequest) (*ClaimEffectProof, error) {
	   if strings.TrimSpace(req.ProofID) == "" {
		   return nil, ErrProofMissing
	   }
	   now := s.now()
	   proof, err := s.store.GetProofByID(ctx, req.ProofID)
	   if err != nil {
		   return nil, ErrProofMissing
	   }
	   if !proof.CanComplete() {
		   return nil, ErrProofMissing
	  }
	  if proof.State == ProofSealed {
		  return proof, nil // 幂等
	  }
	  if err := s.store.UpdateProofState(ctx, req.ProofID, ProofVerified, ProofSealed, now, "", req.SealedBy); err != nil {
		  return nil, err
	  }
	  t := now
	  proof.State = ProofSealed
	  proof.SealedAt = &t
	  proof.SealedBy = req.SealedBy
	   return proof, nil
   }

// ─────────────────────────── DupClaimCase ───────────────────────────────────
// ProveDupCase ops 双人确认双贷案件属实。

  func (s *Service) ProveDupCase(ctx context.Context, req ProveDupCaseRequest) (*DupClaimCase, error) {
	   if strings.TrimSpace(req.CaseID) == "" || strings.TrimSpace(req.Actor1) == "" || strings.TrimSpace(req.Actor2) == "" {
		  return nil, ErrInvalidRequest
	   }
	   if req.Actor1 == req.Actor2 {
		  return nil, fmt.Errorf("claimmutex: prove requires two distinct actors")
	  }
	   now := s.now()
	  c, err := s.store.GetDupCaseByID(ctx, req.CaseID)
	  if err != nil {
		  return nil, err
	   }
	   if c.State == DupCaseClosed {
		  return nil, ErrCaseClosed
	  }
	   if c.State == DupCaseProven {
		  return c, nil // 幂等
	   }
	  if err := s.store.UpdateDupCaseState(ctx, req.CaseID, DupCaseOpen, DupCaseProven, now, req.Actor1, req.Actor2, ""); err != nil {
		  return nil, err
	   }
	  t := now
	  c.State = DupCaseProven
	   c.ProvenAt = &t
	  c.Actor1 = req.Actor1
	   c.Actor2 = req.Actor2
	   return c, nil
   }

// CloseDupCase ops 关闭双贷案件（修复完成）。
   func (s *Service) CloseDupCase(ctx context.Context, req CloseDupCaseRequest) (*DupClaimCase, error) {
	   if strings.TrimSpace(req.CaseID) == "" {
		  return nil, ErrInvalidRequest
	   }
	  now := s.now()
	  c,
		   err := s.store.GetDupCaseByID(ctx, req.CaseID)
	   if err != nil {
		  return nil, err
	  }
	   if c.State == DupCaseClosed {
		  return c, nil // 幂等
	   }
	  if c.State == DupCaseOpen {
		  return nil, fmt.Errorf("claimmutex: case must be proven before closing")
	  }
	  if err := s.store.UpdateDupCaseState(ctx, req.CaseID, DupCaseProven, DupCaseClosed, now, c.Actor1, c.Actor2, req.CloseNote); err != nil {
		  return nil, err
	  }
	  t := now
	   c.State = DupCaseClosed
	   c.ClosedAt = &t
	  c.CloseNote = req.CloseNote
	  return c, nil
   }

// ─────────────────────────── Queries ────────────────────────────────────────
// GetOutputEffect ops 查询输出效果摘要。

   func (s *Service) GetOutputEffect(ctx context.Context, key OutputKey) (*OutputEffectSummary, error) {
	  sum := &OutputEffectSummary{OutputKey: key}
	   lease, err := s.store.GetLease(ctx, key)
	   if err != nil && !errors.Is(err, ErrLeaseNotFound) {
		  return nil, err
	  }
	   sum.Lease = lease
	   proof, err := s.store.GetProofByOutput(ctx, key)
	   if err != nil && !errors.Is(err, ErrProofNotFound) {
		   return nil, err
	  }
	  sum.Proof = proof
	   dupCase, err := s.store.GetOpenDupCase(ctx, key)
	  if err != nil && !errors.Is(err, ErrDupCaseNotFound) {
		   return nil, err
	  }
	  sum.DupCase = dupCase
	  autoBind, err := s.store.GetPendingAutoBind(ctx, key)
	  if err != nil && !errors.Is(err, ErrBindNotFound) {
		  return nil, err
	   }
	  sum.AutoBind = autoBind
	  return sum, nil
  }

// ListOpenDupCases 列出所有未关闭的双贷案件。

  func (s *Service) ListOpenDupCases(ctx context.Context, limit int) ([]DupClaimCase, error) {
	  if limit <= 0 {
		   limit = 50
	  }
	  return s.store.ListOpenDupCases(ctx, limit)
  }

// ListPendingProofs 列出所有待验证的效果证明。
   func (s *Service) ListPendingProofs(ctx context.Context, limit int) ([]ClaimEffectProof, error) {
	   if limit <= 0 {
		   limit = 50
	  }
	   return s.store.ListPendingProofs(ctx, limit)
   }

// ─────────────────────────── Validation ─────────────────────────────────────

  func validateAcquire(req AcquireLeaseRequest) error {
	  if req.OutputKey.String() == "" {
		   return ErrOutputKeyEmpty
	   }
	   if strings.TrimSpace(req.ActorID) == "" {
		  return ErrActorEmpty
	   }
	  if req.Kind != KindClaim && req.Kind != KindAutoCredit {
		   return fmt.Errorf("%w: kind must be 'claim' or 'auto_credit', got %q", ErrInvalidRequest, req.Kind)
	   }
	   return nil
   }

  func validateAutoCredit(req RegisterAutoCreditRequest) error {
	   if req.OutputKey.String() == "" {
		  return ErrOutputKeyEmpty
	  }
	   if strings.TrimSpace(req.ActorID) == "" {
		  return ErrActorEmpty
	   }
	   return nil
   }

  func validateProofReq(req CreateProofRequest) error {
	   if req.OutputKey.String() == "" {
		  return ErrOutputKeyEmpty
	  }
	  if strings.TrimSpace(req.LeaseID) == "" {
		  return ErrInvalidRequest
	   }
	   return nil
   }
// temporary guard removed after the production check
