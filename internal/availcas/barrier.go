package availcas

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Barrier 是对外暴露的统一屏障接口，供预检闸与提交闸共用。
type Barrier interface {
	// Submit 提交一个扣减/冻结意图，返回裁决结果。
	// 并发安全：不同 userID×asset×book 的调用完全独立；
	// 同一 userID×asset×book 的并发调用通过 CAS 互斥，最多一个 accepted。
	Submit(ctx context.Context, intent *DebitIntent) (*BarrierDecision, error)
	// GetDecision 查询已有意图的裁决结果。
	GetDecision(ctx context.Context, intentID string) (*BarrierDecision, error)
	// IssueLease 为指定用户×资产×账本位颁发新租约。
	IssueLease(ctx context.Context, userID, asset, book string, opts LeaseOptions) (*AvailLease, error)
	// GetLease 查询租约。
	GetLease(ctx context.Context, leaseID string) (*AvailLease, error)
	// GetProof 查询竞态证明。
	GetProof(ctx context.Context, proofID string) (*RaceProof, error)
	// ListProofs 列出用户×资产的竞态证明（最近 limit 条，desk 用）。
	ListProofs(ctx context.Context,
		userID, asset string, limit int) ([]*RaceProof, error)
}

// barrierImpl Barrier 实现。

type barrierImpl struct {
	store Store
	lm    *leaseManager
	clock func() time.Time
}

// NewBarrier 构造屏障实现。reader 由调用层（service.go）传入。
   func NewBarrier(store Store, reader LeaseRequester) Barrier {
	   clock := func() time.Time { return time.Now().UTC() }
	  return &barrierImpl{
		   store: store,
		  lm: newLeaseManager(store,
			  reader, clock),
		   clock: clock,
	   }
  }

// newBarrierWithClock 供注入时钟。
   func newBarrierWithClock(store Store, reader LeaseRequester, clock func() time.Time) *barrierImpl {
	   return &barrierImpl{
		  store: store,
		  lm:    newLeaseManager(store, reader, clock),
		  clock: clock,
	   }
   }

  // IssueLease 委托 leaseManager。

   func (b *barrierImpl) IssueLease(ctx context.Context, userID, asset, book string, opts LeaseOptions) (*AvailLease, error) {
	   return b.lm.Issue(ctx, userID, asset, book, opts)
   }

// GetLease 透传 store。

  func (b *barrierImpl) GetLease(ctx context.Context, leaseID string) (*AvailLease, error) {
	  return b.store.GetLease(ctx, leaseID)
  }

// Submit 提交意图，执行 CAS 裁决。
   func (b *barrierImpl) Submit(ctx context.Context, intent *DebitIntent) (*BarrierDecision, error) {
	   if err := validateIntent(intent); err != nil {
		   return nil, err
	   }
	  now := b.clock()
	   // ── 1. 幂等检测 ─────────────────────────────────────────────────────────
	  if intent.IdemKey != "" {
		  existing,
			   err := b.store.FindIntentByIdem(ctx, intent.IdemKey)
		  if err == nil && existing != nil {
			   // 相同幂等键：返回已有裁决，不重复执行。
			  return buildDecision(existing, nil), nil
		  }
	   }
	   // ── 2. 加载租约 ──────────────────────────────────────────────────────────
	  lease, err := b.store.GetLease(ctx, intent.LeaseID)
	   if err != nil {
		  return nil, fmt.Errorf("availcas barrier: load lease: %w", err)
	   }
	   // ── 3. 租约状态检查 ───────────────────────────────────────────────────────
	   if lease.State == LeaseStateRevoked {
		  return b.rejectIntent(ctx, intent, now, lease, RejectCodeLeaseRevoked,
			   "lease revoked before intent submission"), nil
	  }
	  if lease.State == LeaseStateExpired || !now.Before(lease.ExpiresAt) {
		   return b.rejectIntent(ctx, intent, now, lease, RejectCodeLeaseExpired,
			  "lease expired before intent submission"), nil
	   }
	  if lease.State == LeaseStateBound {
		  // 已绑定意味着此窗口已有胜者，本意图必然冲突。
		   return b.rejectIntentWithProof(ctx, intent, now, lease,
			  RejectCodeCASConflict, "lease already bound to accepted intent"), nil
	   }
	  // ── 4. CAS 检验 ──────────────────────────────────────────────────────────
	   // 世代号必须完全一致；否则说明租约快照已被刷新。
	   if intent.LeaseGen != lease.Gen {
		   return b.rejectIntentWithProof(ctx, intent, now, lease,
			  RejectCodeCASConflict,
			  fmt.Sprintf("lease gen mismatch: intent=%d lease=%d", intent.LeaseGen, lease.Gen)), nil
	  }
	   // 期望可用必须与租约快照一致；允许小数精度容差在 0 以内（精确比较）。
	   if !intent.ExpectedAvail.Equal(lease.Available) {
		   return b.rejectIntentWithProof(ctx, intent, now, lease,
			  RejectCodeCASConflict,
			   fmt.Sprintf("expected_avail mismatch: intent=%s lease=%s",
				   intent.ExpectedAvail.String(), lease.Available.String())), nil
	  }
	   // ── 5. 超提检测 ──────────────────────────────────────────────────────────
	  if intent.Amount.GreaterThan(lease.Available) {
		  metricOverdrawAttempts.Inc()
		  return b.rejectIntentWithProof(ctx, intent, now, lease,
			   RejectCodeInsufficientAvail,
			  fmt.Sprintf("amount %s exceeds available %s",
				  intent.Amount.String(),
				   lease.Available.String())), nil
	  }
	  // ── 6. 接受意图 ───────────────────────────────────────────────────────────
	   decidedAt := now
	  intent.ID = idOrNew(intent.ID)
	  intent.State = IntentStateAccepted
	  intent.ProposedAt = now
	  intent.DecidedAt = &decidedAt
	  if err := b.store.SaveIntent(ctx, intent); err != nil {
		  return nil, fmt.Errorf("availcas barrier: save accepted intent: %w", err)
	  }
	   // 绑定租约（open → bound），防止同窗口第二个意图通过。
	   if err := b.lm.Bind(ctx, lease.ID, intent.ID); err != nil {
		   // 绑定失败说明并发竞争，视为 CAS 冲突。
		  return b.rejectIntentWithProof(ctx,
			   intent, now, lease,
			  RejectCodeCASConflict, "concurrent bind failed: "+err.Error()), nil
	  }
	  availAfter := lease.Available.Sub(intent.Amount)
	  metricIntentsAccepted.Inc()
	  return &BarrierDecision{
		   Accepted:   true,
		   Intent:     intent,
		   AvailAfter: availAfter,
	   }, nil
   }

// GetDecision 查询已有意图裁决。

   func (b *barrierImpl) GetDecision(ctx context.Context, intentID string) (*BarrierDecision, error) {
	   intent,
		   err := b.store.GetIntent(ctx, intentID)
	  if err != nil {
		   return nil, err
	  }
	   var proof *RaceProof
	   if intent.RaceProofID != "" {
		   proof, _ = b.store.GetProof(ctx, intent.RaceProofID)
	  }
	   return buildDecision(intent, proof), nil
   }

// GetProof 查询竞态证明。
  func (b *barrierImpl) GetProof(ctx context.Context, proofID string) (*RaceProof, error) {
	   return b.store.GetProof(ctx, proofID)
   }

// ListProofs 列出竞态证明。

  func (b *barrierImpl) ListProofs(ctx context.Context, userID, asset string, limit int) ([]*RaceProof, error) {
	  if limit <= 0 || limit > 500 {
		   limit = 50
	   }
	  return b.store.ListProofs(ctx, userID, asset, limit)
  }

// ─── 内部辅助 ────────────────────────────────────────────────────────────────
// rejectIntent 拒绝意图（无竞态证明，用于过期/撤销场景）。

  func (b *barrierImpl) rejectIntent(
	   ctx context.Context,
	   intent *DebitIntent,
	   now time.Time,
	   _ *AvailLease,
	  code RejectCode,
	  note string,
  ) *BarrierDecision {
	  decidedAt := now
	   intent.ID = idOrNew(intent.ID)
	  intent.State = IntentStateRejected
	   intent.RejectCode = code
	  intent.RejectNote = note
	   intent.ProposedAt = now
	   intent.DecidedAt = &decidedAt
	   _ = b.store.SaveIntent(ctx, intent)
	  metricIntentsRejected.WithLabelValues(string(code)).Inc()
	   return buildDecision(intent, nil)
  }

// rejectIntentWithProof 拒绝意图并尝试写入竞态证明。
// 找不到胜者 intent 时写无 winner 的证明（单意图证明，用于超提场景）。
   func (b *barrierImpl) rejectIntentWithProof(
	  ctx context.Context,
	  loser *DebitIntent,
	   now time.Time,
	  lease *AvailLease,
	   code RejectCode,
	   note string,
  ) *BarrierDecision {
	  decidedAt := now
	   loser.ID = idOrNew(loser.ID)
	  loser.State = IntentStateRejected
	  loser.RejectCode = code
	  loser.RejectNote = note
	   loser.ProposedAt = now
	   loser.DecidedAt = &decidedAt
	   // 尝试查询同租约的已接受意图（胜者）。
	   winnerID := ""
	  winnerAmount := decimal.Zero
	   winner, err := b.store.FindAcceptedIntentByLease(ctx, lease.ID)
	  if err == nil && winner != nil {
		  winnerID = winner.ID
		  winnerAmount = winner.Amount
	   }
	   proof := &RaceProof{
		  ID: uuid.NewString(),
		   UserID: loser.UserID,
		   Asset:  loser.Asset,
		   Book:           loser.Book,
		   LeaseID:        lease.ID,
		   WinnerIntentID: winnerID,
		   LoserIntentID: loser.ID,
		  WinnerAmount:    winnerAmount,
		  LoserAmount:     loser.Amount,
		  AvailAtDecision: lease.Available,
		   WindowStart: lease.CreatedAt,
		  WindowEnd: now,
		   RejectCode: code,
		   Note:      note,
		   CreatedAt: now,
	  }
	  loser.RaceProofID = proof.ID
	   _ = b.store.SaveIntent(ctx, loser)
	   _ = b.store.SaveProof(ctx, proof)
	   metricIntentsRejected.WithLabelValues(string(loser.RejectCode)).Inc()
	   metricRaceProofsCreated.Inc()
	  return buildDecision(loser,
		  proof)
  }

// buildDecision 从已有意图和可选 proof 构造 BarrierDecision。

  func buildDecision(intent *DebitIntent, proof *RaceProof) *BarrierDecision {
	  return &BarrierDecision{
		   Accepted: intent.State == IntentStateAccepted,
		  Intent:   intent,
		   Proof:    proof,
	  }
  }

// validateIntent 验证意图基本字段合法性。
   func validateIntent(intent *DebitIntent) error {
	  if intent == nil {
		  return fmt.Errorf("availcas: nil intent")
	  }
	   if strings.TrimSpace(intent.UserID) == "" {
		  return ErrUserRequired
	  }
	   if strings.TrimSpace(intent.Asset) == "" {
		  return ErrAssetRequired
	   }
	   if strings.TrimSpace(intent.Book) == "" {
		   return ErrBookRequired
	  }
	  if !intent.Amount.IsPositive() {
		  return ErrAmountNonPositive
	  }
	   if strings.TrimSpace(intent.LeaseID) == "" {
		  return fmt.Errorf("availcas: lease_id required")
	   }
	   if intent.Kind != IntentKindDebit && intent.Kind != IntentKindFreeze {
		   return fmt.Errorf("availcas: intent kind must be %q or %q, got %q",
			  IntentKindDebit, IntentKindFreeze, intent.Kind)
	  }
	  return nil
   }

// idOrNew 若 id 为空则生成 UUID。

  func idOrNew(id string) string {
	   if id != "" {
		  return id
	  }
	   return uuid.NewString()
  }
// temporary guard removed after the production check
