// loanrace 包内 service 实现。

// Author: Cason
// Date: 2021.08.30

package loanrace

import (
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/fsm"
)

// Service 借贷逾期冻结与补还核销竞态收口服务。
type Service struct {
	mu sync.Mutex
	store *Store
	clock func() time.Time
	leaseTTL time.Duration
	raceFSM *fsm.Machine[LoanRaceState,
		string]
	leaseFSM *fsm.Machine[LeaseState, string]
	gateFSM *fsm.Machine[GateState, string]
	writeoffFSM *fsm.Machine[WriteoffState, string]
	proofFSM    *fsm.Machine[ProofState,
		string]
}

type Option func(*Service)

   func WithClock(fn func() time.Time) Option {
	   return func(s *Service) { s.clock = fn }
}

   func WithLeaseTTL(d time.Duration) Option {
	   return func(s *Service) { s.leaseTTL = d }
  }

  func New(opts ...Option) *Service {
	   svc := &Service{
		  store: NewStore(),
		   clock: func() time.Time { return time.Now().UTC() },
		  leaseTTL: 30 * time.Second,
		  raceFSM:  LoanRaceFSM(),
		   leaseFSM:    FreezeLeaseFSM(),
		   gateFSM:     PayGateFSM(),
		   writeoffFSM: WriteoffFSM(),
		  proofFSM: UnfreezeProofFSM(),
	   }
	   for _, o := range opts {
		   o(svc)
	   }
	   return svc
   }

  func (svc *Service) now() time.Time {
	   if svc == nil || svc.clock == nil {
		   return time.Now().UTC()
	   }
	   return svc.clock()
   }

// RegisterLoan 注册贷款到竞态管理器。

   func (svc *Service) RegisterLoan(loanID, userID string) {
	  if svc == nil {
		  return
	  }
	   svc.store.RegisterLoan(loanID,
		   userID)
   }

// GetRaceState 获取贷款的竞态状态。

   func (svc *Service) GetRaceState(loanID string) (LoanRaceState, error) {
	  if svc == nil {
		  return "", ErrUnavailable
	   }
	   s, ok := svc.store.GetRaceState(loanID)
	   if !ok {
		   return "", ErrLoanNotFound
	  }
	   return s, nil
   }

// GetView 获取贷款竞态聚合视图。
  func (svc *Service) GetView(loanID string) (LoanRaceView, error) {
	   if svc == nil {
		  return LoanRaceView{}, ErrUnavailable
	   }
	   if _, ok := svc.store.GetRaceState(loanID); !ok {
		   return LoanRaceView{}, ErrLoanNotFound
	   }
	  return svc.store.BuildView(loanID), nil
  }

// FreezeForOverdue 完整的逾期冻结流程：获取租约 → 标记 overdue → 冻结 → 标记 frozen。

   func (svc *Service) FreezeForOverdue(loanID, userID, actor string) (OverdueLease, error) {
	  if svc == nil {
		  return OverdueLease{}, ErrUnavailable
	   }
	   lease, err := svc.AcquireLease(loanID, userID, actor)
	  if err != nil {
		  return OverdueLease{}, err
	   }
	   svc.mu.Lock()
	   state, ok := svc.store.GetRaceState(loanID)
	   if !ok {
		  svc.mu.Unlock()
		  return lease, ErrLoanNotFound
	   }
	   if state == LRActive {
		   next, fsmErr := svc.raceFSM.Apply(state, lrEvOverdueDetected)
		  if fsmErr == nil {
			  svc.store.SetRaceState(loanID, next)
			  state = next
		   }
	  }
	   if state == LROverdue {
		  next, fsmErr := svc.raceFSM.Apply(state, lrEvFreezeApplied)
		  if fsmErr == nil {
			  svc.store.SetRaceState(loanID, next)
		   }
		  svc.store.SetFrozen(loanID, true)
	  }
	   svc.mu.Unlock()
	   applied, err := svc.ApplyLease(lease.ID)
	  if err != nil {
		  return lease, err
	   }
	  return applied, nil
   }

// StartControlledRepay 启动受控还款路径：开门 + 推进 FSM 至 repaying。
  func (svc *Service) StartControlledRepay(loanID, leaseID, actor, amount string) (PayGate, error) {
	   if svc == nil {
		   return PayGate{}, ErrUnavailable
	   }
	  gate, err := svc.OpenPayGate(loanID, leaseID, actor, amount)
	   if err != nil {
		   return PayGate{}, err
	  }
	   svc.mu.Lock()
	  state,
		  _ := svc.store.GetRaceState(loanID)
	   if state == LRFrozen {
		   if next, fsmErr := svc.raceFSM.Apply(state, lrEvRepayStart); fsmErr == nil {
			  svc.store.SetRaceState(loanID, next)
		   }
	  }
	  svc.mu.Unlock()
	   return gate, nil
  }

// CompleteRepay 还款完成：关门。不推进 FSM 到 closed，需等 ClearFrozenWithProof。
  func (svc *Service) CompleteRepay(gateID, actor string) (PayGate, error) {
	   if svc == nil {
		  return PayGate{}, ErrUnavailable
	   }
	   gate, err := svc.ClosePayGate(gateID, actor)
	   if err != nil {
		   return PayGate{}, err
	  }
	   return gate, nil
   }

// FailRepay 还款失败回退：推进 FSM 回 frozen。

  func (svc *Service) FailRepay(loanID, actor string) error {
	  if svc == nil {
		   return ErrUnavailable
	  }
	  if actor == "" {
		  return ErrActor
	   }
	   svc.mu.Lock()
	  defer svc.mu.Unlock()
	   state,
		  ok := svc.store.GetRaceState(loanID)
	  if !ok {
		   return ErrLoanNotFound
	  }
	   if state != LRRepaying {
		   return ErrState
	   }
	   if next, err := svc.raceFSM.Apply(state, lrEvRepayFail); err == nil {
		   svc.store.SetRaceState(loanID, next)
	   }
	   return nil
  }

// AllRegisteredLoans 返回所有已注册贷款 ID（用于扫描）。

  func (svc *Service) AllRegisteredLoans() []string {
	  if svc == nil {
		  return nil
	   }
	  return svc.store.AllRegisteredLoans()
  }

// LoansInState 返回指定状态的贷款列表。
  func (svc *Service) LoansInState(s LoanRaceState) []string {
	   if svc == nil {
		  return nil
	   }
	   return svc.store.LoansInState(s)
  }
