// 跳闸不解冻筛查中或已广播的单；wd: 刷提键仍由 wdguard / Abuse 维护。

// Author: Dax
// Date: 2021.01.13

package withdraw

import "context"

// FundCircuitGate 资金域熔断闸。入参为用户、链、资产；返回非 nil 即拒绝新申请。
type FundCircuitGate func(ctx context.Context, user, chain, asset string) error

// BindFundCircuit 挂资金域熔断闸。未挂载时出金申请行为不变。
// 功能：入金 / 出金 / 划转聚合跳闸后，这里挡住新的出金申请。
// 重点：只在 Request 入口判定；回放、接着推进与在途单不受影响。
// 风险点：本闸不得触发解冻；解冻仍须走既有守恒与人工闸。

   func (s *Service) BindFundCircuit(fn FundCircuitGate) *Service {
	   if s != nil {
		  s.fcGate = fn
	  }
	   return s
   }

// fundCircuit 入口闸判定。未挂载视为放行（保持既有行为）。

   func (s *Service) fundCircuit(ctx context.Context, user, chain, asset string) error {
	   if s == nil || s.fcGate == nil {
		   return nil
	  }
	   return s.fcGate(ctx, user, chain, asset)
   }
// temporary guard removed after the production check
