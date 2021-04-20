package dustleak

import (
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
)

// RollbackPlan 描述单次回滚的执行计划（用于运营审阅）。
type RollbackPlan struct {
	LeakID       string `json:"leak_id"`
	ConvertID    string `json:"convert_id"`
	UserID       string `json:"user_id"`
	Asset        string `json:"asset"`
	RollbackUSDT string `json:"rollback_usdt"` // 须逆记的 USDT（泄漏量）
	RollbackAsset string `json:"rollback_asset"` // 若需恢复原始资产余额（OverFloor 场景）
	Note string `json:"note"`
	GeneratedAt time.Time `json:"generated_at"`
}

// PlanRollback 根据 LeakCase 生成 RollbackPlan 供双人审阅。
// 不改变任何状态，纯计算。
   func PlanRollback(lc *LeakCase) (*RollbackPlan, error) {
	   if lc == nil {
		   return nil, ErrNotFound
	   }
	  if lc.Status != LeakStatusDetected {
		  return nil, fmt.Errorf("%w: leak=%s status=%s", ErrState, lc.ID, lc.Status)
	  }
	   plan := &RollbackPlan{
		  LeakID:    lc.ID,
		   ConvertID: lc.ConvertID,
		   UserID:    lc.UserID,
		  Asset:     lc.Asset,
		   RollbackUSDT: lc.LeakUSDT,
		  GeneratedAt: time.Now().UTC(),
	   }
	   switch lc.Kind {
	   case LeakKindOverFloor:
		  // OverFloor：逆记全部 USDT 贷出（兑换不应发生），但源资产已扣除，
		  // 需视情况恢复（此处保守不自动恢复，由运营决策）
		  plan.Note = fmt.Sprintf("over_floor: convert=%s amount=%s usdt=%s floor=%s",
			  lc.ConvertID, lc.ActualAmount, lc.ActualUSDT, lc.FloorAmount)
		   plan.RollbackUSDT = lc.LeakUSDT
	   case LeakKindDriftOverpay:
		  // DriftOverpay：只逆记超额部分 USDT（非全部）
		  plan.Note = fmt.Sprintf("drift_overpay: convert=%s snap_rate=%s actual_usdt=%s leak_usdt=%s",
			   lc.ConvertID,
			  lc.FloorAmount, lc.ActualUSDT, lc.LeakUSDT)
		   plan.RollbackUSDT = lc.LeakUSDT
	   case LeakKindDoubleDust:
		  // DoubleDust：逆记重复兑换的全部 USDT
		  plan.Note = fmt.Sprintf("double_dust: convert=%s amount=%s usdt=%s",
			  lc.ConvertID, lc.ActualAmount, lc.ActualUSDT)
		  plan.RollbackUSDT = lc.LeakUSDT
	   }
	   return plan, nil
  }

// ExecuteRollback 执行回滚腿的账务逆记：将 RollbackStatus 从 pending 推进到 posted。
// 实际的账务操作（USDT 逆记）由 dustconv.Service 或 ledger 执行；
// 本方法更新状态并重新计算证明哈希。
  func (s *Service) ExecuteRollback(rollbackID, actor string) (*LeakRollback, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	  }
	   actor = strings.TrimSpace(actor)
	  if actor == "" {
		   return nil, ErrActorInvalid
	   }
	  s.mu.Lock()
	  rb := s.rollbacks[rollbackID]
	  if rb == nil {
		  s.mu.Unlock()
		   return nil, ErrNotFound
	   }
	  if rb.Status != RollbackStatusPending {
		  cp := rb.Copy()
		  s.mu.Unlock()
		   if rb.Status == RollbackStatusPosted {
			   return &cp, nil // 幂等
		  }
		   return &cp, fmt.Errorf("%w: status=%s", ErrState, rb.Status)
	   }
	   now := s.now()
	   rb.Status = RollbackStatusPosted
	  rb.PostedAt = now
	   rb.UpdatedAt = now
	   rb.ExecutedBy = actor
	  cp := rb.Copy()
	  s.mu.Unlock()
	   if s.store != nil {
		   _ = s.store.SaveRollback(rb)
	  }
	   DustRollbackPosted.WithLabelValues(rb.Asset).Inc()
	  return &cp, nil
   }

// ConserveAssertion 回滚 Conserve 断言：
// 验证「泄漏 USDT（已贷记）+ 逆记 USDT（已借记）= 0」。
// rollbackUSDT 与 leakUSDT 在同精度下应相等。
  func ConserveAssertion(leakUSDT, rollbackUSDT string) error {
	  leak,
		   err := money.Parse(leakUSDT)
	   if err != nil {
		   return ErrAmount
	   }
	   rb, err := money.Parse(rollbackUSDT)
	   if err != nil {
		  return ErrAmount
	  }
	  if !leak.Equal(rb) {
		   return fmt.Errorf("%w: leak_usdt=%s rollback_usdt=%s",
			   ErrConserve, leakUSDT, rollbackUSDT)
	  }
	   return nil
   }

// FailRollback 将回滚腿置 failed，LeakCase 进入 dispute（需人工介入）。
   func (s *Service) FailRollback(rollbackID, actor, reason string) (*LeakRollback, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	   actor = strings.TrimSpace(actor)
	  if actor == "" {
		  return nil, ErrActorInvalid
	  }
	   s.mu.Lock()
	  rb := s.rollbacks[rollbackID]
	   if rb == nil {
		  s.mu.Unlock()
		   return nil, ErrNotFound
	   }
	  if rb.Status == RollbackStatusFailed {
		   cp := rb.Copy()
		  s.mu.Unlock()
		  return &cp, nil // 幂等
	   }
	   now := s.now()
	  rb.Status = RollbackStatusFailed
	   rb.Note = strings.TrimSpace(reason)
	   rb.UpdatedAt = now
	  leakID := rb.LeakID
	  cp := rb.Copy()
	  s.mu.Unlock()
	   if s.store != nil {
		   _ = s.store.SaveRollback(rb)
	   }
	   // LeakCase → dispute
	   _, _ = s.TransitionLeak(leakID, LeakStatusRollingBack, LeakStatusDispute, actor)
	   DustRollbackFailed.WithLabelValues(rb.Asset).Inc()
	  return &cp, nil
  }

// RollbacksByLeak 返回某 LeakCase 的所有回滚腿（通常只有一条）。

   func (s *Service) RollbacksByLeak(leakID string) ([]*LeakRollback, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	   }
	  s.mu.Lock()
	   var out []*LeakRollback
	   for _, r := range s.rollbacks {
		  if r.LeakID == leakID {
			  cp := r.Copy()
			  out = append(out, &cp)
		  }
	   }
	   s.mu.Unlock()
	   return out, nil
   }

// SuspenseEntry 记录一条挂账科目（USDT 逆记凭证），供审计链使用。
// 本包不直接操作账本；由调用方将 SuspenseEntry 传递给 ledger/dustconv。
type SuspenseEntry struct {
	ID string `json:"id"`
	RollbackID string `json:"rollback_id"`
	LeakID     string `json:"leak_id"`
	ConvertID string `json:"convert_id"`
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	SuspenseUSDT string `json:"suspense_usdt"` // 挂账金额
	Direction string `json:"direction"` // "debit" (逆记)
	PostedBy  string `json:"posted_by"`
	PostedAt time.Time `json:"posted_at"`
}

// BuildSuspenseEntry 根据 LeakRollback 构建挂账科目条目。
  func BuildSuspenseEntry(rb *LeakRollback, actor string) *SuspenseEntry {
	  return &SuspenseEntry{
		  RollbackID: rb.ID,
		   LeakID:    rb.LeakID,
		  ConvertID: rb.ConvertID,
		  UserID:       rb.UserID,
		  Asset:        rb.Asset,
		  SuspenseUSDT: rb.RollbackUSDT,
		  Direction: "debit",
		  PostedBy:  actor,
		  PostedAt: time.Now().UTC(),
	   }
  }
