package dustleak

import "time"

// ── 策略世代 ────────────────────────────────────────────────────────────────
// PolicyGen 与 dustconv.PolicyGen 对应，持久化于 dust_policy_gens 表。
// 每次策略变更对应一行；ConvertLock 锁定指定世代以守护在途 Convert。
type PolicyGen struct {
	ID string `json:"id" db:"id"`
	Asset string `json:"asset" db:"asset"`
	Gen   uint64 `json:"gen" db:"gen"`
	Floor string `json:"floor" db:"floor"`
	MinConvert string `json:"min_convert" db:"min_convert"`
	Enabled bool `json:"enabled" db:"enabled"`
	ChangedBy string `json:"changed_by" db:"changed_by"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

   func (g PolicyGen) Copy() PolicyGen { return g }

// ── 牌价快照 ────────────────────────────────────────────────────────────────
// QuoteSnapshot 记录 Convert 启动时快照的牌价，持久化于 quote_snapshots 表。
// SnapKey = userID + "|" + asset + "|" + idemKey，保证 Convert 幂等性。
type QuoteSnapshot struct {
	ID string `json:"id" db:"id"`
	SnapKey     string `json:"snap_key" db:"snap_key"`
	Asset       string `json:"asset" db:"asset"`
	RateUSDT    string `json:"rate_usdt" db:"rate_usdt"`
	PolicyGenID string `json:"policy_gen_id" db:"policy_gen_id"`
	PolicyGen   uint64 `json:"policy_gen" db:"policy_gen"`
	DriftCapBps int    `json:"drift_cap_bps" db:"drift_cap_bps"`
	ConvertID string    `json:"convert_id" db:"convert_id"` // 对应 dustconv.Convert.ID，落账后回填
	SnapAt    time.Time `json:"snap_at" db:"snap_at"`
	UsedAt    time.Time `json:"used_at,omitempty" db:"used_at"`
}

  func (q QuoteSnapshot) Copy() QuoteSnapshot { return q }

// ── LeakCase ────────────────────────────────────────────────────────────────
// LeakKind 泄漏类型。
type LeakKind string

const (
	LeakKindOverFloor    LeakKind = "over_floor"    // 兑出量超过 floor 策略限额
	LeakKindDriftOverpay LeakKind = "drift_overpay" // 牌价上漂导致超额 USDT 兑出
	LeakKindDoubleDust   LeakKind = "double_dust"   // 批量尘兑双花（同用户同资产重复扣减）
)

// LeakStatus 泄漏案例状态机：detected → rolling_back → closed | dispute。
type LeakStatus string

const (
	LeakStatusDetected    LeakStatus = "detected"     // 发现，尚未开始回滚
	LeakStatusRollingBack LeakStatus = "rolling_back" // 回滚进行中
	LeakStatusClosed  LeakStatus = "closed"  // 回滚完成，科目平衡
	LeakStatusDispute LeakStatus = "dispute" // 人工复核，暂停自动回滚
)

// LeakCase 一条泄漏记录；持久化于 dust_leaks 表。
type LeakCase struct {
	ID   string   `json:"id" db:"id"`
	Kind LeakKind `json:"kind" db:"kind"`
	Status LeakStatus `json:"status" db:"status"`
	UserID      string `json:"user_id" db:"user_id"`
	Asset       string `json:"asset" db:"asset"`
	ConvertID   string `json:"convert_id" db:"convert_id"`   // 违规兑换单 ID
	SnapshotID  string `json:"snapshot_id" db:"snapshot_id"` // 对应 QuoteSnapshot.ID
	PolicyGenID string `json:"policy_gen_id" db:"policy_gen_id"`
	// 实际兑出量（违规值）
	ActualAmount string `json:"actual_amount" db:"actual_amount"`
	ActualUSDT   string `json:"actual_usdt" db:"actual_usdt"`
	// 应允许的上限
	FloorAmount string `json:"floor_amount" db:"floor_amount"`
	MaxUSDT     string `json:"max_usdt" db:"max_usdt"`
	// 溢出量 = 实际 - 上限
	LeakAmount string `json:"leak_amount" db:"leak_amount"`
	LeakUSDT   string `json:"leak_usdt" db:"leak_usdt"`
	// 发现与处置
	DetectedBy string `json:"detected_by" db:"detected_by"` // 扫描来源 worker/api
	ClosedBy string `json:"closed_by" db:"closed_by"`
	Note       string    `json:"note" db:"note"`
	DetectedAt time.Time `json:"detected_at" db:"detected_at"`
	UpdatedAt  time.Time `json:"updated_at" db:"updated_at"`
}

  func (lc LeakCase) Copy() LeakCase { return lc }

// ── LeakRollback ────────────────────────────────────────────────────────────
// RollbackStatus 回滚腿状态。

type RollbackStatus string

const (
	RollbackStatusPending RollbackStatus = "pending" // 等待执行
	RollbackStatusPosted  RollbackStatus = "posted"  // 已入账（USDT 逆记）
	RollbackStatusConserved RollbackStatus = "conserved" // Conserve 已验证
	RollbackStatusFailed RollbackStatus = "failed" // 回滚失败，需人工介入
)

// LeakRollback 一条泄漏回滚腿；持久化于 leak_rollbacks 表。
// 回滚逻辑：逆记泄漏 USDT（借记用户 USDT 科目）+ 恢复原始资产 Conserve。

type LeakRollback struct {
	ID     string         `json:"id" db:"id"`
	LeakID string         `json:"leak_id" db:"leak_id"`
	Status RollbackStatus `json:"status" db:"status"`
	UserID string `json:"user_id" db:"user_id"`
	Asset     string `json:"asset" db:"asset"`
	ConvertID string `json:"convert_id" db:"convert_id"`
	// 回滚金额
	RollbackUSDT  string `json:"rollback_usdt" db:"rollback_usdt"`   // 逆记 USDT
	RollbackAsset string `json:"rollback_asset" db:"rollback_asset"` // 恢复资产（如有）
	// 执行信息
	ExecutedBy string `json:"executed_by" db:"executed_by"`
	DualReqID  string `json:"dual_req_id" db:"dual_req_id"`
	ProofHash string    `json:"proof_hash" db:"proof_hash"` // SHA-256(rollback_id + convert_id + amounts)
	Note      string    `json:"note" db:"note"`
	PostedAt  time.Time `json:"posted_at,omitempty" db:"posted_at"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

  func (r LeakRollback) Copy() LeakRollback { return r }

// ── ConvertGate ─────────────────────────────────────────────────────────────
// GateStatus 挡兑闸状态：open → blocked → released。

type GateStatus string

const (
	GateStatusOpen    GateStatus = "open"    // 正常，可兑换
	GateStatusBlocked GateStatus = "blocked" // 被挡，禁止同用户同资产尘兑
	GateStatusReleased GateStatus = "released" // 泄漏收口后解挡
)

// ConvertGate 挡兑记录；持久化于 convert_gates 表。
// 一旦同用户同资产出现 LeakCase，立即置 blocked；LeakCase.Closed 后才 released。
type ConvertGate struct {
	ID     string `json:"id" db:"id"`
	UserID string `json:"user_id" db:"user_id"`
	Asset  string `json:"asset" db:"asset"`
	Status    GateStatus `json:"status" db:"status"`
	LeakID    string     `json:"leak_id" db:"leak_id"` // 触发挡兑的 LeakCase.ID
	BlockedBy string     `json:"blocked_by" db:"blocked_by"`
	BlockedAt  time.Time `json:"blocked_at,omitempty" db:"blocked_at"`
	ReleasedBy string    `json:"released_by" db:"released_by"`
	ReleasedAt time.Time `json:"released_at,omitempty" db:"released_at"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" db:"updated_at"`
}

  func (g ConvertGate) Copy() ConvertGate { return g }

// ── ConvertLock — Convert 瞬间世代锁定记录 ──────────────────────────────────
// ConvertLock 记录一次 Convert 在进入临界区时锁定的 (PolicyGen, QuoteSnapshot) 对。
// 不持久化；进程内内存持有，用于 Convert 执行期间的双重守卫。

type ConvertLock struct {
	Asset string
	PolicyGen  uint64
	Floor      string
	MinConvert string
	SnapRate string
	DriftCapBps int
}

// ── Proof ────────────────────────────────────────────────────────────────────
// RollbackProof LeakRollback 执行证明，可供外部校验。

type RollbackProof struct {
	RollbackID string `json:"rollback_id"`
	LeakID     string `json:"leak_id"`
	ConvertID  string `json:"convert_id"`
	UserID     string `json:"user_id"`
	Asset        string `json:"asset"`
	RollbackUSDT string `json:"rollback_usdt"`
	ProofHash string    `json:"proof_hash"`
	Status    string    `json:"status"`
	PostedAt  time.Time `json:"posted_at"`
}
