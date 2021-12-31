// 功能：- Snapshot 返回当前 WriteGate 配置快照（引擎健康、探针状态、可用 scope 列表）。
// - Snapshot 返回当前 WriteGate 配置快照（引擎健康、探针状态、可用 scope 列表）。
// - AuditEntry 用于记录每次 Check 调用的审计行（供持久化层调用）。
// Snapshot 和 AuditEntry 均为只读操作；不修改 Gate 或 Engine 状态。
// 不变量：AuditEntry 的 Result 字段精确反映 Check 的实际结果。

// 作者：Hale
// 日期：2021.12.29

package writegate

import (
	"time"
	"account-assets.kucoin.net/funding/internal/pricepoison"
)

// GateSnapshot WriteGate 当前状态快照。

type GateSnapshot struct {
	// EngineHealth 底层 PoisonEngine 健康状态。
	EngineHealth pricepoison.HealthCheck `json:"engine_health"`
	// ProbingPairs 当前处于半开探针状态的交易对。
	ProbingPairs map[string]PairProbe `json:"probing_pairs"`
	// KnownScopes 写门支持的作用域列表。
	KnownScopes []string `json:"known_scopes"`
	// SnapshotAt 快照时刻（UTC）。
	SnapshotAt time.Time `json:"snapshot_at"`
}

// Snapshot 返回 WriteGate 状态快照。

   func (g *WriteGate) Snapshot() GateSnapshot {
	  now := time.Now().UTC()
	   snap := GateSnapshot{
		  KnownScopes: []string{
			  string(ScopeCredit),
			   string(ScopeOccupancy),
			  string(ScopeLiquidationTrigger),
		  },
		  SnapshotAt: now,
	   }
	  if g == nil {
		  return snap
	  }
	   if g.engine != nil {
		   snap.EngineHealth = g.engine.Health()
	   }
	   if g.probe != nil {
		  snap.ProbingPairs = g.probe.Snapshot()
	   }
	  return snap
   }

// AuditResult Write Gate 审计结果。
type AuditResult string

const (
	AuditAllowed AuditResult = "allowed"
	AuditBlockedToken AuditResult = "blocked_token"
	AuditBlockedProbe AuditResult = "blocked_probe"
	AuditBlockedScope AuditResult = "blocked_scope"
	AuditBlockedUnknown AuditResult = "blocked_unknown"
)

// AuditEntry 单次 WriteGate.Check 的审计快照。供持久化层插入 aa_write_gate_token。
type AuditEntry struct {
	// Pair 交易对。
	Pair string `json:"pair"`
	// Generation 令牌代次。
	Generation uint64 `json:"generation"`
	// TickDigest tick 内容摘要。
	TickDigest string `json:"tick_digest"`
	// WriteScope 请求的写作用域。
	WriteScope string `json:"write_scope"`
	// TokenScope 令牌授权的作用域。
	TokenScope string `json:"token_scope"`
	// Result 门控结果。
	Result AuditResult `json:"result"`
	// BlockReason 拒绝原因（错误文本；allowed 时为空）。
	BlockReason string `json:"block_reason,omitempty"`
	// ProofDigest PoisonProof 摘要（可选，由调用方构造后填入）。
	ProofDigest string `json:"proof_digest,omitempty"`
	// IssuedAt 令牌签发时刻。
	IssuedAt time.Time `json:"issued_at"`
	// CheckedAt 校验时刻（UTC）。
	CheckedAt time.Time `json:"checked_at"`
}

// CheckAudit 同 Check，但同时返回 AuditEntry（fail-closed 语义不变）。
// 调用方可选择将 AuditEntry 持久化到 aa_write_gate_token。
   func (g *WriteGate) CheckAudit(scope WriteScope, tok pricepoison.GenToken) (AuditEntry, error) {
	   now := time.Now().UTC()
	   entry := AuditEntry{
		   Pair:       tok.Pair,
		   Generation: tok.Generation,
		   TickDigest: tok.TickDigest,
		   WriteScope: string(scope),
		   TokenScope: tok.Scope,
		   IssuedAt:   tok.IssuedAt,
		  CheckedAt: now,
	  }
	  err := g.Check(scope, tok)
	   if err == nil {
		  entry.Result = AuditAllowed
		  return entry, nil
	   }
	  // 从错误类型推断审计结果
	   switch {
	   case isErr(err, ErrProbeLocked):
		  entry.Result = AuditBlockedProbe
	   case isErr(err, ErrTokenInvalid):
		   entry.Result = AuditBlockedToken
	   case isErr(err, ErrScopeMismatch):
		   entry.Result = AuditBlockedScope
	  case isErr(err,
		  ErrUnknownScope):
		   entry.Result = AuditBlockedScope
	   default:
		  entry.Result = AuditBlockedUnknown
	  }
	  entry.BlockReason = err.Error()
	   return entry, err
  }

// isErr 浅层 errors.Is 替代（避免 writegate 引入 errors 包循环）。
   func isErr(err, target error) bool {
	   if err == nil || target == nil {
		  return false
	   }
	   type unwrapper interface{ Unwrap() error }
	   for {
		  if err == target {
			   return true
		   }
		   u, ok := err.(unwrapper)
		   if !ok {
			   return false
		  }
		   err = u.Unwrap()
	   }
  }
