package dustguard

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// AllocateHook 分配地址前的防护检查。
// 由 addrpool.Pool.Allocate 在派生/查询地址后、返回给调用方前调用。

type AllocateHook struct {
	fsm *QuarantineFSM
	detector *Detector
	policy   *PolicyManager
	store    Store
	clock func() time.Time
}

// NewAllocateHook 构造分配钩子。

   func NewAllocateHook(fsm *QuarantineFSM, detector *Detector, policy *PolicyManager, store Store) *AllocateHook {
	   return &AllocateHook{
		   fsm: fsm,
		  detector: detector,
		  policy: policy,
		  store: store,
		   clock: func() time.Time { return time.Now().UTC() },
	  }
   }

// Check 在目标地址确认前执行防护检查。
// 被隔离的地址返回 ErrAddressQuarantined；分值达到隔离阈值时同样拒绝并自动开案。
  func (h *AllocateHook) Check(ctx context.Context, chain, address, userID string) error {
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	   address = strings.TrimSpace(address)
	   // 已有活跃案件直接拒
	  if h.fsm.IsBlocked(ctx, chain, address) {
		   DustAllocateBlocked.WithLabelValues(chain).Inc()
		  return fmt.Errorf("%w: chain=%s addr=%s", ErrAddressQuarantined, chain, address)
	   }
	   // 若检测器认为该地址当前分值达攻击级别，且策略要求自动隔离，则开案并拒绝
	  pol := h.policy.Active(ctx, chain)
	   if !pol.AutoQuarantine {
		   return nil
	   }
	   score,
		  ok := h.detector.Score(ctx, chain, address)
	  if !ok {
		  return nil
	   }
	  if score.Score < pol.ScoreAttack {
		   return nil
	   }
	   // 自动开案（Open 内部已做重复检查，幂等）
	   _, err := h.fsm.Open(ctx, chain, address, userID, "system", score,
		   fmt.Sprintf("auto quarantine: score=%d level=%s", score.Score, score.Level))
	   if err != nil && err != ErrAlreadyQuarantined {
		   // 开案失败时不静默放行；返回防护错误
		  return fmt.Errorf("%w: score=%d (auto open failed: %v)", ErrAddressQuarantined, score.Score, err)
	   }
	   DustAllocateBlocked.WithLabelValues(chain).Inc()
	  return fmt.Errorf("%w: chain=%s addr=%s score=%d", ErrAddressQuarantined, chain, address, score.Score)
  }

// ─── ObserveHook ─────────────────────────────────────────────────────────
// ObserveHook 入金观察路径防护钩子。
// 由 deposit.Service.Observe 在 s.led.Create 前调用。
type ObserveHook struct {
	fsm      *QuarantineFSM
	detector *Detector
	policy *PolicyManager
	store Store
	clock func() time.Time
}

// NewObserveHook 构造观察钩子。
  func NewObserveHook(fsm *QuarantineFSM, detector *Detector, policy *PolicyManager, store Store) *ObserveHook {
	   return &ObserveHook{
		  fsm:      fsm,
		  detector: detector,
		   policy: policy,
		  store: store,
		   clock: func() time.Time { return time.Now().UTC() },
	  }
  }

// ObserveParams 观察路径传入参数。

type ObserveParams struct {
	UserID  string
	Chain   string
	Address string
	TxHash string
	Asset  string
	Amount string
	FromAddr string
	IdemKey string
}

// Check 在贷记前检查目标地址是否受防护拦截。
// 若地址处于活跃隔离或分值超阈，开 GuardTicket 并返回 ErrObserveBlocked。
// 不静默放行：有任何防护命中都必须返回错误，让上层停写。
// 同时把观察记录到检测器的信号窗口（信号采集不影响本次 credit 结果）。

   func (h *ObserveHook) Check(ctx context.Context, p ObserveParams) (*ObserveGuardResult, error) {
	   chain := strings.ToUpper(strings.TrimSpace(p.Chain))
	  address := strings.TrimSpace(p.Address)
	   // 信号采集（异步不阻主流程；但信号 Record 可在 hook 内同步写窗口）
	  sig := DustSignal{
		   Chain:   chain,
		   Address: address,
		   FromAddr: p.FromAddr,
		   Asset:    p.Asset,
		   Amount:   p.Amount,
		  TxHash:   p.TxHash,
		  SignalKind: SignalFlood,
		  SeenAt: h.clock(),
	  }
	  // 非阻塞信号写（失败不影响防护判断结果）
	  _, _ = h.detector.Record(ctx, sig)
	  // 幂等：同 tx_hash 已有 ticket 则复用
	  if existing, ok := h.store.ActiveTicket(ctx, chain, address, p.TxHash); ok {
		  DustCreditBlocked.WithLabelValues(chain).Inc()
		  return &ObserveGuardResult{
			  Blocked:  true,
			  TicketID: existing.ID,
			  CaseID: existing.CaseID,
			   Reason: existing.Reason,
		  }, ErrObserveBlocked
	   }
	  // 检查是否处于活跃隔离
	  activeCase, isBlocked := h.fsm.ActiveCase(ctx, chain, address)
	   if !isBlocked {
		   // 非隔离地址：检查实时分值是否已超阈值
		   pol := h.policy.Active(ctx,
			   chain)
		   score, ok := h.detector.Score(ctx, chain, address)
		   if !ok || score.Score < pol.ScoreAttack {
			  return &ObserveGuardResult{Blocked: false}, nil
		  }
		  // 分值超阈但尚无案件：自动开案
		   c, err := h.fsm.Open(ctx, chain, address, p.UserID, "system", score,
			   fmt.Sprintf("observe block: score=%d", score.Score))
		  if err != nil && err != ErrAlreadyQuarantined {
			  return &ObserveGuardResult{Blocked: false}, nil
		  }
		   if c != nil {
			  activeCase = *c
			  isBlocked = true
		   } else {
			  // open 幂等返回 ErrAlreadyQuarantined：重新查活跃案
			   if ac, ok2 := h.fsm.ActiveCase(ctx, chain, address); ok2 {
				   activeCase = ac
				   isBlocked = true
			  }
		   }
	   }
	   if !isBlocked {
		  return &ObserveGuardResult{Blocked: false}, nil
	   }
	  // 开 GuardTicket
	  now := h.clock()
	  ticket := GuardTicket{
		   ID:    uuid.NewString(),
		   Chain: chain,
		   Address: address,
		  UserID:  p.UserID,
		  TxHash:  p.TxHash,
		   Asset:  p.Asset,
		   Amount: p.Amount,
		  CaseID: activeCase.ID,
		   State: TicketBlocking,
		  Reason: fmt.Sprintf("address quarantined (case=%s level=%s)", activeCase.ID, activeCase.TriggerLevel),
		  CreatedAt: now,
		  UpdatedAt: now,
	  }
	   if err := h.store.CreateTicket(ctx, ticket); err != nil {
		   // 开单失败依然拒绝贷记（fail-closed）
		   DustCreditBlocked.WithLabelValues(chain).Inc()
		   return &ObserveGuardResult{
			   Blocked: true,
			  CaseID: activeCase.ID,
			   Reason: "guard ticket create failed; credit blocked",
		   }, ErrObserveBlocked
	  }
	  audit := newAudit(AuditTicket, ticket.ID, "system", "open", "", string(TicketBlocking), ticket.Reason)
	   _ = h.store.WriteAudit(ctx, audit)
	   DustCreditBlocked.WithLabelValues(chain).Inc()
	   return &ObserveGuardResult{
		   Blocked: true,
		   TicketID: ticket.ID,
		   CaseID:   activeCase.ID,
		  Reason:   ticket.Reason,
	   }, ErrObserveBlocked
   }

// DustGuardChecker 观察路径防护检查接口（ObserveHook 实现此接口）。
// 注意：deposit 包通过 deposit.DustGuardChecker（CheckObserve 签名）注入，
// 由 DepositAdapter 桥接；本接口供包内组合及单元引用。
type DustGuardChecker interface {
	Check(ctx context.Context,
		p ObserveParams) (*ObserveGuardResult, error)
}

// TicketReleaser 防护单释放接口，供运维 API 调用。

type TicketReleaser struct {
	store Store
	clock func() time.Time
}

// NewTicketReleaser 构造防护单释放器。

  func NewTicketReleaser(store Store) *TicketReleaser {
	  return &TicketReleaser{store: store, clock: func() time.Time { return time.Now().UTC() }}
  }

// ReleaseTicket 解除防护单拦截（ops 确认地址已清白后调用）。

  func (tr *TicketReleaser) ReleaseTicket(ctx context.Context, ticketID, actor string) (GuardTicket, error) {
	   t, err := tr.store.GetTicket(ctx, ticketID)
	   if err != nil {
		   return GuardTicket{}, err
	   }
	   if t.State == TicketReleased {
		   return t, nil // 幂等
	  }
	   from := string(t.State)
	  t.State = TicketReleased
	  t.UpdatedAt = tr.clock()
	   if err := tr.store.UpdateTicket(ctx, t); err != nil {
		   return GuardTicket{}, err
	  }
	  audit := newAudit(AuditTicket, ticketID, actor, "release", from, string(TicketReleased), "")
	  _ = tr.store.WriteAudit(ctx, audit)
	   return t, nil
   }

// ListTickets 按条件列出防护单。

   func (tr *TicketReleaser) ListTickets(ctx context.Context, chain, address string, state TicketState) ([]GuardTicket, error) {
	   return tr.store.ListTickets(ctx,
		  chain, address, state)
  }
