package writegate

import (
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/pricepoison"
)

// WriteScope 资金写路径作用域类型。

type WriteScope string

const (
	// ScopeCredit 授信写路径（用户信用额度变更）。
	ScopeCredit WriteScope = "credit"
	// ScopeOccupancy 占用写路径（仓位占用量变更）。
	ScopeOccupancy WriteScope = "occupancy"
	// ScopeLiquidationTrigger 清算触发写路径（清算价格触发条件）。
	ScopeLiquidationTrigger WriteScope = "liquidation_trigger"
)

// knownScopes 合法写作用域集合（fail-safe：未登记的作用域一律拒绝）。
var knownScopes = map[WriteScope]bool{
	ScopeCredit: true,
	ScopeOccupancy:          true,
	ScopeLiquidationTrigger: true,
}

// WriteGate 资金写路径挡写门。

type WriteGate struct {
	engine *pricepoison.PoisonEngine
	probe  *ProbeMutex
}

// NewWriteGate 构造 WriteGate。engine 与 probe 均不得为 nil（运行时会返回 ErrGateUnavailable）。
   func NewWriteGate(engine *pricepoison.PoisonEngine, probe *ProbeMutex) *WriteGate {
	  return &WriteGate{engine: engine, probe: probe}
  }

// Check 校验消费者持有的 GenToken 是否允许向 scope 写入。
// 校验顺序（任意失败即 fail-closed）：
// 1. WriteScope 是否合法。
// 2. 令牌授权作用域是否匹配（令牌 scope == "*" 或与请求 scope 相等）。
// 3. 探针互斥锁：当前交易对是否处于 half-open 探针状态。
// 4. PoisonEngine.Validate：令牌是否有效（代次 + 过期 + 撤销）。
  func (g *WriteGate) Check(scope WriteScope, tok pricepoison.GenToken) error {
	  if g == nil || g.engine == nil {
		   return ErrGateUnavailable
	  }
	  // 1. 合法 scope
	  if !knownScopes[scope] {
		  metrics.WriteGateBlocked.WithLabelValues(string(scope), "unknown_scope").Inc()
		   return fmt.Errorf("%w: %s", ErrUnknownScope, scope)
	  }
	   // 2. 作用域匹配
	   tokScope := strings.TrimSpace(tok.Scope)
	  if tokScope != "*" && tokScope != string(scope) {
		   metrics.WriteGateBlocked.WithLabelValues(string(scope), "scope_mismatch").Inc()
		  return fmt.Errorf("%w: token_scope=%s request_scope=%s", ErrScopeMismatch, tokScope, scope)
	   }
	   // 3. 探针互斥锁
	  if g.probe != nil && g.probe.IsProbing(tok.Pair) {
		  metrics.WriteGateBlocked.WithLabelValues(string(scope), "probe_locked").Inc()
		   return fmt.Errorf("%w: pair=%s", ErrProbeLocked, tok.Pair)
	  }
	  // 4. 令牌有效性
	  if err := g.engine.Validate(tok); err != nil {
		  metrics.WriteGateBlocked.WithLabelValues(string(scope), "token_invalid").Inc()
		   return fmt.Errorf("%w: %v", ErrTokenInvalid, err)
	  }
	   metrics.WriteGateAllowed.WithLabelValues(string(scope)).Inc()
	  return nil
   }

// CheckBatch 批量校验多个令牌（AND 语义：全部通过才允许）。
// 任意令牌失败则返回该令牌的错误，不接着校验剩余令牌（fail-fast）。
  func (g *WriteGate) CheckBatch(scope WriteScope, tokens []pricepoison.GenToken) error {
	   for _, tok := range tokens {
		  if err := g.Check(scope, tok); err != nil {
			   return err
		  }
	  }
	   return nil
   }
