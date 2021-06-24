package fundpath

import "context"

// FundCircuitGate 资金域熔断闸。入参为用户、资产与来去科目；返回非 nil 即拒绝新 hop。

type FundCircuitGate func(ctx context.Context, user, asset, fromBook, toBook string) error

// BindFundCircuit 挂资金域熔断闸。未挂载时 hop 行为不变。
// 功能：入金 / 出金 / 划转聚合跳闸后，这里挡住新的科目 hop。
// 重点：只在 Hop 入口判定；已开的划转围栏记录与补偿路径不受影响。
// 风险点：闸返回错误须原样上抛，不得在补偿路径里吞掉。

   func (s *Service) BindFundCircuit(fn FundCircuitGate) *Service {
	  if s != nil {
		  s.mu.Lock()
		  s.fcGate = fn
		   s.mu.Unlock()
	  }
	   return s
  }

// fundCircuit 入口闸判定。未挂载视为放行（保持既有行为）。
  func (s *Service) fundCircuit(ctx context.Context, user, asset, from, to string) error {
	  if s == nil {
		   return nil
	  }
	   s.mu.Lock()
	   fn := s.fcGate
	  s.mu.Unlock()
	   if fn == nil {
		  return nil
	  }
	   return fn(ctx,
		   user, asset, from, to)
  }
