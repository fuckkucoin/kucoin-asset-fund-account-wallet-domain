package utamodegate

import "time"

// ─────────────────────────────────────────────────────────────────────────────
// MarginMode 枚举
// ─────────────────────────────────────────────────────────────────────────────
// MarginMode 与 utabook 中的 MarginMode 对应，不引入 import cycle。

type MarginMode string

const (
	MarginModeUnified MarginMode = "UNIFIED"
	MarginModeClassic MarginMode = "CLASSIC"
	MarginModeCross   MarginMode = "CROSS"
	MarginModeIsolated MarginMode = "ISOLATED"
)

// ─────────────────────────────────────────────────────────────────────────────
// ModeGeneration
// ─────────────────────────────────────────────────────────────────────────────
// ModeGeneration 记录用户 UTA 模式世代 CAS 计数器。
// CAS 协议：
// Acquire（模式切换）:
// UPDATE uta_mode_generations
// SET generation = generation + 1,
// current_mode = ?,
// updated_at = NOW(3)
// WHERE user_id = ? AND generation = ?
// -- 影响行数 = 0 → ErrStaleGen；= 1 → 成功，新世代 = old+1
// Insert-first（首次）:
// INSERT INTO uta_mode_generations (id, user_id, generation, current_mode, updated_at)
// VALUES (?, ?, 1, ?, NOW(3))
// ON CONFLICT (user_id) DO NOTHING
// -- 若冲突则对方已 INSERT；重试 SELECT + UPDATE
type ModeGeneration struct {
	ID string `json:"id"`
	UserID string `json:"user_id"`
	Generation int64 `json:"generation"`
	CurrentMode MarginMode `json:"current_mode"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ─────────────────────────────────────────────────────────────────────────────
// LeaseStatus 枚举
// ─────────────────────────────────────────────────────────────────────────────
// LeaseStatus 描述 ModeLease 在状态机中的位置。

type LeaseStatus string

const (
	LeaseOpen     LeaseStatus = "open"     // 申请已发起，世代已递增，绑定等待确认
	LeaseBound    LeaseStatus = "bound"    // 绑定已确认，旧世代令牌已作废
	LeaseSwitched LeaseStatus = "switched" // 切换完成终态
	LeaseBlocked LeaseStatus = "blocked" // 在途负债/抵押互斥，进入待切换队列
)

// ─────────────────────────────────────────────────────────────────────────────
// ModeLease
// ─────────────────────────────────────────────────────────────────────────────
// ModeLease 记录一次 UTA 模式切换的租约生命周期。
// 状态机转移：
// Acquire: → open（CAS 世代递增 + 在途检查通过）
// → blocked（存在 open LiabilityBind，入待切换队列）
// Bind: open → bound（旧世代 LiabilityBind 均已 released 或 CAS 通过）
// Switch: bound → switched（worker 确认无在途负债后推进）
// Unblock: blocked → open（worker 扫描：在途清零后重新激活）
// TriggerRef 形态：{userID}|mode|{fromMode}→{toMode}|{unixMillis}

type ModeLease struct {
	ID string `json:"id"`
	UserID   string     `json:"user_id"`
	FromMode MarginMode `json:"from_mode"`
	ToMode   MarginMode `json:"to_mode"`
	Generation int64 `json:"generation"` // 本次切换递增后的新世代号
	TriggerKind string      `json:"trigger_kind"`
	TriggerRef  string      `json:"trigger_ref"`
	Status      LeaseStatus `json:"status"`
	Note        string      `json:"note,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// IsTerminal 判断租约是否处于终态（switched）。

  func (l *ModeLease) IsTerminal() bool {
	  return l != nil && l.Status == LeaseSwitched
  }

// ─────────────────────────────────────────────────────────────────────────────
// BindKind 枚举
// ─────────────────────────────────────────────────────────────────────────────
// BindKind 描述 LiabilityBind 绑定的操作类型。

type BindKind string

const (
	BindKindBorrow BindKind = "borrow" // 借出操作
	BindKindRepay  BindKind = "repay"  // 还款操作
	BindKindCollateral BindKind = "collateral" // 抵押占用
	BindKindRelease    BindKind = "release"    // 抵押释放
)

// BindStatus 描述 LiabilityBind 的状态。
type BindStatus string

const (
	BindOpen BindStatus = "open" // 在途中，尚未结清
	BindReleased BindStatus = "released" // 已还款/释放，可安全切模
	BindExpired BindStatus = "expired" // TTL 过期（异常场景，触发漂移告警）
)

// ─────────────────────────────────────────────────────────────────────────────
// LiabilityBind
// ─────────────────────────────────────────────────────────────────────────────
// LiabilityBind 将一次借出/抵押/还款操作与生效的模式世代绑定。
// 借出或抵押占用时创建（BindOpen）；还款或抵押释放时推进至 BindReleased。
// 模式切换前扫描所有 BindOpen 记录：若存在则拒绝 SetMode 或进入待切换队列。
// Currency + MarginMode 构成负债键，对应 utabook 的分键语义。
// 旧 MarginMode（切换前）的 BindOpen 若在新 MarginMode 读面聚合，产出 ModeDriftCase。

type LiabilityBind struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	BindKind   BindKind   `json:"bind_kind"`
	Currency   string     `json:"currency"`
	MarginMode MarginMode `json:"margin_mode"` // 操作时生效的模式键
	Amount string `json:"amount"`
	Generation int64      `json:"generation"` // 操作时生效的世代号
	RefID      string     `json:"ref_id"`     // 关联的借还/抵押订单 ID
	Status     BindStatus `json:"status"`
	ExpiresAt  time.Time  `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ─────────────────────────────────────────────────────────────────────────────
// DriftKind 枚举
// ─────────────────────────────────────────────────────────────────────────────
// DriftKind 描述检测到的漂移类型。
type DriftKind string

const (
	// DriftKindLiabilityKey 负债按旧模式键记账，读面按新模式聚合，导致漂移。
	DriftKindLiabilityKey DriftKind = "liability_key"
	// DriftKindCollateralKey 抵押品键漂移（旧模式折扣适用于新模式读面）。
	DriftKindCollateralKey DriftKind = "collateral_key"
	// DriftKindReverseAggregate 新模式负债按旧模式读面汇总（反向漂移）。
	DriftKindReverseAggregate DriftKind = "reverse_aggregate"
	// DriftKindStaleGenWrite 旧世代令牌触发的写操作（CAS 拒绝后仍漂移，紧急标记）。
	DriftKindStaleGenWrite DriftKind = "stale_gen_write"
)

// ─────────────────────────────────────────────────────────────────────────────
// DriftCaseStatus 枚举
// ─────────────────────────────────────────────────────────────────────────────
// DriftCaseStatus 描述 ModeDriftCase 的状态。

type DriftCaseStatus string

const (
	DriftOpen DriftCaseStatus = "open" // 漂移已发现，等待证明
	DriftProven DriftCaseStatus = "proven" // 证明已附，等待四眼收口
	DriftClosed DriftCaseStatus = "closed" // 已四眼收口，漂移已消解
)

// ─────────────────────────────────────────────────────────────────────────────
// ModeDriftCase
// ─────────────────────────────────────────────────────────────────────────────
// ModeDriftCase 记录一次 UTA 模式漂移事件的完整生命周期。
// 状态机转移：
// Open: 发现漂移 → open（DriftDetector.Open）
// AttachProof: open → proven（DriftProofVault.Build + DriftDetector.AttachProof）
// ConfirmClose: proven + 两名不同操作员确认 → closed（四眼收口）
// 存在 open/proven DriftCase 时：
// - 同用户后续 SetMode 被拒绝（ErrDriftBlocked）
// - 高风险借出（大额/跨模式键）被拒绝（ErrDriftBlocked）

type ModeDriftCase struct {
	ID              string     `json:"id"`
	UserID          string     `json:"user_id"`
	Asset           string     `json:"asset"` // 漂移涉及的币种
	Kind            DriftKind  `json:"kind"`
	OldMode         MarginMode `json:"old_mode"`         // 切换前模式
	NewMode         MarginMode `json:"new_mode"`         // 切换后模式
	OldGeneration   int64      `json:"old_generation"`   // 漂移时的旧世代号
	NewGeneration   int64      `json:"new_generation"`   // 漂移时的新世代号
	LiabilityAmount string     `json:"liability_amount"` // 漂移负债量
	BindID string `json:"bind_id,omitempty"` // 关联的 LiabilityBind
	LeaseID string `json:"lease_id,omitempty"` // 关联的 ModeLease
	ProofID string          `json:"proof_id,omitempty"` // 附加的证明 ID
	Status  DriftCaseStatus `json:"status"`
	Confirm1By string     `json:"confirm1_by,omitempty"` // 第一个四眼操作员
	Confirm2By string     `json:"confirm2_by,omitempty"` // 第二个四眼操作员
	Confirm1At *time.Time `json:"confirm1_at,omitempty"`
	Confirm2At *time.Time `json:"confirm2_at,omitempty"`
	Note       string     `json:"note,omitempty"`
	DetectedAt time.Time `json:"detected_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// IsBlocking 判断漂移案是否处于阻塞状态（open 或 proven，均未收口）。
  func (dc *ModeDriftCase) IsBlocking() bool {
	  return dc != nil && (dc.Status == DriftOpen || dc.Status == DriftProven)
  }

// ─────────────────────────────────────────────────────────────────────────────
// ModeDriftProof
// ─────────────────────────────────────────────────────────────────────────────
// ModeDriftProof 是 ModeDriftCase 的附属证明记录。
// 证明包含：
// - 旧模式键下的负债快照（OldKeySnapshot）
// - 新模式聚合面的负债读值（NewAggSnapshot）
// - 差值摘要（DeltaDigest：sha256(OldKeySnapshot+NewAggSnapshot+CaseID)）
// 验证流程：重计算 sha256，与存储的 DeltaDigest ；不一致则 ErrProofDigestMismatch。
type ModeDriftProof struct {
	ID string `json:"id"`
	DriftCaseID    string `json:"drift_case_id"`
	OldKeySnapshot string `json:"old_key_snapshot"` // JSON 序列化的旧键负债快照
	NewAggSnapshot string `json:"new_agg_snapshot"` // JSON 序列化的新聚合读面
	DeltaDigest    string `json:"delta_digest"`     // sha256 hex 摘要
	BuiltBy string `json:"built_by"`
	VerifiedAt time.Time `json:"verified_at"`
	CreatedAt  time.Time `json:"created_at"`
}

// ─────────────────────────────────────────────────────────────────────────────
// 请求/入参类型
// ─────────────────────────────────────────────────────────────────────────────
// AcquireLeaseReq 模式切换租约申请请求。

type AcquireLeaseReq struct {
	UserID   string     `json:"user_id"`
	FromMode MarginMode `json:"from_mode"`
	ToMode   MarginMode `json:"to_mode"`
	ExpectedGen int64  `json:"expected_gen"` // CAS 前置条件
	TriggerKind string `json:"trigger_kind"`
	TriggerRef  string `json:"trigger_ref"`
}

// RegisterBindReq 注册借还/抵押操作的世代绑定请求。
type RegisterBindReq struct {
	UserID string `json:"user_id"`
	BindKind BindKind `json:"bind_kind"`
	Currency string `json:"currency"`
	MarginMode MarginMode `json:"margin_mode"`
	Amount     string     `json:"amount"`
	RefID      string     `json:"ref_id"`
	TTLSeconds int `json:"ttl_seconds"` // 0 表示使用默认 TTL（1 小时）
}

// ReleaseBindReq 释放借还/抵押操作的世代绑定请求。
type ReleaseBindReq struct {
	UserID string `json:"user_id"`
	BindID string `json:"bind_id"`
	RefID string `json:"ref_id"` // 通过 ref_id 定位绑定（bind_id 为空时使用）
}

// OpenDriftCaseReq 开立漂移案请求。

type OpenDriftCaseReq struct {
	UserID          string     `json:"user_id"`
	Asset           string     `json:"asset"`
	Kind            DriftKind  `json:"kind"`
	OldMode         MarginMode `json:"old_mode"`
	NewMode         MarginMode `json:"new_mode"`
	OldGeneration   int64      `json:"old_generation"`
	NewGeneration   int64      `json:"new_generation"`
	LiabilityAmount string     `json:"liability_amount"`
	BindID  string `json:"bind_id,omitempty"`
	LeaseID string `json:"lease_id,omitempty"`
	Note string `json:"note,omitempty"`
}

// BuildProofReq 构建漂移证明请求。

type BuildProofReq struct {
	DriftCaseID    string `json:"drift_case_id"`
	OldKeySnapshot string `json:"old_key_snapshot"`
	NewAggSnapshot string `json:"new_agg_snapshot"`
	BuiltBy string `json:"built_by"`
}

// ConfirmCloseReq 四眼收口确认请求。
type ConfirmCloseReq struct {
	DriftCaseID string `json:"drift_case_id"`
	Operator    string `json:"operator"`
}
