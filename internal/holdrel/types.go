package holdrel

import "time"

// ReleaseReq 强制释放请求。
type ReleaseReq struct {
	TokenID     string `json:"token_id"`
	CallerLayer string `json:"caller_layer"` // Layer.Code，调用方层
	ReasonCode string `json:"reason_code"` // 释放原因码
	ApprovalRef string `json:"approval_ref,omitempty"` // 审批单 ID（legal/aml 层必填）
	IdemKey string `json:"idem_key"` // 幂等键；相同 idem_key 返回同一 proof
	Requester string `json:"requester"` // 操作人账号
	Note string    `json:"note,omitempty"`
	At   time.Time `json:"at"`
}

// ReleaseProof 释放证明，不可变；按 ProofRef 幂等查询。
type ReleaseProof struct {
	ProofID string `json:"proof_id"`
	ProofRef string `json:"proof_ref"` // 等于 ReleaseReq.IdemKey，全局唯一
	TokenID  string `json:"token_id"`
	UserID   string `json:"user_id"`
	Asset    string `json:"asset"`
	Book        string `json:"book"`
	Amount      string `json:"amount"`
	FrozenLayer string `json:"frozen_layer"` // 被释放 token 的层
	CallerLayer string `json:"caller_layer"`
	ReasonCode  string `json:"reason_code"`
	ApprovalRef string `json:"approval_ref,omitempty"`
	LedgerRef   string `json:"ledger_ref,omitempty"`
	Requester string `json:"requester"`
	Note      string `json:"note,omitempty"`
	ReleasedAt time.Time `json:"released_at"`
}

// GapKind 缺口类型枚举。
type GapKind string

const (
	GapKindOrphanTag      GapKind = "orphan_tag"      // 分类账有标签但账本无冻结
	GapKindLedgerUnfrozen GapKind = "ledger_unfrozen" // 账本已解冻但分类账仍 active
	GapKindLayerViolation GapKind = "layer_violation" // 跨层违规释放尝试
	GapKindAmountDrift    GapKind = "amount_drift"    // 令牌金额与账本冻结不符
	GapKindMissingToken GapKind = "missing_token" // 账本有冻结但无对应 token
	GapKindTimeout GapKind = "timeout_escalation" // 超时未释升级
)

// GapState 缺口单生命周期。
type GapState string

const (
	GapStateOpen  GapState = "open"  // 新建，待确认
	GapStateAcked GapState = "acked" // 已确认，处理中
	GapStateClosed GapState = "closed" // 已关闭
)

// HoldGap 缺口工单。
type HoldGap struct {
	GapID   string `json:"gap_id"`
	TokenID string `json:"token_id,omitempty"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Book   string `json:"book"`
	Kind GapKind `json:"kind"`
	State GapState `json:"state"`
	Layer string   `json:"layer,omitempty"`
	TokenAmount  string `json:"token_amount,omitempty"`
	LedgerFrozen string `json:"ledger_frozen,omitempty"`
	Detail       string `json:"detail"`
	AckedBy string `json:"acked_by,omitempty"`
	ClosedBy string `json:"closed_by,omitempty"`
	Note     string    `json:"note,omitempty"`
	OpenedAt time.Time `json:"opened_at"`
	AckedAt  *time.Time `json:"acked_at,omitempty"`
	ClosedAt *time.Time `json:"closed_at,omitempty"`
}

// ReleaseAudit 释放操作审计行（无论成功/拒绝均记录）。
type ReleaseAudit struct {
	AuditID string `json:"audit_id"`
	TokenID string `json:"token_id"`
	CallerLayer string `json:"caller_layer"`
	FrozenLayer string `json:"frozen_layer"`
	ReasonCode string `json:"reason_code"`
	Requester  string `json:"requester"`
	Allowed    bool   `json:"allowed"`
	DenyReason string `json:"deny_reason,omitempty"`
	ProofID string `json:"proof_id,omitempty"` // 成功时引用 proof
	At time.Time `json:"at"`
}
