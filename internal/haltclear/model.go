package haltclear

import (
	"fmt"
	"time"
)

// ---- MisClearCase ---------------------------------------------------
// MisClearCaseState 误清案例状态机：open → reengaged → closed。
type MisClearCaseState string

const (
	MisClearStateOpen MisClearCaseState = "open"
	MisClearStateReengaged MisClearCaseState = "reengaged"
	MisClearStateClosed MisClearCaseState = "closed"
)

// MisClearTrigger 误清触发来源。
type MisClearTrigger string

const (
	TriggerForcedClear MisClearTrigger = "forced_clear" // ClearForced 后仍有差异
	TriggerOpenDiffScan MisClearTrigger = "open_diff_scan" // 周期巡检发现 Flag.On=false 但差异仍开
	TriggerManual       MisClearTrigger = "manual"         // 值班手工上报
)

// MisClearCase 误清案例：已解除熔断但仍存在未关差异的异常状态。
// 状态机：open → reengaged（再熔断触发）→ closed（差异已处理）。
// 产出 MisclearID 供对账路径只读关联；本包不写对账工单。

type MisClearCase struct {
	MisclearID string `json:"misclear_id"`
	Scope string `json:"scope"` // global / chain / asset / pair
	Chain string `json:"chain,omitempty"`
	Asset      string            `json:"asset,omitempty"`
	Trigger    MisClearTrigger   `json:"trigger"`
	State      MisClearCaseState `json:"state"`
	DetectedAt time.Time         `json:"detected_at"`
	OpenDiffCount int `json:"open_diff_count"` // 发现时未关差异数
	OpenDiffKeys []string `json:"open_diff_keys,omitempty"` // 最多 32 条差异工单 key
	// 关联上下文（只读）。
	ForcedAuditID string `json:"forced_audit_id,omitempty"` // 触发源审计 ID
	ClearPlanID string `json:"clear_plan_id,omitempty"` // 关联四眼计划 ID（可为空）
	// 再熔断信息。
	ReengagedAt     time.Time `json:"reengaged_at,omitempty"`
	ReengageActorID string    `json:"reengage_actor_id,omitempty"` // system / 运营账号
	ReengageLegID string `json:"reengage_leg_id,omitempty"` // 关联 ReEngageLeg
	// 关闭信息。
	ClosedAt time.Time `json:"closed_at,omitempty"`
	ClosedBy string `json:"closed_by,omitempty"`
	CloseNote string `json:"close_note,omitempty"`
	// 审计字段。
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// IsTerminal 判断案例是否已终态。

   func (m *MisClearCase) IsTerminal() bool {
	  return m.State == MisClearStateClosed
   }

// ---- ReEngageLeg ---------------------------------------------------
// ReEngageLegState 再熔断腿状态：pending → executing → engaged / failed。

type ReEngageLegState string

const (
	ReEngageStatePending   ReEngageLegState = "pending"
	ReEngageStateExecuting ReEngageLegState = "executing"
	ReEngageStateEngaged ReEngageLegState = "engaged"
	ReEngageStateFailed  ReEngageLegState = "failed"
)

// ReEngageLeg 误清后自动/半自动再熔断腿；记录回滚证明。
// 一个 MisClearCase 对应最多一条活跃 ReEngageLeg；重试时新建腿。
type ReEngageLeg struct {
	LegID string `json:"leg_id"`
	MisclearID string `json:"misclear_id"` // 关联的误清案例
	Scope string `json:"scope"`
	Chain string           `json:"chain,omitempty"`
	Asset string           `json:"asset,omitempty"`
	State ReEngageLegState `json:"state"`
	ActorID string `json:"actor_id"` // system 或操作人
	Reason  string `json:"reason"`
	IsAuto      bool      `json:"is_auto"` // true=自动触发；false=半自动（需人工确认）
	ScheduledAt time.Time `json:"scheduled_at"`
	ExecutedAt  time.Time `json:"executed_at,omitempty"`
	EngagedAt  time.Time `json:"engaged_at,omitempty"`
	FailedAt   time.Time `json:"failed_at,omitempty"`
	FailReason string    `json:"fail_reason,omitempty"`
	// 回滚证明：再熔断成功后的不可变快照。
	RollbackProof *RollbackProof `json:"rollback_proof,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// RollbackProof 再熔断回滚证明；供审计与对账一致。

type RollbackProof struct {
	ProofID    string `json:"proof_id"`
	LegID      string `json:"leg_id"`
	MisclearID string `json:"misclear_id"`
	Scope      string `json:"scope"`
	Chain     string    `json:"chain,omitempty"`
	Asset     string    `json:"asset,omitempty"`
	EngagedAt time.Time `json:"engaged_at"`
	ActorID   string    `json:"actor_id"`
	Reason        string `json:"reason"`
	OpenDiffCount int    `json:"open_diff_count"` // 再熔断时实测差异数
	Checksum      string `json:"checksum"`        // 证明指纹
}

// VerifyRollbackProof 校验回滚证明指纹。
   func VerifyRollbackProof(p *RollbackProof) bool {
	   if p == nil {
		  return false
	   }
	   want := rollbackChecksum(p.LegID, p.MisclearID, p.Scope, p.ActorID, p.OpenDiffCount)
	   return want == p.Checksum
   }

   func rollbackChecksum(legID, misclearID, scope, actor string, openDiffs int) string {
	   raw := legID + "|" + misclearID + "|" + scope + "|" + actor + "|" + fmt.Sprintf("%d", openDiffs)
	   var acc uint64 = 14695981039346656037
	  for i := 0; i < len(raw); i++ {
		   acc ^= uint64(raw[i])
		  acc *= 1099511628211
	   }
	   return fmt.Sprintf("%016x", acc)
   }

// NewRollbackProof 生成回滚证明。
   func NewRollbackProof(leg *ReEngageLeg, openDiffs int) *RollbackProof {
	  proofID := fmt.Sprintf("rbp:%s:%d", leg.LegID, time.Now().UnixNano())
	  cs := rollbackChecksum(leg.LegID, leg.MisclearID, leg.Scope, leg.ActorID, openDiffs)
	   return &RollbackProof{
		  ProofID:    proofID,
		   LegID:      leg.LegID,
		  MisclearID: leg.MisclearID,
		   Scope: leg.Scope,
		   Chain:     leg.Chain,
		  Asset:     leg.Asset,
		   EngagedAt: time.Now().UTC(),
		   ActorID: leg.ActorID,
		   Reason: leg.Reason,
		   OpenDiffCount: openDiffs,
		  Checksum:      cs,
	   }
  }
