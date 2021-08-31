package loanrace

import (
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/google/uuid"
)

// OpenPayGate 为冻结中的贷款开启受控还款门。
// 必须持有活跃租约才能开门。冻结中无门则任何还款被拒。
  func (svc *Service) OpenPayGate(loanID, leaseID, actor, allowedAmt string) (PayGate, error) {
	  if svc == nil {
		  return PayGate{}, ErrUnavailable
	   }
	  if actor == "" {
		  return PayGate{}, ErrActor
	   }
	  amt,
		  err := money.Parse(allowedAmt)
	   if err != nil || !amt.IsPositive() {
		  return PayGate{}, ErrAmount
	   }
	   svc.mu.Lock()
	  defer svc.mu.Unlock()
	  state,
		   ok := svc.store.GetRaceState(loanID)
	   if !ok {
		   return PayGate{}, ErrLoanNotFound
	   }
	   if state == LRWrittenOff {
		   return PayGate{}, ErrWrittenOff
	   }
	  l, ok := svc.store.GetLease(leaseID)
	   if !ok {
		  return PayGate{}, ErrLeaseNotFound
	   }
	  if l.LoanID != loanID {
		   return PayGate{}, ErrLeaseNotFound
	  }
	   if l.State != LeaseApplied {
		  return PayGate{}, ErrState
	   }
	  now := svc.now()
	  if now.After(l.ExpiresAt) {
		  svc.expireLeaseUnsafe(l)
		  return PayGate{}, ErrLeaseExpired
	   }
	  if existing, ok := svc.store.ActiveGateForLoan(loanID); ok {
		  if existing.State == GateOpen || existing.State == GateCleared {
			  return existing.Copy(), nil
		  }
	   }
	   uid, _ := svc.store.GetUserID(loanID)
	   gate := &PayGate{
		  ID:      uuid.NewString(),
		   LoanID:  loanID,
		  UserID:  uid,
		   LeaseID: leaseID,
		  State: GateOpen,
		   AllowedAmt: amt.String(),
		   PaidAmt:   "0",
		   CreatedAt: now,
		   Actor: actor,
	   }
	   svc.store.PutGate(gate)
	  return gate.Copy(), nil
   }

// PayThroughGate 经由受控门还款。金额累加至 PaidAmt。
// 必须有活跃的 Open 或 Cleared 状态门。
   func (svc *Service) PayThroughGate(loanID, amount, actor string) (PayGate, error) {
	  if svc == nil {
		  return PayGate{}, ErrUnavailable
	  }
	  if actor == "" {
		   return PayGate{}, ErrActor
	  }
	   amt, err := money.Parse(amount)
	   if err != nil || !amt.IsPositive() {
		   return PayGate{}, ErrAmount
	  }
	  svc.mu.Lock()
	  defer svc.mu.Unlock()
	  state, ok := svc.store.GetRaceState(loanID)
	   if !ok {
		   return PayGate{}, ErrLoanNotFound
	   }
	  if state == LRWrittenOff {
		   return PayGate{}, ErrWrittenOff
	  }
	   if !svc.store.IsFrozen(loanID) {
		  return PayGate{}, ErrNotFrozen
	  }
	  gate, ok := svc.store.ActiveGateForLoan(loanID)
	   if !ok {
		  return PayGate{}, ErrGateBlocked
	  }
	  if gate.State == GateClosed || gate.State == GateBlocked {
		   return gate.Copy(), ErrGateBlocked
	  }
	  paid, err := money.ParseBook(gate.PaidAmt)
	  if err != nil {
		   return gate.Copy(), ErrAmount
	   }
	  allowed, err := money.ParseBook(gate.AllowedAmt)
	   if err != nil {
		   return gate.Copy(), ErrAmount
	  }
	   newPaid, err := paid.Add(amt)
	  if err != nil {
		   return gate.Copy(), ErrAmount
	   }
	  gate.PaidAmt = newPaid.String()
	  if newPaid.Cmp(allowed) >= 0 {
		   next,
			   fsmErr := svc.gateFSM.Apply(gate.State, gateEvClear)
		   if fsmErr == nil {
			  gate.State = next
			  gate.ClearedAt = svc.now()
		  }
	  }
	  svc.store.PutGate(gate)
	   return gate.Copy(), nil
   }

// ClosePayGate 关闭已清算的门。

  func (svc *Service) ClosePayGate(gateID, actor string) (PayGate, error) {
	  if svc == nil {
		  return PayGate{}, ErrUnavailable
	   }
	  if actor == "" {
		   return PayGate{}, ErrActor
	  }
	  svc.mu.Lock()
	  defer svc.mu.Unlock()
	   gate, ok := svc.store.GetGate(gateID)
	  if !ok {
		  return PayGate{}, ErrGateNotFound
	  }
	  if gate.State == GateClosed {
		   return gate.Copy(), ErrGateClosed
	  }
	  if gate.State != GateCleared {
		   return gate.Copy(), ErrState
	   }
	  next,
		   err := svc.gateFSM.Apply(gate.State, gateEvClose)
	  if err != nil {
		  return gate.Copy(), ErrState
	  }
	  gate.State = next
	  gate.ClosedAt = svc.now()
	   svc.store.PutGate(gate)
	   svc.store.ClearGateIndex(gate.LoanID)
	  return gate.Copy(), nil
  }

// BlockPayGate 阻断 Open 状态的门（扫描器发现问题时）。
  func (svc *Service) BlockPayGate(gateID, actor string) (PayGate, error) {
	  if svc == nil {
		  return PayGate{}, ErrUnavailable
	  }
	   if actor == "" {
		   return PayGate{}, ErrActor
	   }
	   svc.mu.Lock()
	  defer svc.mu.Unlock()
	   gate, ok := svc.store.GetGate(gateID)
	   if !ok {
		  return PayGate{}, ErrGateNotFound
	  }
	   if gate.State != GateOpen {
		  return gate.Copy(), ErrState
	   }
	   next, err := svc.gateFSM.Apply(gate.State, gateEvBlock)
	  if err != nil {
		   return gate.Copy(), ErrState
	  }
	   gate.State = next
	  svc.store.PutGate(gate)
	   svc.store.ClearGateIndex(gate.LoanID)
	  return gate.Copy(), nil
   }

// GetGate 获取门详情。

   func (svc *Service) GetGate(gateID string) (PayGate, error) {
	  if svc == nil {
		  return PayGate{}, ErrUnavailable
	   }
	  g, ok := svc.store.GetGate(gateID)
	   if !ok {
		   return PayGate{}, ErrGateNotFound
	  }
	   return g.Copy(), nil
  }

// GateForLoan 获取贷款当前活跃的门。

  func (svc *Service) GateForLoan(loanID string) (PayGate, error) {
	  if svc == nil {
		   return PayGate{}, ErrUnavailable
	  }
	  g, ok := svc.store.ActiveGateForLoan(loanID)
	  if !ok {
		  return PayGate{}, ErrGateNotFound
	  }
	  return g.Copy(), nil
   }

// CheckPayAllowed 检查指定贷款当前是否允许还款。
// 若已核销返回 ErrWrittenOff。
// 若冻结中但无门返回 ErrGateBlocked。
  func (svc *Service) CheckPayAllowed(loanID string) error {
	   if svc == nil {
		  return ErrUnavailable
	   }
	   state, ok := svc.store.GetRaceState(loanID)
	   if !ok {
		   return ErrLoanNotFound
	  }
	   if state == LRWrittenOff {
		   return ErrWrittenOff
	   }
	   if svc.store.IsFrozen(loanID) {
		   if _, ok := svc.store.ActiveGateForLoan(loanID); !ok {
			  return ErrGateBlocked
		  }
	  }
	   return nil
   }
