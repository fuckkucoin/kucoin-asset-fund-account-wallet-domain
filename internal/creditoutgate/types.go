package creditoutgate

import "time"

// BoardGeneration 板卡/车道指纹世代快照。
// 每次 board/card/lane 关闭或翻转，world_gen 单调递增；
// EnqueueCredit 绑定当前快照；DeliverCredit 复验快照是否仍有效。

type BoardGeneration struct {
	// BoardKey 板卡唯一标识（chain|asset 或运营板卡 ID）。
	BoardKey string `json:"board_key"`
	// WorldGen 全局单调世代序号。护栏翻转时+1。
	WorldGen int64 `json:"world_gen"`
	// LaneGen 车道（充/提/清扫）单调世代序号。车道翻转时+1。
	LaneGen int64 `json:"lane_gen"`
	// Closed 护栏/板卡是否已关闭。关闭后不允许新 Enqueue。
	Closed bool `json:"closed"`
	// ClosedAt 关闭时刻；为空表示未关闭。
	ClosedAt *time.Time `json:"closed_at,omitempty"`
	// UpdatedAt 最近一次世代变更时刻。
	UpdatedAt time.Time `json:"updated_at"`
}

// CreditDeliverLease 贷记投递租约。
// EnqueueCredit 成功后创建，绑定当时的 BoardGeneration 快照（world_gen + lane_gen）。
// DeliverCredit 时必须复验：当前板卡世代须与租约绑定的世代一致；
// 若护栏已翻转（world_gen 或 lane_gen 变大）则租约无效，拒绝贷记并落 CreditFenceCase。
// FSM: enqueued → delivering → delivered
// └→ revoked
type CreditDeliverLease struct {
	// ID 租约唯一标识（UUID）。
	ID string `json:"id"`
	// CreditKey fundsafety CreditOutbox.Key（chain|hash|user|asset）。
	CreditKey string `json:"credit_key"`
	// BoardKey 绑定的板卡标识。
	BoardKey string `json:"board_key"`
	// BindWorldGen 绑定时的全局世代。
	BindWorldGen int64 `json:"bind_world_gen"`
	// BindLaneGen 绑定时的车道世代。
	BindLaneGen int64 `json:"bind_lane_gen"`
	// Epoch fundsafety CreditOutbox.Epoch（与 Token 构成围栏）。
	Epoch int64 `json:"epoch"`
	// Token fundsafety CreditOutbox.Token。
	Token string `json:"token"`
	// Amount 贷记金额（冗余校验用，用于同 key 不同金额冲突检测）。
	Amount string `json:"amount"`
	// State 投递状态机。
	State LeaseState `json:"state"`
	// EnqueuedAt 入队时刻。
	EnqueuedAt time.Time `json:"enqueued_at"`
	// DeliveringAt 开始投递时刻；为空表示尚未开始。
	DeliveringAt *time.Time `json:"delivering_at,omitempty"`
	// DeliveredAt 投递完成时刻；为空表示未完成。
	DeliveredAt *time.Time `json:"delivered_at,omitempty"`
	// RevokedAt 吊销时刻；为空表示未吊销。
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	// RevokeReason 吊销原因简述。
	RevokeReason string `json:"revoke_reason,omitempty"`
}

// LeaseState 投递租约 FSM 状态。
type LeaseState string

const (
	// LeaseEnqueued 已入队，等待 Deliver。
	LeaseEnqueued LeaseState = "enqueued"
	// LeaseDelivering 正在执行 Deliver（已通过复验，写入贷记前）。
	LeaseDelivering LeaseState = "delivering"
	// LeaseDelivered 贷记完成。
	LeaseDelivered LeaseState = "delivered"
	// LeaseRevoked 被护栏翻转或双贷记探针吊销。
	LeaseRevoked LeaseState = "revoked"
)

// CreditFenceCaseKind 证明类型。
type CreditFenceCaseKind string

const (
	// CaseFenceClose EnqueueCredit 后护栏关闭，旧 epoch/token 持有者尝试 DeliverCredit 被拒。
	CaseFenceClose CreditFenceCaseKind = "fence_close"
	// CaseDualCredit 同一 credit_key 出现两条贷记（金额可能不同）。
	CaseDualCredit CreditFenceCaseKind = "dual_credit"
	// CaseGenDrift 世代漂移：Deliver 时发现当前板卡世代 > 绑定世代。
	CaseGenDrift CreditFenceCaseKind = "gen_drift"
	// CaseStaleReplay 旧 epoch/token 重放 Deliver（幂等返回或拒绝）。
	CaseStaleReplay CreditFenceCaseKind = "stale_replay"
	// CaseAmountConflict 同 key 不同金额的 Enqueue 冲突。
	CaseAmountConflict CreditFenceCaseKind = "amount_conflict"
)

// CreditFenceCase 贷记围栏事故证明记录。只追加，不修改。
// 表：fund_credit_fence_cases

type CreditFenceCase struct {
	// ID 证明记录唯一标识（UUID）。
	ID string `json:"id"`
	// Kind 证明类型。
	Kind CreditFenceCaseKind `json:"kind"`
	// CreditKey 对应的 CreditOutbox.Key。
	CreditKey string `json:"credit_key"`
	// LeaseID 对应的 CreditDeliverLease.ID（可为空，Enqueue 冲突时无租约）。
	LeaseID string `json:"lease_id,omitempty"`
	// BoardKey 对应的板卡标识。
	BoardKey string `json:"board_key"`
	// BindWorldGen 租约绑定时的全局世代。
	BindWorldGen int64 `json:"bind_world_gen"`
	// CurrentWorldGen 发现事故时的当前全局世代。
	CurrentWorldGen int64 `json:"current_world_gen"`
	// Epoch 触发事故的 epoch。
	Epoch int64 `json:"epoch"`
	// Token 触发事故的 token。
	Token string `json:"token"`
	// ConflictLeaseID 冲突的第二条租约 ID（dual_credit 用）。
	ConflictLeaseID string `json:"conflict_lease_id,omitempty"`
	// Evidence 事故现场摘要（JSON 片段，≤4096 字节）。
	Evidence string `json:"evidence"`
	// DetectedAt 检测时刻。
	DetectedAt time.Time `json:"detected_at"`
	// ExportedAt 已导出至下游对账系统的时刻；为空表示未导出。
	ExportedAt *time.Time `json:"exported_at,omitempty"`
}

// CreditProof 投递证明存档。Deliver 成功后追加一行，证明某次贷记已落账。
// 表：fund_credit_proofs

type CreditProof struct {
	// ID 证明唯一标识（UUID）。
	ID string `json:"id"`
	// CreditKey 对应 CreditOutbox.Key。
	CreditKey string `json:"credit_key"`
	// LeaseID 对应 CreditDeliverLease.ID。
	LeaseID string `json:"lease_id"`
	// BoardKey 板卡标识。
	BoardKey string `json:"board_key"`
	// WorldGen 投递时绑定的全局世代。
	WorldGen int64 `json:"world_gen"`
	// LaneGen 投递时绑定的车道世代。
	LaneGen int64 `json:"lane_gen"`
	// Epoch 投递使用的 epoch。
	Epoch int64 `json:"epoch"`
	// Amount 贷记金额。
	Amount string `json:"amount"`
	// ProvedAt 证明写入时刻。
	ProvedAt time.Time `json:"proved_at"`
	// ExportedAt 已导出的时刻；为空表示未导出。
	ExportedAt *time.Time `json:"exported_at,omitempty"`
}

// EnqueueReq EnqueueCredit 请求参数。
type EnqueueReq struct {
	Chain  string `json:"chain"`
	TxHash string `json:"tx_hash"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
}

// EnqueueResult EnqueueCredit 结果。

type EnqueueResult struct {
	// Lease 新建的投递租约。
	Lease CreditDeliverLease `json:"lease"`
	// BoardGen 绑定时的板卡世代快照。
	BoardGen BoardGeneration `json:"board_gen"`
	// Replay 为 true 表示同 key 同金额的幂等返回（不是新建）。
	Replay bool `json:"replay"`
}

// DeliverReq DeliverCredit 请求参数。
type DeliverReq struct {
	CreditKey string `json:"credit_key"`
	Epoch int64  `json:"epoch"`
	Token string `json:"token"`
}

// DeliverResult DeliverCredit 结果。

type DeliverResult struct {
	// Proof 投递证明（投递成功时非空）。
	Proof *CreditProof `json:"proof,omitempty"`
	// Replay 为 true 表示已 Deliver 过（幂等成功，不二次贷记）。
	Replay bool `json:"replay"`
	// Revoked 为 true 表示租约已被吊销（护栏翻转）。
	Revoked bool `json:"revoked,omitempty"`
}

// FenceFlipReq 护栏/板卡/车道翻转请求。

type FenceFlipReq struct {
	// BoardKey 被翻转的板卡标识。
	BoardKey string `json:"board_key"`
	// LaneFlip 为 true 时同时抬升 lane_gen（车道翻转）。
	LaneFlip bool `json:"lane_flip"`
	// Close 为 true 时标记板卡为关闭状态（阻止新 Enqueue）。
	Close bool `json:"close"`
	// Operator 操作者标识（审计）。
	Operator string `json:"operator"`
}

// FenceFlipResult 护栏翻转结果。

type FenceFlipResult struct {
	// NewGen 翻转后的板卡世代。
	NewGen BoardGeneration `json:"new_gen"`
	// RevokedCount 本次翻转吊销的在途租约数量。
	RevokedCount int `json:"revoked_count"`
	// Cases 本次吊销产生的事故证明。
	Cases []CreditFenceCase `json:"cases,omitempty"`
}

// PatrolResult 双贷记巡检结果。
type PatrolResult struct {
	// Scanned 扫描的已完成投递数量。
	Scanned int `json:"scanned"`
	// Detected 发现的双贷记冲突数量。
	Detected int `json:"detected"`
	// Cases 发现的事故证明。
	Cases []CreditFenceCase `json:"cases,omitempty"`
}

// ProofExport 证明导出摘要（用于对账系统消费）。
type ProofExport struct {
	// Proofs 已投递的证明列表（按 ProvedAt 升序）。
	Proofs []CreditProof `json:"proofs"`
	// Cases 已检测的事故证明（按 DetectedAt 升序）。
	Cases []CreditFenceCase `json:"cases"`
	// ExportedAt 导出时刻。
	ExportedAt time.Time `json:"exported_at"`
	// TotalProofs 证明总数。
	TotalProofs int `json:"total_proofs"`
	// TotalCases 事故证明总数。
	TotalCases int `json:"total_cases"`
}

// GenView 只读世代视图，供其他包只读消费（credit_deliver_gen 接口）。
type GenView struct {
	// BoardKey 板卡标识。
	BoardKey string `json:"board_key"`
	// WorldGen 当前全局世代。
	WorldGen int64 `json:"world_gen"`
	// LaneGen 当前车道世代。
	LaneGen int64 `json:"lane_gen"`
	// Closed 是否已关闭。
	Closed bool `json:"closed"`
	// AsOf 快照时刻。
	AsOf time.Time `json:"as_of"`
}
