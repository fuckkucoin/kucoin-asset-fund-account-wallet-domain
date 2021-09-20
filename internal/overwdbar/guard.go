package overwdbar

import (
	"context"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/availcas"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Guard 超提拦截器外观接口。
type Guard interface {
	// PreCheck 预检阶段：以 leaseID 验证当前可用余额可覆盖 amount。
	// 若通过，返回 dry-run decision（未绑定租约，不消费 lease）；
	// 若超提，返回 ErrOverdrawBlocked 并发出 VarianceEvent。
	PreCheck(ctx context.Context,
		req PreCheckRequest) (*availcas.BarrierDecision, error)
	// Commit 提交阶段：以 leaseID 提交 binding CAS 意图。
	// 必须先调用 PreCheck 通过后再调用 Commit。
	// 若 CAS 冲突或超提，返回 ErrOverdrawBlocked 并发出 VarianceEvent。
	Commit(ctx context.Context, req CommitRequest) (*availcas.BarrierDecision, error)
}

// PreCheckRequest 预检请求。
type PreCheckRequest struct {
	// UserID 用户标识。
	UserID string
	// Asset 资产代码。
	Asset string
	// Book 账本位。
	Book string
	// LeaseID 已持有的租约 ID（由调用方预先通过 availcas.IssueLease 获取）。
	LeaseID string
	// Amount 预检扣减额。
	Amount decimal.Decimal
	// ActorID 发起方标识。
	ActorID string
	// IsLiveHash 是否为 live-hash 冻结场景。
	IsLiveHash bool
}

// CommitRequest 提交请求（与 PreCheckRequest 对应）。

type CommitRequest struct {
	// UserID 用户标识。
	UserID string
	// Asset 资产代码。
	Asset string
	// Book 账本位。
	Book string
	// LeaseID 与 PreCheck 同一租约 ID。
	LeaseID string
	// LeaseGen 租约世代（来自 PreCheck 返回的 AvailLease.Gen）。
	LeaseGen int64
	// ExpectedAvail 租约快照可用余额（来自 PreCheck 返回的 AvailLease.Available）。
	ExpectedAvail decimal.Decimal
	// Amount 实际扣减额（应与 PreCheck.Amount 一致）。
	Amount decimal.Decimal
	// Kind 操作类型：debit 或 freeze。
	Kind string
	// Reason 业务语义说明。
	Reason string
	// IdemKey 幂等键（出金 ID 或划转 ID 派生）。
	IdemKey string
	// ActorID 发起方标识。
	ActorID string
	// RequestIP 来源 IP（审计）。
	RequestIP string
	// IsLiveHash 是否为 live-hash 冻结场景。
	IsLiveHash bool
}

// guardImpl OverdrawGuard 实现。

type guardImpl struct {
	svc *availcas.Service
	emitter VarianceEmitter
	clock func() time.Time
}

// NewGuard 构造 OverdrawGuard。
// emitter 若为 nil，使用空实现（不发事件）。
   func NewGuard(svc *availcas.Service, emitter VarianceEmitter) Guard {
	  if emitter == nil {
		  emitter = noopEmitter{}
	  }
	  return &guardImpl{
		  svc:     svc,
		  emitter: emitter,
		   clock: func() time.Time { return time.Now().UTC() },
	   }
   }

// newGuardWithClock 供注入时钟。
   func newGuardWithClock(svc *availcas.Service, emitter VarianceEmitter, clock func() time.Time) *guardImpl {
	   if emitter == nil {
		   emitter = noopEmitter{}
	   }
	  return &guardImpl{svc: svc, emitter: emitter, clock: clock}
   }

  // PreCheck 预检阶段实现。
   // 以 dry-run 方式检验：加载租约快照，验证 Amount ≤ Available。
  // 不消费租约（不绑定）；真正绑定在 Commit 阶段。
  func (g *guardImpl) PreCheck(ctx context.Context, req PreCheckRequest) (*availcas.BarrierDecision, error) {
	   if err := validatePreCheck(req); err != nil {
		   return nil, err
	   }
	  // 加载租约，获取当前可用快照。
	  lease, err := g.svc.GetLease(ctx, req.LeaseID)
	   if err != nil {
		  return nil, fmt.Errorf("overwdbar precheck: get lease: %w", err)
	   }
	  now := g.clock()
	  if !lease.Usable(now) {
		   // 租约不可用：发出差异事件（非超提，属于租约失效）。
		  evt := g.buildVarianceEvent(req.UserID, req.Asset, req.Book,
			   req.Amount, lease.Available, GatePreCheck,
			  availcas.RejectCodeLeaseExpired, "", "", req.ActorID, req.IsLiveHash, now)
		  _ = g.emitter.EmitOverdrawVariance(evt)
		  return nil, ErrOverdrawBlocked
	   }
	   // 干运行检验：Amount > Available 则超提拒绝。
	   if req.Amount.GreaterThan(lease.Available) {
		  shortfall := req.Amount.Sub(lease.Available)
		  evt := g.buildVarianceEvent(req.UserID,
			   req.Asset, req.Book,
			  req.Amount, lease.Available, GatePreCheck,
			  availcas.RejectCodeInsufficientAvail, "", "", req.ActorID, req.IsLiveHash, now)
		   evt.Shortfall = shortfall
		   _ = g.emitter.EmitOverdrawVariance(evt)
		   return &availcas.BarrierDecision{
			  Accepted: false,
			  Intent: &availcas.DebitIntent{
				  UserID: req.UserID,
				  Asset:  req.Asset,
				  Book:   req.Book,
				   Amount: req.Amount,
				   State:  availcas.IntentStateRejected,
				   RejectCode: availcas.RejectCodeInsufficientAvail,
				   RejectNote: fmt.Sprintf("precheck: amount %s > available %s", req.Amount, lease.Available),
				   LeaseID:    req.LeaseID,
				  LeaseGen:   lease.Gen,
				  ExpectedAvail: lease.Available,
				  ActorID: req.ActorID,
			  },
		  }, ErrOverdrawBlocked
	   }
	  // 预检通过：返回 dry-run decision（未绑定租约）。
	  return &availcas.BarrierDecision{
		   Accepted: true,
		   AvailAfter: lease.Available.Sub(req.Amount),
		  Intent: &availcas.DebitIntent{
			  UserID: req.UserID,
			  Asset: req.Asset,
			   Book:  req.Book,
			   Amount: req.Amount,
			  State:         availcas.IntentStateProposed,
			  LeaseID:       req.LeaseID,
			  LeaseGen:      lease.Gen,
			   ExpectedAvail: lease.Available,
			   ActorID: req.ActorID,
		   },
	   }, nil
   }

// Commit 提交阶段实现。
// 以 binding CAS 提交真正意图；一旦 accepted，租约被绑定（open → bound）。
  func (g *guardImpl) Commit(ctx context.Context, req CommitRequest) (*availcas.BarrierDecision, error) {
	  if err := validateCommit(req); err != nil {
		   return nil, err
	   }
	  now := g.clock()
	   idemKey := req.IdemKey
	  if idemKey == "" {
		   idemKey = uuid.NewString()
	  }
	   intent := &availcas.DebitIntent{
		  IdemKey: idemKey,
		  UserID:  req.UserID,
		  Asset:   req.Asset,
		   Book:   req.Book,
		  Amount: req.Amount,
		   Kind: req.Kind,
		  Reason:   req.Reason,
		   LeaseID:  req.LeaseID,
		  LeaseGen: req.LeaseGen,
		   ExpectedAvail: req.ExpectedAvail,
		  ActorID: req.ActorID,
		   RequestIP:  req.RequestIP,
		   ProposedAt: now,
	  }
	  decision, err := g.svc.Submit(ctx, intent)
	  if err != nil {
		   return nil, fmt.Errorf("overwdbar commit: barrier submit: %w", err)
	   }
	   if !decision.Accepted {
		  // 提交阶段被屏障拒绝：发出差异事件，绝不自动解冻 live-hash。
		   proofID := ""
		   if decision.Proof != nil {
			   proofID = decision.Proof.ID
		   }
		   intentID := ""
		  if decision.Intent != nil {
			   intentID = decision.Intent.ID
		   }
		  code := availcas.RejectCodeCASConflict
		   if decision.Intent != nil {
			   code = decision.Intent.RejectCode
		   }
		  evt := g.buildVarianceEvent(req.UserID, req.Asset, req.Book,
			   req.Amount, req.ExpectedAvail, GateCommit,
			  code, intentID, proofID, req.ActorID, req.IsLiveHash, now)
		  _ = g.emitter.EmitOverdrawVariance(evt)
		   return decision,
			  ErrOverdrawBlocked
	  }
	  return decision, nil
  }

// ─── 内部辅助 ────────────────────────────────────────────────────────────────

  func (g *guardImpl) buildVarianceEvent(
	  userID, asset, book string,
	  amount, avail decimal.Decimal,
	  gate string,
	   code availcas.RejectCode,
	  intentID, proofID, actorID string,
	  isLiveHash bool,
	  now time.Time,
  ) *OverdrawVarianceEvent {
	  shortfall := amount.Sub(avail)
	  if shortfall.IsNegative() {
		  shortfall = decimal.Zero
	  }
	  return &OverdrawVarianceEvent{
		  EventID: uuid.NewString(),
		   UserID: userID,
		   Asset:               asset,
		   Book:                book,
		   RequestedAmount:     amount,
		  AvailableAtDecision: avail,
		  Shortfall: shortfall,
		   RejectCode:  code,
		  IntentID:    intentID,
		  RaceProofID: proofID,
		   Gate: gate,
		  ActorID:    actorID,
		   IsLiveHash: isLiveHash,
		   OccurredAt: now,
	   }
  }

  func validatePreCheck(req PreCheckRequest) error {
	   if strings.TrimSpace(req.UserID) == "" {
		  return ErrOverdrawBlocked
	  }
	   if strings.TrimSpace(req.Asset) == "" {
		   return ErrOverdrawBlocked
	  }
	   if strings.TrimSpace(req.LeaseID) == "" {
		  return ErrLeaseRequired
	  }
	   if !req.Amount.IsPositive() {
		  return ErrAmountRequired
	  }
	   return nil
  }

   func validateCommit(req CommitRequest) error {
	  if strings.TrimSpace(req.UserID) == "" {
		   return ErrOverdrawBlocked
	   }
	  if strings.TrimSpace(req.LeaseID) == "" {
		   return ErrLeaseRequired
	   }
	   if !req.Amount.IsPositive() {
		  return ErrAmountRequired
	   }
	  if req.Kind != availcas.IntentKindDebit && req.Kind != availcas.IntentKindFreeze {
		  return fmt.Errorf("overwdbar: kind must be debit or freeze, got %q", req.Kind)
	  }
	  return nil
  }
