package coolmarkgate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// ──────────────────────────────────────────────────────────────────────────────
// 世代标记
// ──────────────────────────────────────────────────────────────────────────────
// CoolGeneration 归集冷却世代标记。
// 功能：每次 Mark(asset) 递增世代号；Exec 路径须携带匹配的世代才能放行。
// 重点：
// GenID 以 asset 散列构造，跨节点相同 asset 得到相同 GenID 键。
// Generation 从 1 开始，单调递增，不可逆归零。
// ActiveLeaseID 指向当前生效的 CoolLease，空串表示无活跃冷却。

type CoolGeneration struct {
	GenID      string `json:"gen_id"`
	Asset      string `json:"asset"`
	Generation int64  `json:"generation"`
	ActiveLeaseID string `json:"active_lease_id"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GenIDFor 从 asset 构造世代标记主键，确保跨节点幂等。

   func GenIDFor(asset string) string {
	  raw := fmt.Sprintf("coolmarkgate:gen:%s", strings.ToUpper(strings.TrimSpace(asset)))
	   sum := sha256.Sum256([]byte(raw))
	   return hex.EncodeToString(sum[:16])
   }

// ──────────────────────────────────────────────────────────────────────────────
// 冷却租约
// ──────────────────────────────────────────────────────────────────────────────
// CoolLeaseState 冷却租约状态机节点。
type CoolLeaseState string

const (
	// CoolMarked 已触发冷却 Mark；冷却窗口开始计时，Exec 必须拒绝。
	CoolMarked CoolLeaseState = "marked"
	// CoolBlocking 冷却窗口活跃中；同义于 CoolMarked，用于更细粒度监控。
	CoolBlocking CoolLeaseState = "blocking"
	// CoolCleared 冷却窗口已结束，且已签发 CoolProof；Exec 持新世代可放行。
	CoolCleared CoolLeaseState = "cleared"
)

// IsTerminal 判断租约是否已到达不可再流转的终态。
  func (s CoolLeaseState) IsTerminal() bool {
	   return s == CoolCleared
  }

// CoolDefaultWindow 默认冷却窗口时长：与 sweeppol.Rule.Cooldown 默认值一致。

const CoolDefaultWindow = time.Hour

// CoolLease 冷却租约。
// 功能：绑定资产冷却时段，持世代号与 CoolUntil 截止时刻；
// Exec 路径须在 CoolUntil 之后、持正确 Generation 才可放行。
// 重点：
// LeaseID 全局唯一，用于关联 CoolBreachCase 和 CoolProof。
// Generation 与 CoolGeneration.Generation 对应；不匹配则拒绝。
// CoolUntil 为冷却截止时刻（UTC），到达后须由 worker 或调用方触发 Clear。

type CoolLease struct {
	LeaseID string `json:"lease_id"`
	Asset   string `json:"asset"`
	Generation int64          `json:"generation"`
	State      CoolLeaseState `json:"state"`
	MarkedAt   time.Time      `json:"marked_at"`
	CoolUntil time.Time  `json:"cool_until"`
	ClearedAt *time.Time `json:"cleared_at,omitempty"`
	ProofID string `json:"proof_id,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// IsActive 以给定时钟判断冷却窗口是否仍然活跃（未到 CoolUntil）。
  func (l *CoolLease) IsActive(now time.Time) bool {
	   if l == nil {
		   return false
	   }
	   return l.State != CoolCleared && now.Before(l.CoolUntil)
  }

// ──────────────────────────────────────────────────────────────────────────────
// 违约案例
// ──────────────────────────────────────────────────────────────────────────────
// BreachDecision 违约处置决策。

type BreachDecision string

const (
	// BreachBlocked 执行请求已在冷却窗内被拦截，归集未发出。
	BreachBlocked BreachDecision = "blocked"
	// BreachDrifted 检测到冷却窗内存在已发出的归集，属于漂移事件。
	BreachDrifted BreachDecision = "drifted"
)

// CoolBreachCase 冷却窗违约案例。
// 功能：冷却窗内 Exec/MarkSent 被拦截时，或 Stale Plan Detector 发现漂移时落库；
// 供审计、告警、人工处置追溯。
// 重点：
// CaseID 全局唯一。
// LeaseID 关联当时的 CoolLease。
// Generation 记录违约时世代，方便与 CoolGeneration 。
// Decision = "blocked" 表示主动拦截；"drifted" 表示后验发现。

type CoolBreachCase struct {
	CaseID     string `json:"case_id"`
	Asset      string `json:"asset"`
	Generation int64  `json:"generation"`
	LeaseID string `json:"lease_id"`
	ExecID string `json:"exec_id,omitempty"`
	AttemptedAt time.Time      `json:"attempted_at"`
	Decision    BreachDecision `json:"decision"`
	Reason      string         `json:"reason"`
	Resolved bool `json:"resolved"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}

// ──────────────────────────────────────────────────────────────────────────────
// 冷却证明
// ──────────────────────────────────────────────────────────────────────────────
// CoolProof 冷却证明。
// 功能：冷却窗结束后，引擎签发证明并落库；后续 Exec 须核验证明存在且世代匹配。
// 重点：
// ProofID 全局唯一，对应一条 CoolLease 的 cleared 事件。
// Generation 须与调用方携带的 cool_gen 一致，否则拒绝。
// SignedAt 为证明签发时刻（UTC）。
// 缺失证明（ProofID 为空或查无结果）等同于 Exec 路径禁止放行。
type CoolProof struct {
	ProofID string `json:"proof_id"`
	LeaseID string `json:"lease_id"`
	Asset      string `json:"asset"`
	Generation int64  `json:"generation"`
	CoolUntil time.Time `json:"cool_until"`
	SignedAt time.Time `json:"signed_at"`
	Digest   string    `json:"digest"`
}

// ComputeDigest 计算证明摘要：对 asset+generation+cool_until+signed_at 取 SHA-256 前 32 hex 字符。
   func (p *CoolProof) ComputeDigest() string {
	   if p == nil {
		   return ""
	   }
	  raw := fmt.Sprintf("coolproof:%s:%d:%s:%s",
		   strings.ToUpper(p.Asset),
		   p.Generation,
		  p.CoolUntil.UTC().Format(time.RFC3339Nano),
		  p.SignedAt.UTC().Format(time.RFC3339Nano),
	   )
	  sum := sha256.Sum256([]byte(raw))
	   return hex.EncodeToString(sum[:16])
  }

// ──────────────────────────────────────────────────────────────────────────────
// 输入/输出结构
// ──────────────────────────────────────────────────────────────────────────────
// MarkInput Mark 操作入参。
type MarkInput struct {
	// Asset 归集资产代码（大写），如 "USDT"、"BTC"。
	Asset string
	// Reason 触发冷却的原因，用于审计追溯。
	Reason string
	// Window 冷却窗口时长；为 0 时使用 CoolDefaultWindow。
	Window time.Duration
}

// MarkResult Mark 操作输出。
type MarkResult struct {
	// Lease 新签发或当前生效的 CoolLease。
	Lease *CoolLease
	// Generation 本次 Mark 后的世代号。
	Generation int64
	// Reused 为 true 表示租约复用（相同资产冷却窗仍活跃，无需再创建）。
	Reused bool
}

// ExecVerifyInput Exec 路径复验入参。
type ExecVerifyInput struct {
	// Asset 归集资产代码。
	Asset string
	// CoolGen 调用方携带的世代号；必须与当前 CoolGeneration.Generation 匹配。
	CoolGen int64
	// ExecID 归集执行标识，用于落 BreachCase。
	ExecID string
}

// ExecVerifyResult Exec 复验输出。
type ExecVerifyResult struct {
	// Allowed 为 true 表示放行。
	Allowed bool
	// ProofID 放行时关联的 CoolProof 标识。
	ProofID string
	// Generation 当前世代号（供调用方）。
	Generation int64
	// BreachCaseID 拒绝时落库的 CoolBreachCase 标识（Allowed=false 时有效）。
	BreachCaseID string
	// Reason 拒绝原因（Allowed=false 时有效）。
	Reason string
}
