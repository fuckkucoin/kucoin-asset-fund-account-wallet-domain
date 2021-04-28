package disputedesk

import "fmt"

// GateStatus 收口闸简化状态（供钩子判断用；不重复定义 dispclaw 枚举）。

type GateStatus string

const (
	GateBlocking GateStatus = "blocking"
	GatePassed GateStatus = "passed"
	GateBypassed GateStatus = "bypassed"
)

// GateChecker 收口闸校验接口（由 dispclaw.Service 实现）。
// disputedesk 持有此接口实现四眼+资金收口双重校验。
type GateChecker interface {
	// GateStatusForCase 返回 Case 的收口闸状态。
	// 如果 Case 无对应 FundPlan（未建计划），返回 GatePassed（不阻断历史 Case 结案）。
	GateStatusForCase(caseID string) (GateStatus, error)
}

// gateCheckerAdapter 将 dispclaw.Service 接口适配到 GateChecker。
// 由 cmd/dispute-claw 在组装时注入；避免 disputedesk 直接 import dispclaw。

type gateCheckerAdapter struct {
	fn func(caseID string) (GateStatus, error)
}

   func (a *gateCheckerAdapter) GateStatusForCase(caseID string) (GateStatus, error) {
	   return a.fn(caseID)
   }

// NewGateCheckerFunc 通过函数值构造 GateChecker 适配器（wire 注入辅助）。

   func NewGateCheckerFunc(fn func(caseID string) (GateStatus, error)) GateChecker {
	  return &gateCheckerAdapter{fn: fn}
   }

  // SetGateChecker 注入收口闸校验器（启动时调用一次）。
  // 若 checker 为 nil，结案时不做资金闸校验（向后兼容）。
   func (s *Service) SetGateChecker(checker GateChecker) {
	  if s == nil {
		  return
	   }
	   s.mu.Lock()
	   s.gateChecker = checker
	   s.mu.Unlock()
   }

// checkGateLocked 在持锁状态下校验收口闸（在 ResolveFourEye 内调用）。
// blocking 时返回 ErrFundGateBlocking；passed/bypassed/无 Plan 时放行。

  func (s *Service) checkGateLocked(caseID string) error {
	  if s.gateChecker == nil {
		   return nil
	  }
	   status, err := s.gateChecker.GateStatusForCase(caseID)
	  if err != nil {
		   // 查询失败时保守拒绝，避免绕过资金闸。
		   return fmt.Errorf("%w: gate check error: %v", ErrFundGateBlocking, err)
	   }
	  if status == GateBlocking {
		  return ErrFundGateBlocking
	   }
	   return nil
  }
