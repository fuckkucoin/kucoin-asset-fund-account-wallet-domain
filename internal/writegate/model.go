package writegate

import (
	"fmt"
	"time"
)

// ─── CheckScope 写作用域 ──────────────────────────────────────────────────
// ScopeKind 写作用域类型。

type ScopeKind string

const (
	// ScopeKindWithdraw 出金路径写作用域。
	ScopeKindWithdraw ScopeKind = "withdraw"
	// ScopeKindTransfer 内部划转写作用域。
	ScopeKindTransfer ScopeKind = "transfer"
	// ScopeKindSettle 结算写作用域。
	ScopeKindSettle ScopeKind = "settle"
	// ScopeKindRedeem 赎回写作用域。
	ScopeKindRedeem ScopeKind = "redeem"
)

// CheckScope 资金写路径作用域描述。
// 不变量：
// - Kind 必须为已知 ScopeKind。
// - SubjectID 标识写操作主体（用户 ID / 账户 ID / 批次 ID）。
// - RefID 为调用方传入的外部单据 ID（可为空）。
type CheckScope struct {
	// Kind 作用域类型。
	Kind ScopeKind
	// SubjectID 操作主体 ID（用户 ID / 账户 ID）。
	SubjectID string
	// RefID 关联外部单据 ID（可为空）。
	RefID string
	// Chain 目标链（出金场景必填；划转可为空）。
	Chain string
	// Asset 资产代码（如 BTC、USDT）。
	Asset string
}

// Validate 校验作用域合法性。

   func (s CheckScope) Validate() error {
	   switch s.Kind {
	  case ScopeKindWithdraw, ScopeKindTransfer, ScopeKindSettle, ScopeKindRedeem:
	   default:
		  return fmt.Errorf("writegate: unsupported scope kind %q", s.Kind)
	  }
	  if s.SubjectID == "" {
		  return fmt.Errorf("writegate: subject_id required")
	  }
	   return nil
  }

// ScopeKey 唯一键字符串（用于 lease 索引）。

   func (s CheckScope) ScopeKey() string {
	   return fmt.Sprintf("%s|%s|%s|%s", s.Kind, s.SubjectID, s.Chain, s.Asset)
  }

// ─── ProbeMutex 探针互斥状态 ──────────────────────────────────────────────
// ProbeState 探针互斥状态快照。
// 不变量：
// - IsProbing 为 true 时，PairID 非空，指纹 StateFingerprint 记录当前探针开启参数。
// - IsProbing 为 false 时，表示当前无活跃探针挡写。
// - StateFingerprint = SHA-256(PairID + IsProbing + SnapAt)，供联合租约世代。

type ProbeState struct {
	// IsProbing 当前是否处于探针挡写状态。
	IsProbing bool
	// PairID 探针识（非空时 IsProbing=true）。
	PairID string
	// StateFingerprint 探针状态指纹（签发联合租约时固化）。
	StateFingerprint string
	// SnapAt 状态快照时刻。
	SnapAt time.Time
}

// ─── GenToken 令牌世代快照 ────────────────────────────────────────────────
// TokenState 令牌世代状态。
type TokenState string

const (
	// TokenStateActive 令牌有效（未撤令）。
	TokenStateActive TokenState = "active"
	// TokenStateRevoked 令牌已撤令（毒价撤令 / 管理员撤令）。
	TokenStateRevoked TokenState = "revoked"
	// TokenStateExpired 令牌已过期。
	TokenStateExpired TokenState = "expired"
)

// GenTokenSnap 令牌世代快照。
// 不变量：
// - Generation 单调递增；撤令后推进 +1。
// - TokenHash = SHA-256(Generation + RevokeReason + SnapAt)，供联合租约。
// - State=revoked 时，旧世代联合租约必须拒绝放行写提交。
type GenTokenSnap struct {
	// Generation 令牌世代编号（单调递增）。
	Generation int64
	// TokenHash 令牌指纹（签发联合租约时固化）。
	TokenHash string
	// State 令牌状态。
	State TokenState
	// RevokeReason 撤令原因（仅 revoked 状态非空）。
	RevokeReason string
	// SnapAt 快照时刻。
	SnapAt time.Time
}

// IsValid 判断令牌是否有效（未撤令且未过期）。
   func (g GenTokenSnap) IsValid() bool {
	   return g.State == TokenStateActive
   }

// ─── WriteGateSnap Check 原子快照 ─────────────────────────────────────────
// WriteGateSnap writegate.Check 时捕获的原子状态快照。
// 语义：Check 成功后，引擎将 ProbeState + GenTokenSnap 打包为快照，
// 签发 ProbeTokenLease 时将快照指纹写入租约，供后续写提交复验。
// 不变量：
// - SnapFingerprint = SHA-256(ProbeState.StateFingerprint + GenToken.TokenHash + SnapAt)。
// - Allowed 为 false 时，Check 拒绝，不签发租约。
type WriteGateSnap struct {
	// Scope 写作用域。
	Scope CheckScope
	// Probe 探针互斥状态快照。
	Probe ProbeState
	// Token 令牌世代快照。
	Token GenTokenSnap
	// SnapFingerprint 联合快照指纹。
	SnapFingerprint string
	// Allowed Check 是否放行。
	Allowed bool
	// RejectReason 拒绝原因（Allowed=false 时非空）。
	RejectReason string
	// SnapAt 快照时刻。
	SnapAt time.Time
}

// ─── 只读观测接口 ──────────────────────────────────────────────────────────
// ProbeMutexReader 只读查询探针互斥状态（不调用 MarkProbe/ClearProbe）。
// 绑定 pricepoison.PoisonEngine 的 ProbeState 只读端点；Engine 不改写探针状态。

type ProbeMutexReader interface {
	// ReadProbeState 读取当前探针互斥状态快照。
	ReadProbeState(scope CheckScope) (ProbeState, error)
}

// GenTokenReader 只读查询令牌世代快照（不调用 RotateToken/RevokeToken）。
// 绑定 pricepoison.PoisonEngine 的 GenToken 只读端点；Engine 不改写令牌世代。
type GenTokenReader interface {
	// ReadGenToken 读取当前令牌世代快照。
	ReadGenToken(scope CheckScope) (GenTokenSnap, error)
}

// PoisonProofReader 只读查询毒价证明（不调用 PoisonEngine 内核写方法）。

type PoisonProofReader interface {
	// HasActivePoison 判断指定 scope 是否有活跃毒价挡写证明（只读）。
	HasActivePoison(scope CheckScope) (bool,
		error)
}

// ─── 默认内存实现（供注入） ────────────────────────────────────────────
// MemProbeMutexReader 内存探针状态读取器（用）。
type MemProbeMutexReader struct {
	state map[string]ProbeState // scopeKey → ProbeState
}

// NewMemProbeMutexReader 创建内存探针读取器。
   func NewMemProbeMutexReader() *MemProbeMutexReader {
	   return &MemProbeMutexReader{state: map[string]ProbeState{}}
  }

// SetState 设置探针状态（辅助）。

   func (m *MemProbeMutexReader) SetState(scope CheckScope, ps ProbeState) {
	   m.state[scope.ScopeKey()] = ps
  }

// ReadProbeState 读取探针状态。
   func (m *MemProbeMutexReader) ReadProbeState(scope CheckScope) (ProbeState, error) {
	   if ps, ok := m.state[scope.ScopeKey()]; ok {
		  return ps, nil
	   }
	   return ProbeState{IsProbing: false}, nil
  }

// MemGenTokenReader 内存令牌世代读取器（用）。

type MemGenTokenReader struct {
	snaps map[string]GenTokenSnap // scopeKey → GenTokenSnap
}

// NewMemGenTokenReader 创建内存令牌读取器。
  func NewMemGenTokenReader() *MemGenTokenReader {
	   return &MemGenTokenReader{snaps: map[string]GenTokenSnap{}}
  }

// SetSnap 设置令牌世代快照（辅助）。
   func (m *MemGenTokenReader) SetSnap(scope CheckScope, snap GenTokenSnap) {
	  m.snaps[scope.ScopeKey()] = snap
  }

// ReadGenToken 读取令牌世代快照。
  func (m *MemGenTokenReader) ReadGenToken(scope CheckScope) (GenTokenSnap, error) {
	   if snap, ok := m.snaps[scope.ScopeKey()]; ok {
		  return snap, nil
	   }
	  return GenTokenSnap{Generation: 0, State: TokenStateActive}, nil
  }

// MemPoisonProofReader 内存毒价证明读取器（用）。
type MemPoisonProofReader struct {
	active map[string]bool // scopeKey → hasActivePoison
}

// NewMemPoisonProofReader 创建内存毒价证明读取器。

  func NewMemPoisonProofReader() *MemPoisonProofReader {
	  return &MemPoisonProofReader{active: map[string]bool{}}
   }

// SetActive 设置毒价证明活跃状态（辅助）。

  func (m *MemPoisonProofReader) SetActive(scope CheckScope, active bool) {
	   m.active[scope.ScopeKey()] = active
  }

// HasActivePoison 查询是否有活跃毒价挡写证明。
   func (m *MemPoisonProofReader) HasActivePoison(scope CheckScope) (bool, error) {
	   return m.active[scope.ScopeKey()],
		   nil
   }

// ─── 哨兵错误 ─────────────────────────────────────────────────────────────
// ErrPoisonBlock 毒价挡写（活跃 PoisonProof 存在）。

var ErrPoisonBlock = fmt.Errorf("writegate: blocked by active poison proof")

// ErrProbeActive 探针互斥挡写（IsProbing=true 时拒绝写提交）。

var ErrProbeActive = fmt.Errorf("writegate: probe mutex active, write blocked")

// ErrTokenRevoked 令牌已撤令，写路径封锁。
var ErrTokenRevoked = fmt.Errorf("writegate: gen token revoked")

// ErrTokenExpired 令牌已过期。
var ErrTokenExpired = fmt.Errorf("writegate: gen token expired")

// ErrScopeInvalid 作用域非法。

var ErrScopeInvalid = fmt.Errorf("writegate: invalid scope")
