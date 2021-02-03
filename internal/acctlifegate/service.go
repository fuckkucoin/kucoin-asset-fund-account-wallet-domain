package acctlifegate

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// nowFn 可被覆盖。
var nowFn = func() time.Time { return time.Now().UTC() }

// Service 资金账户生命周期竞态封堵引擎。
type Service struct {
	store Store
}

// New 构造 Service。
   func New(store Store) *Service {
	  return &Service{store: store}
   }

// ── 输入校验辅助 ─────────────────────────────────────────────────────────────

   func validateUserBook(userID, book string) error {
	   if strings.TrimSpace(userID) == "" {
		  return ErrInvalidUserID
	   }
	   if strings.TrimSpace(book) == "" {
		   return ErrInvalidBook
	  }
	  return nil
   }

  func newID() string { return uuid.New().String() }

// ── BookLifeGeneration ───────────────────────────────────────────────────────
// CurrentGeneration 查询 (userID, book) 的最新生命周期世代。

   func (svc *Service) CurrentGeneration(ctx context.Context, userID, book string) (BookLifeGeneration, error) {
	  if err := validateUserBook(userID, book); err != nil {
		   return BookLifeGeneration{}, err
	   }
	  return svc.store.CurrentGeneration(ctx, userID, book)
  }

// ── Lifecycle Operations（含世代联动）────────────────────────────────────────
// OpenBook 开通科目：抬升 life_gen，吊销 open 租约（解冻路径也会调用）。
// 前置条件：当前科目须处于 frozen 或初始（gen=0）状态，调用方自行确认；
// acctlifegate 层仅负责世代管理，不重复校验科目状态（交由 account 层）。
  func (svc *Service) OpenBook(ctx context.Context, req OpenRequest) (BookLifeGeneration, error) {
	   if err := validateUserBook(req.UserID, req.Book); err != nil {
		   return BookLifeGeneration{}, err
	  }
	   // 抬升世代。
	  gen, err := svc.store.BumpGeneration(ctx, req.UserID, req.Book, "open", req.Actor)
	  if err != nil {
		   return gen, err
	   }
	  // 吊销旧 open 租约（open 后旧令牌无意义）。
	  now := nowFn()
	  note := fmt.Sprintf("book opened at gen %d", gen.Generation)
	   _, _ = svc.store.RevokeOpenLeases(ctx, req.UserID, req.Book, now, note)
	   return gen, nil
   }

// FreezeBook 冻结科目，含 Hop/Life 互斥检查。
// 互斥规则：若 (userID, fromBook) 或 (userID, toBook) 存在 pending MoveBind，
// 拒绝本次冻结并返回 ErrMoveBindInFlight，由调用方决定是否排队或通知用户重试。
// 成功后抬升 life_gen 并吊销所有 open 租约，使在途 Move 的旧令牌作废。
   func (svc *Service) FreezeBook(ctx context.Context,
	   req FreezeRequest) (BookLifeGeneration, error) {
	   if err := validateUserBook(req.UserID, req.Book); err != nil {
		  return BookLifeGeneration{}, err
	   }
	  if req.Frozen {
		  // 冻结：先检查是否有在途 Move。
		   if err := svc.assertNoPendingBinds(ctx, req.UserID, req.Book); err != nil {
			   return BookLifeGeneration{}, err
		   }
		   gen, err := svc.store.BumpGeneration(ctx, req.UserID, req.Book, "freeze", req.Actor)
		  if err != nil {
			  return gen, err
		  }
		   now := nowFn()
		   note := fmt.Sprintf("book frozen at gen %d: %s", gen.Generation, req.Reason)
		   _, _ = svc.store.RevokeOpenLeases(ctx, req.UserID, req.Book, now, note)
		  return gen, nil
	  }
	  // 解冻：也需抬升世代（解冻后新 Move 须携带新世代）。
	   gen, err := svc.store.BumpGeneration(ctx, req.UserID, req.Book, "unfreeze", req.Actor)
	  if err != nil {
		  return gen, err
	  }
	  now := nowFn()
	   note := fmt.Sprintf("book unfrozen at gen %d", gen.Generation)
	   _, _ = svc.store.RevokeOpenLeases(ctx, req.UserID, req.Book, now, note)
	   return gen, nil
  }

// CloseBook 关闭科目，含 Hop/Life 互斥检查。
// 与 FreezeBook 互斥规则一致：存在 pending MoveBind 则拒绝。

  func (svc *Service) CloseBook(ctx context.Context, req CloseRequest) (BookLifeGeneration, error) {
	  if err := validateUserBook(req.UserID, req.Book); err != nil {
		   return BookLifeGeneration{}, err
	   }
	   if err := svc.assertNoPendingBinds(ctx, req.UserID, req.Book); err != nil {
		  return BookLifeGeneration{}, err
	   }
	   gen, err := svc.store.BumpGeneration(ctx, req.UserID, req.Book, "close", req.Actor)
	   if err != nil {
		  return gen, err
	   }
	  now := nowFn()
	  note := fmt.Sprintf("book closed at gen %d", gen.Generation)
	   _,
		  _ = svc.store.RevokeOpenLeases(ctx, req.UserID, req.Book, now, note)
	   return gen, nil
   }

// assertNoPendingBinds 检查 (userID, book) 是否存在 pending MoveBind。

   func (svc *Service) assertNoPendingBinds(ctx context.Context, userID, book string) error {
	   pending, err := svc.store.ListPendingMoveBinds(ctx, userID, book)
	   if err != nil {
		  return fmt.Errorf("acctlifegate: check pending binds: %w", err)
	   }
	  if len(pending) > 0 {
		  return fmt.Errorf("%w: %d bind(s) pending for user=%s book=%s",
			   ErrMoveBindInFlight, len(pending), userID, book)
	   }
	  return nil
  }

// ── LifeLease（租约发放与校验）──────────────────────────────────────────────
// AcquireLease GateMove 调用方在 Move 执行前取得生命周期世代租约。
// 发放前校验：
// 1. 检查 (userID, book) 是否存在 open/proven 漂移案件 → 若有则拒绝（ErrDriftBlockWrite）。
// 2. 读取当前世代；generation=0 且科目 book 未被明确操作过时亦可发放
// （funding book 默认 active，generation=0 表示从未被冻结/关闭）。
// 3. 写入新租约，State=open。
  func (svc *Service) AcquireLease(ctx context.Context, req AcquireLeaseRequest) (*LifeLease, error) {
	   if err := validateUserBook(req.UserID, req.Book); err != nil {
		   return nil, err
	  }
	  // 漂移屏蔽检查。
	   if err := svc.CheckDebitAllowed(ctx, req.UserID, req.Book); err != nil {
		   return nil, err
	  }
	   cur, err := svc.store.CurrentGeneration(ctx, req.UserID, req.Book)
	  if err != nil {
		   return nil, fmt.Errorf("acctlifegate: acquire_lease read gen: %w", err)
	  }
	   ttl := req.TTL
	   if ttl <= 0 {
		  ttl = DefaultLeaseTTL
	   }
	  now := nowFn()
	  lease := LifeLease{
		   LeaseID: newID(),
		  UserID:  req.UserID,
		  Book:       req.Book,
		  Generation: cur.Generation,
		   State:      LeaseStateOpen,
		   IssuedAt:   now,
		   ExpiresAt:  now.Add(ttl),
	  }
	  if err := svc.store.InsertLease(ctx, lease); err != nil {
		   return nil, fmt.Errorf("acctlifegate: acquire_lease insert: %w", err)
	  }
	  return &lease, nil
   }

// ValidateLease 校验租约是否可被 Move 消费。
// 校验顺序：
// 1. 租约必须存在。
// 2. 租约必须处于 open 状态（已 used/expired/revoked 均拒绝）。
// 3. 租约未超期（TTL 未到）。
// 4. 租约携带的 generation 须等于当前 per-(user,book) 最新 generation。
// 若 Freeze/Close 在 AcquireLease 之后发生则 generation 已抬升，本次租约作废。
   func (svc *Service) ValidateLease(ctx context.Context, leaseID, userID, book string) (*LifeLease, error) {
	   lease, err := svc.store.GetLease(ctx, leaseID)
	  if err != nil {
		  return nil, err
	   }
	  if lease.UserID != userID || lease.Book != book {
		   return nil, fmt.Errorf("%w: lease user/book mismatch", ErrInvalidLease)
	  }
	   now := nowFn()
	  switch lease.State {
	  case LeaseStateRevoked:
		  return nil, ErrLeaseRevoked
	   case LeaseStateUsed:
		   return nil, ErrLeaseUsed
	  case LeaseStateExpired:
		  return nil, ErrLeaseExpired
	   }
	  if lease.IsExpired(now) {
		   // 状态尚未被 worker 推进，但 TTL 已过。
		   return nil, ErrLeaseExpired
	   }
	   // 世代一致性检查：租约世代须等于当前最新世代。
	   cur, err := svc.store.CurrentGeneration(ctx, userID, book)
	   if err != nil {
		  return nil, fmt.Errorf("acctlifegate: validate_lease read gen: %w", err)
	   }
	  if cur.Generation != lease.Generation {
		   return nil, fmt.Errorf("%w: lease gen=%d current gen=%d",
			  ErrGenerationMismatch, lease.Generation, cur.Generation)
	  }
	   return lease, nil
   }

// ── MoveBind（在途 Move 互斥屏障）───────────────────────────────────────────
// BindMove Move 执行前创建在途绑定并消费租约（标记为 used）。
// 幂等：若 idemKey 已存在且状态为 pending，直接返回已有绑定（幂等重试安全）。

   func (svc *Service) BindMove(ctx context.Context, req BindMoveRequest) (*MoveBind, error) {
	  if err := validateUserBook(req.UserID, req.FromBook); err != nil {
		  return nil, err
	   }
	  if strings.TrimSpace(req.IdemKey) == "" {
		   return nil, fmt.Errorf("acctlifegate: bind_move idem_key required")
	   }
	   // 幂等检查。
	  existing, err := svc.store.GetMoveBindByIdem(ctx, req.IdemKey)
	   if err == nil {
		   return existing, nil
	   }
	  if err != ErrMoveBindNotFound {
		  return nil, fmt.Errorf("acctlifegate: bind_move idem check: %w", err)
	   }
	   // 校验租约。
	   lease, err := svc.ValidateLease(ctx, req.LeaseID, req.UserID, req.FromBook)
	   if err != nil {
		   return nil, err
	   }
	  now := nowFn()
	   bind := MoveBind{
		  BindID: newID(),
		   UserID: req.UserID,
		  FromBook: req.FromBook,
		  ToBook: req.ToBook,
		  Asset: req.Asset,
		  Amount: req.Amount,
		   LeaseID: req.LeaseID,
		   Generation: lease.Generation,
		  IdemKey: req.IdemKey,
		  State:     MoveBindPending,
		  BoundAt:   now,
		   ExpiresAt: now.Add(MoveBindTTL),
	   }
	   if err := svc.store.InsertMoveBind(ctx, bind); err != nil {
		  return nil, fmt.Errorf("acctlifegate: bind_move insert: %w", err)
	  }
	   // 消费租约（标记为 used），防止同一租约绑定多条 Move。
	   if err := svc.store.UpdateLeaseState(ctx, req.LeaseID, LeaseStateOpen, LeaseStateUsed, now, ""); err != nil {
		   // 若已被并发消费，返回 ErrLeaseUsed。
		  if err == ErrGenerationMismatch {
			   return nil, ErrLeaseUsed
		   }
		  return nil, fmt.Errorf("acctlifegate: bind_move mark_lease_used: %w", err)
	  }
	   return &bind, nil
  }

// ReleaseMove Move 结束后释放绑定（completed 或 compensated）。
// 幂等：若绑定已处于终态则直接返回，不报错。
  func (svc *Service) ReleaseMove(ctx context.Context, req ReleaseMoveRequest) error {
	   if strings.TrimSpace(req.BindID) == "" {
		   return fmt.Errorf("acctlifegate: release_move bind_id required")
	  }
	  bind, err := svc.store.GetMoveBind(ctx, req.BindID)
	   if err != nil {
		   return err
	   }
	   if bind.State != MoveBindPending {
		   // 已到终态：幂等返回。
		  return nil
	   }
	   now := nowFn()
	  next := req.State
	  if next != MoveBindCompleted && next != MoveBindCompensated {
		   return fmt.Errorf("acctlifegate: release_move invalid state %s", next)
	   }
	   return svc.store.UpdateMoveBindState(ctx,
		   req.BindID, MoveBindPending, next, now, req.Note)
  }

// ── 漂移屏蔽检查 ─────────────────────────────────────────────────────────────
// CheckDebitAllowed 检查 (userID, book) 是否被漂移案件屏蔽了借记。
// 若存在 open/proven 案件则返回 ErrDriftBlockWrite。

   func (svc *Service) CheckDebitAllowed(ctx context.Context, userID, book string) error {
	   if err := validateUserBook(userID, book); err != nil {
		   return err
	   }
	  cases, err := svc.store.ListOpenDriftCases(ctx, userID, book)
	  if err != nil {
		  return fmt.Errorf("acctlifegate: check_debit_allowed: %w", err)
	   }
	  for _, c := range cases {
		   if c.IsBlockingDebit() {
			   return fmt.Errorf("%w: case_id=%s detected_at=%s",
				   ErrDriftBlockWrite, c.CaseID, c.DetectedAt.Format(time.RFC3339))
		  }
	   }
	   return nil
  }

// ── 半开漂移检测 ─────────────────────────────────────────────────────────────
// ReportDrift 上报半开漂移事件（CheckHop active → Move frozen/closed 夹缝）。
// 调用时机：GateMove/Move 发现科目在 CheckHop 之后已被冻结/关闭，由上层构造
// ReportDriftRequest 调用本方法。
// 写入 LifeDriftCase（状态 open，BlockedAt=now），从此同 (user,book) 借记
// 均受 CheckDebitAllowed 屏蔽，直至案件 closed。
  func (svc *Service) ReportDrift(ctx context.Context, req ReportDriftRequest) (*LifeDriftCase, error) {
	  if err := validateUserBook(req.UserID, req.Book); err != nil {
		  return nil, err
	  }
	   now := nowFn()
	   c := LifeDriftCase{
		  CaseID: newID(),
		  UserID: req.UserID,
		  Book: req.Book,
		   DetectedAt:    now,
		  CheckHopAt:    req.CheckHopAt,
		   MoveAttemptAt: req.MoveAttemptAt,
		  StatusAtCheck: req.StatusAtCheck,
		  StatusAtMove: req.StatusAtMove,
		  LeaseID:        req.LeaseID,
		  MoveIdemKey:    req.MoveIdemKey,
		  LifeGenAtCheck: req.LifeGenAtCheck,
		  LifeGenAtMove:  req.LifeGenAtMove,
		  State:          DriftStateOpen,
		   BlockedAt: &now,
	   }
	   if err := svc.store.InsertDriftCase(ctx, c); err != nil {
		   return nil, fmt.Errorf("acctlifegate: report_drift insert: %w", err)
	   }
	   return &c, nil
  }

// ── 漂移案件 ops 收口 ────────────────────────────────────────────────────────
// GetDriftCase 按 ID 查询漂移案件。
  func (svc *Service) GetDriftCase(ctx context.Context, caseID string) (*LifeDriftCase, error) {
	   return svc.store.GetDriftCase(ctx, caseID)
  }

// ListOpenDriftCases 查询 (userID, book) 的未关闭漂移案件。

  func (svc *Service) ListOpenDriftCases(ctx context.Context, userID, book string) ([]LifeDriftCase, error) {
	  return svc.store.ListOpenDriftCases(ctx,
		   userID, book)
   }

// ProveDrift ops 双人确认漂移证明。
// 前置条件：案件须处于 open 状态；actor1、actor2、proofNote 均不可为空。
// 证明通过后案件转为 proven；此时借记屏蔽仍有效（需 CloseDrift 才释放）。

   func (svc *Service) ProveDrift(ctx context.Context, req ProveDriftRequest) (*LifeDriftCase, error) {
	  if strings.TrimSpace(req.CaseID) == "" {
		  return nil, fmt.Errorf("acctlifegate: prove_drift case_id required")
	  }
	   if strings.TrimSpace(req.Actor1) == "" || strings.TrimSpace(req.Actor2) == "" {
		  return nil, ErrDriftProofIncomplete
	  }
	  if req.Actor1 == req.Actor2 {
		  return nil, fmt.Errorf("acctlifegate: prove_drift actor1 and actor2 must differ")
	  }
	  if strings.TrimSpace(req.ProofNote) == "" {
		   return nil, ErrDriftProofIncomplete
	  }
	   c, err := svc.store.GetDriftCase(ctx, req.CaseID)
	   if err != nil {
		  return nil, err
	   }
	   if c.State != DriftStateOpen {
		  return nil, fmt.Errorf("acctlifegate: prove_drift case state is %s, expected open", c.State)
	   }
	  now := nowFn()
	   if err := svc.store.UpdateDriftState(ctx, req.CaseID, DriftStateOpen, DriftStateProven, now,
		   req.Actor1, req.Actor2, req.ProofNote); err != nil {
		   return nil, fmt.Errorf("acctlifegate: prove_drift update: %w", err)
	  }
	  return svc.store.GetDriftCase(ctx, req.CaseID)
   }

// CloseDrift ops 关闭漂移案件（修复完成，释放借记屏蔽）。
// 前置条件：案件须处于 proven 状态（未经 ProveDrift 确认的案件不得直接关闭）。
  func (svc *Service) CloseDrift(ctx context.Context, req CloseDriftRequest) (*LifeDriftCase, error) {
	   if strings.TrimSpace(req.CaseID) == "" {
		   return nil, fmt.Errorf("acctlifegate: close_drift case_id required")
	  }
	  c,
		   err := svc.store.GetDriftCase(ctx, req.CaseID)
	  if err != nil {
		  return nil, err
	  }
	   if c.State == DriftStateClosed {
		  // 幂等。
		   return c, nil
	   }
	  if c.State != DriftStateProven {
		   return nil, ErrDriftNotProven
	  }
	   now := nowFn()
	  if err := svc.store.UpdateDriftState(ctx, req.CaseID, DriftStateProven, DriftStateClosed, now,
		  "", "", req.CloseNote); err != nil {
		   return nil, fmt.Errorf("acctlifegate: close_drift update: %w", err)
	   }
	  return svc.store.GetDriftCase(ctx, req.CaseID)
   }

// ── 查询聚合 ─────────────────────────────────────────────────────────────────
// BookDriftSummaryFor 返回 (userID, book) 的生命周期与漂移概要（ops 查询用）。
  func (svc *Service) BookDriftSummaryFor(ctx context.Context, userID, book string) (BookDriftSummary, error) {
	  if err := validateUserBook(userID, book); err != nil {
		   return BookDriftSummary{}, err
	  }
	   gen, err := svc.store.CurrentGeneration(ctx, userID, book)
	   if err != nil {
		   return BookDriftSummary{}, err
	   }
	  cases, err := svc.store.ListOpenDriftCases(ctx, userID, book)
	   if err != nil {
		  return BookDriftSummary{}, err
	   }
	  blocked := false
	  for _, c := range cases {
		   if c.IsBlockingDebit() {
			   blocked = true
			  break
		  }
	  }
	   return BookDriftSummary{
		   UserID: userID,
		   Book:         book,
		   Generation:   gen.Generation,
		   OpenCases:    cases,
		   DebitBlocked: blocked,
	   }, nil
  }

// LifeGenViewFor 返回 (userID, book) 的世代视图。

  func (svc *Service) LifeGenViewFor(ctx context.Context, userID, book string) (LifeGenView, error) {
	  if err := validateUserBook(userID, book); err != nil {
		  return LifeGenView{}, err
	  }
	  gen, err := svc.store.CurrentGeneration(ctx, userID, book)
	  if err != nil {
		   return LifeGenView{}, err
	   }
	   return LifeGenView{
		   UserID:     userID,
		   Book:       book,
		   Generation: gen.Generation,
		   LastAction: gen.Action,
		   UpdatedAt: gen.CreatedAt,
	   }, nil
   }

// GetMoveBind 按 BindID 查询 MoveBind。
  func (svc *Service) GetMoveBind(ctx context.Context, bindID string) (*MoveBind, error) {
	  return svc.store.GetMoveBind(ctx, bindID)
   }

// GetLease 按 LeaseID 查询 LifeLease。

  func (svc *Service) GetLease(ctx context.Context, leaseID string) (*LifeLease, error) {
	   return svc.store.GetLease(ctx,
		   leaseID)
  }

// ListPendingMoveBinds 列出 (userID, book) 的在途绑定。

  func (svc *Service) ListPendingMoveBinds(ctx context.Context, userID, book string) ([]MoveBind, error) {
	  return svc.store.ListPendingMoveBinds(ctx,
		  userID, book)
   }
