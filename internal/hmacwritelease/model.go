package hmacwritelease

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// ──────────────────────────────────────────────────────────────────────────────
// Halt 世代标记
// ──────────────────────────────────────────────────────────────────────────────
// HaltGeneration 记录 HMAC 围栏 Halt 翻转世代。
// 功能：每次 SetHalt(true) 须递增 Generation；WriteCheck 时须验证
// 租约携带的 HaltGen 与当前 HaltGeneration.Generation 一致。
// 重点：
// GenID 全局唯一固定键（整个服务实例只有一条 HaltGeneration 记录）。
// Generation 从 0 开始；0 表示从未 Halt 过，租约有效性仅依赖 TTL。
// Halted 布尔值记录当前是否处于 Halt 状态。

type HaltGeneration struct {
	GenID string `json:"gen_id"`
	Generation int64 `json:"generation"`
	Halted    bool      `json:"halted"`
	UpdatedAt time.Time `json:"updated_at"`
}

// HaltGenGlobalID 全局唯一 HaltGeneration 主键。

const HaltGenGlobalID = "hmacwritelease:haltgen:global"

// ──────────────────────────────────────────────────────────────────────────────
// Nonce 持久防重放
// ──────────────────────────────────────────────────────────────────────────────
// HmacNonceSeen 持久化 nonce 防重放记录。
// 功能：Verify 成功后将 key_id+nonce 写入持久存储；
// 跨节点、跨重启均可防止同一 nonce 重放。
// 重点：
// SeenID 由 key_id + nonce 组合散列构造，幂等唯一。
// SeenAt 记录首次验签成功时刻，供 nonce 窗清理 worker 使用。

type HmacNonceSeen struct {
	SeenID  string `json:"seen_id"`
	KeyID   string `json:"key_id"`
	Nonce   string `json:"nonce"`
	LeaseID string `json:"lease_id"`
	SeenAt time.Time `json:"seen_at"`
	ExpireAt time.Time `json:"expire_at"`
}

// NoneSeenIDFor 由 key_id + nonce 构造持久防重放主键，跨节点幂等。
  func NoneSeenIDFor(keyID, nonce string) string {
	  raw := fmt.Sprintf("hmacwritelease:nonce:%s:%s",
		   strings.TrimSpace(keyID), strings.TrimSpace(nonce))
	   sum := sha256.Sum256([]byte(raw))
	   return hex.EncodeToString(sum[:16])
   }

// ──────────────────────────────────────────────────────────────────────────────
// 写路径租约
// ──────────────────────────────────────────────────────────────────────────────
// LeaseState HMAC 写路径租约状态机节点。

type LeaseState string

const (
	// LeaseIssued 已签发，等待资金写提交携带并绑定。
	LeaseIssued LeaseState = "issued"
	// LeaseBound 已被一次资金写提交绑定（WriteCheck 通过，尚未 consumed）。
	LeaseBound LeaseState = "bound"
	// LeaseConsumed 资金写提交已提交到数据库，租约正常消费终态。
	LeaseConsumed LeaseState = "consumed"
	// LeaseExpired TTL 到期，未被任何写提交使用；由 worker 流转到此态。
	LeaseExpired LeaseState = "expired"
	// LeaseRevoked Halt 翻转或密钥吊销后批量置为此态；旧 halt_gen 租约立即失效。
	LeaseRevoked LeaseState = "revoked"
)

// IsTerminal 判断租约是否已到达不可再流转的终态。

  func (s LeaseState) IsTerminal() bool {
	  switch s {
	   case LeaseConsumed, LeaseExpired, LeaseRevoked:
		   return true
	   }
	   return false
   }

// DefaultLeaseTTL 租约默认有效期：验签后资金写提交须在此窗口内完成。
const DefaultLeaseTTL = 5 * time.Minute

// HmacWriteLease HMAC 验签写路径租约。
// 功能：Verify 成功后签发；资金写提交（冻结/划转/贷记）必须携带未过期且
// 通过 WriteCheck 的租约才能进入写成功路径。
// 重点：
// LeaseID 全局唯一，供他包只读引用。
// KeyID + Nonce 构成验签身份指纹，防止租约伪造或混用。
// MethodPath 为 method+"::"+path 的组合指纹，防止跨接口租约复用。
// HaltGen 记录签发时的 HaltGeneration.Generation；
// WriteCheck 时必须核验与当前世代一致，Halt 翻转后旧租约立即失效。
// ExpireAt 为租约失效截止时刻（UTC）。
type HmacWriteLease struct {
	LeaseID string `json:"lease_id"`
	KeyID string `json:"key_id"`
	Nonce string `json:"nonce"`
	TS         int64  `json:"ts"`
	MethodPath string `json:"method_path"`
	HaltGen int64      `json:"halt_gen"`
	State   LeaseState `json:"state"`
	IssuedAt time.Time `json:"issued_at"`
	ExpireAt time.Time `json:"expire_at"`
	BoundAt *time.Time `json:"bound_at,omitempty"`
	ConsumedAt *time.Time `json:"consumed_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	RevokeNote string     `json:"revoke_note,omitempty"`
}

// IsValid 以给定时钟判断租约是否处于可用状态（issued 或 bound，且未过期）。

   func (l *HmacWriteLease) IsValid(now time.Time) bool {
	   if l == nil {
		  return false
	  }
	   if l.State != LeaseIssued && l.State != LeaseBound {
		   return false
	   }
	   return now.Before(l.ExpireAt)
  }

// MethodPathFor 构造 method/path 指纹，用于租约绑定与复验。
   func MethodPathFor(method, path string) string {
	  return strings.ToUpper(strings.TrimSpace(method)) + "::" + strings.TrimSpace(path)
  }

// ──────────────────────────────────────────────────────────────────────────────
// 漂移证明
// ──────────────────────────────────────────────────────────────────────────────
// DriftReason 漂移证明产生原因。
type DriftReason string

const (
	// DriftNoLease 资金写提交未携带租约（fail-closed）。
	DriftNoLease DriftReason = "no_lease"
	// DriftExpired 携带的租约已过期。
	DriftExpired DriftReason = "lease_expired"
	// DriftRevoked 携带的租约已被吊销（Halt 翻转或密钥吊销）。
	DriftRevoked DriftReason = "lease_revoked"
	// DriftHaltGenMismatch Halt 世代不匹配：租约签发后发生了 Halt 翻转。
	DriftHaltGenMismatch DriftReason = "halt_gen_mismatch"
	// DriftNonceReplay nonce 重放：该 nonce 已被使用，不得再发租约。
	DriftNonceReplay DriftReason = "nonce_replay"
	// DriftKeyRevoked 密钥已从 key ring 吊销，签发时 key_id 合法但复验时已无效。
	DriftKeyRevoked DriftReason = "key_revoked"
)

// HmacWriteDriftProof HMAC 写路径漂移证明。
// 功能：WriteCheck 拒绝时（无租约/过期/吊销/Halt 世代不符）落库；
// 供审计追溯；禁止删改。
// 重点：
// ProofID 全局唯一。
// LeaseID 可为空（DriftNoLease 场景下无租约）。
// Reason 枚举须与 DriftReason 一致。
// AttemptedAt 记录拒绝时刻（UTC）。

type HmacWriteDriftProof struct {
	ProofID string `json:"proof_id"`
	LeaseID string `json:"lease_id,omitempty"`
	KeyID string `json:"key_id,omitempty"`
	Nonce string `json:"nonce,omitempty"`
	WriteScope string      `json:"write_scope"`
	Reason     DriftReason `json:"reason"`
	Detail     string      `json:"detail"`
	AttemptedAt time.Time `json:"attempted_at"`
	HaltGen     int64     `json:"halt_gen"`
	Digest string `json:"digest"`
}

// ComputeDigest 计算漂移证明摘要。
   func (p *HmacWriteDriftProof) ComputeDigest() string {
	   if p == nil {
		   return ""
	   }
	   raw := fmt.Sprintf("driftproof:%s:%s:%s:%s:%s",
		   p.ProofID,
		   p.LeaseID,
		   string(p.Reason),
		   p.WriteScope,
		  p.AttemptedAt.UTC().Format(time.RFC3339Nano),
	  )
	  sum := sha256.Sum256([]byte(raw))
	   return hex.EncodeToString(sum[:16])
  }

// ──────────────────────────────────────────────────────────────────────────────
// 输入/输出结构
// ──────────────────────────────────────────────────────────────────────────────
// VerifyAndLeaseInput VerifyAndLease 操作入参。

type VerifyAndLeaseInput struct {
	// KeyID 验签密钥 ID。
	KeyID string
	// Secret 验签密钥明文（由调用方从 KeyRing 取得后传入）。
	Secret string
	// TS 请求时间戳（Unix 秒字符串）。
	TS string
	// Nonce 请求随机串，须唯一。
	Nonce string
	// Method HTTP 方法（大写）。
	Method string
	// Path 请求路径。
	Path string
	// Sig HMAC-SHA256 签名 hex 字符串。
	Sig string
	// Body 请求体原始字节。
	Body []byte
	// TTL 租约有效期；为 0 时使用 DefaultLeaseTTL。
	TTL time.Duration
}

// VerifyAndLeaseResult VerifyAndLease 操作输出。

type VerifyAndLeaseResult struct {
	// Lease 新签发的 HmacWriteLease。
	Lease *HmacWriteLease
	// HaltGen 签发时的 Halt 世代号。
	HaltGen int64
}

// WriteCheckInput WriteCheck 操作入参。
type WriteCheckInput struct {
	// LeaseID 资金写提交携带的租约 ID。
	LeaseID string
	// WriteScope 写操作标识（如 "withdraw.freeze" / "transfer" / "credit"）。
	WriteScope string
}

// WriteCheckResult WriteCheck 操作输出。
type WriteCheckResult struct {
	// Allowed 为 true 时允许资金写入。
	Allowed bool
	// Lease 通过时关联的 HmacWriteLease（Allowed=true 时有效）。
	Lease *HmacWriteLease
	// DriftProofID 拒绝时落库的 HmacWriteDriftProof 标识（Allowed=false 时有效）。
	DriftProofID string
	// Reason 拒绝原因（Allowed=false 时有效）。
	Reason DriftReason
	// Detail 补充说明。
	Detail string
}
