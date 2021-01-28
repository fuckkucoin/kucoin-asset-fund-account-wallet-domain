// 本闸不改观察、确认、贷记的任何跃迁，也不解冻任何在途单；dep: 速度键仍由 usage 维护。

// Author: Cason
// Date: 2021.01.08

package deposit

import "context"

// FundCircuitGate 资金域熔断闸。入参为用户、链、资产；返回非 nil 即拒绝新观察写。
type FundCircuitGate func(ctx context.Context, user, chain, asset string) error

// BindFundCircuit 挂资金域熔断闸。未挂载时入金写路径行为不变。
// 功能：跨入金 / 出金 / 划转的滥用聚合跳闸后，这里挡住新的观察写。
// 重点：只在入口判定；已进账本的单据与在途冻结不受影响。
// 风险点：闸返回错误必须原样上抛，不得降级为放行。
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
