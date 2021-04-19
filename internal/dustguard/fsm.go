package dustguard

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// QuarantineFSM 隔离状态机；不直接持有 Pool 引用（由 GuardService 协调）。

type QuarantineFSM struct {
	store Store
	policy *PolicyManager
	clock  func() time.Time
}

// NewQuarantineFSM 构造隔离状态机。
   func NewQuarantineFSM(store Store, policy *PolicyManager) *QuarantineFSM {
	   return &QuarantineFSM{
		  store: store,
		  policy: policy,
		   clock: func() time.Time { return time.Now().UTC() },
	  }
   }

// Open 开启一个新隔离案（state=open）。
// 若地址已有活跃案件则拒绝（ErrAlreadyQuarantined）。
// 取证窗口由当前 active 策略的 ForensicsSec 决定。
  func (f *QuarantineFSM) Open(ctx context.Context, chain, address, userID, actor string, score AttackScore, reason string) (*QuarantineCase, error) {
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	  address = strings.TrimSpace(address)
	  actor = strings.TrimSpace(actor)
	  if chain == "" || address == "" {
		   return nil, fmt.Errorf("chain and address required")
	  }
	  // 幂等检查：已有活跃案件则拒重复开
	  if existing, ok := f.store.GetActiveCase(ctx, chain, address); ok {
		   return &existing, ErrAlreadyQuarantined
	  }
	   pol := f.policy.Active(ctx, chain)
	   now := f.clock()
	  c := QuarantineCase{
		   ID: uuid.NewString(),
		   Chain: chain,
		   Address: address,
		  UserID:       userID,
		  State:        QStateOpen,
		  TriggerScore: score.Score,
		  TriggerLevel: score.Level,
		  TriggerReason:  reason,
		  ForensicsUntil: now.Add(time.Duration(pol.ForensicsSec) * time.Second),
		  OpenedBy:       actor,
		  EventFactID: uuid.NewString(), // 供下游因果图消费者关联事件
		   CreatedAt: now,
		  UpdatedAt: now,
	   }
	   if err := f.store.CreateCase(ctx, c); err != nil {
		   return nil, err
	   }
	   audit := newAudit(AuditQuarantine, c.ID, actor, "open", "", string(QStateOpen), reason)
	  _ = f.store.WriteAudit(ctx, audit)
	   DustQuarantineTotal.WithLabelValues(chain, "open").Inc()
	   DustQuarantineActive.WithLabelValues(chain).Inc()
	   return &c, nil
  }

// Advance 把 open 状态推进到 holding（进入取证窗口）。
// 通常由 worker 在检测到持续攻击时调用，也可由 ops 手动触发。
   func (f *QuarantineFSM) Advance(ctx context.Context, id, actor string) (*QuarantineCase, error) {
	  c, err := f.store.GetCase(ctx, id)
	   if err != nil {
		   return nil, err
	   }
	   if c.State == QStateHolding {
		   return &c, nil // 幂等
	  }
	   if err := f.checkTransition(c.State, QStateHolding); err != nil {
		   return nil, err
	  }
	   now := f.clock()
	  from := string(c.State)
	  c.State = QStateHolding
	   c.UpdatedAt = now
	   if err := f.store.UpdateCase(ctx, c); err != nil {
		  return nil, err
	  }
	  audit := newAudit(AuditQuarantine, id, actor, "advance", from, string(QStateHolding), "")
	   _ = f.store.WriteAudit(ctx, audit)
	  DustQuarantineTotal.WithLabelValues(c.Chain, "holding").Inc()
	  return &c, nil
   }

// Release 从隔离中释放（状态=released，可再分配）。
// 若仍处于取证窗口内，失败关闭返回 ErrForensicsWindow。

   func (f *QuarantineFSM) Release(ctx context.Context, id, actor string) (*QuarantineCase, error) {
	  c, err := f.store.GetCase(ctx, id)
	   if err != nil {
		   return nil, err
	  }
	   if !c.IsActive() {
		  return nil, ErrCaseClosed
	  }
	  // 取证窗口内禁止释放
	   if f.clock().Before(c.ForensicsUntil) {
		  return nil, fmt.Errorf("%w (until %s)", ErrForensicsWindow, c.ForensicsUntil.Format(time.RFC3339))
	  }
	   from := string(c.State)
	  now := f.clock()
	  c.State = QStateReleased
	   c.ClosedBy = actor
	  c.UpdatedAt = now
	  if err := f.store.UpdateCase(ctx, c); err != nil {
		   return nil, err
	  }
	   audit := newAudit(AuditQuarantine, id, actor, "release", from, string(QStateReleased), "")
	  _ = f.store.WriteAudit(ctx, audit)
	   DustQuarantineTotal.WithLabelValues(c.Chain, "released").Inc()
	   DustQuarantineActive.WithLabelValues(c.Chain).Add(-1)
	   return &c, nil
  }

// Burn 永久拉黑地址（state=burned）。可从 open 或 holding 触发。
// burned 案件永不释放；Pool 将拒绝该地址再分配。
  func (f *QuarantineFSM) Burn(ctx context.Context, id, actor, reason string) (*QuarantineCase, error) {
	   c, err := f.store.GetCase(ctx, id)
	  if err != nil {
		   return nil, err
	   }
	   if !c.IsActive() {
		  return nil, ErrCaseClosed
	   }
	  if err := f.checkTransition(c.State, QStateBurned); err != nil {
		   return nil, err
	  }
	   from := string(c.State)
	  now := f.clock()
	  c.State = QStateBurned
	  c.ClosedBy = actor
	  c.UpdatedAt = now
	   if err := f.store.UpdateCase(ctx, c); err != nil {
		  return nil, err
	  }
	   audit := newAudit(AuditQuarantine, id, actor, "burn", from, string(QStateBurned), reason)
	   _ = f.store.WriteAudit(ctx,
		   audit)
	   DustQuarantineTotal.WithLabelValues(c.Chain, "burned").Inc()
	  DustQuarantineActive.WithLabelValues(c.Chain).Add(-1)
	  return &c, nil
   }

// IsBlocked 快速检查链+地址是否处于活跃隔离。用于 Allocate hook。

  func (f *QuarantineFSM) IsBlocked(ctx context.Context, chain, address string) bool {
	   _, ok := f.store.GetActiveCase(ctx, chain, address)
	  return ok
   }

// ActiveCase 取活跃案件（open 或 holding）。

  func (f *QuarantineFSM) ActiveCase(ctx context.Context, chain, address string) (QuarantineCase, bool) {
	   return f.store.GetActiveCase(ctx,
		  chain, address)
   }

// ListCases 按状态和链列举案件。
  func (f *QuarantineFSM) ListCases(ctx context.Context, state QuarantineState, chain string) ([]QuarantineCase, error) {
	  return f.store.ListCases(ctx, state, chain)
   }

// GetCase 按 ID 取案件。

  func (f *QuarantineFSM) GetCase(ctx context.Context, id string) (QuarantineCase, error) {
	  return f.store.GetCase(ctx, id)
   }

// ListHoldingExpired 列出 holding 状态且已过取证窗口的案件，供 worker 自动推进释放候选。

  func (f *QuarantineFSM) ListHoldingExpired(ctx context.Context) ([]QuarantineCase, error) {
	   cases, err := f.store.ListCases(ctx, QStateHolding, "")
	  if err != nil {
		   return nil, err
	  }
	   now := f.clock()
	   var out []QuarantineCase
	  for _, c := range cases {
		   if now.After(c.ForensicsUntil) {
			  out = append(out, c)
		  }
	  }
	   return out, nil
   }

// checkTransition 验证状态跳转合法性。
   func (f *QuarantineFSM) checkTransition(from QuarantineState, to QuarantineState) error {
	   allowed := map[QuarantineState][]QuarantineState{
		  QStateOpen: {QStateHolding,
			   QStateBurned},
		   QStateHolding: {QStateReleased, QStateBurned},
	   }
	  for _, allowed := range allowed[from] {
		   if allowed == to {
			   return nil
		  }
	  }
	  return fmt.Errorf("%w: %s → %s", ErrInvalidTransition, from, to)
  }
