package dustguard

import "time"

// ─── 攻击信号 ─────────────────────────────────────────────────────────────
// SignalKind 信号来源类型。

type SignalKind string

const (
	SignalFlood   SignalKind = "flood"   // 窗口内高频低额入站
	SignalCluster SignalKind = "cluster" // 同 from-addr 簇射
	SignalSurface SignalKind = "surface" // 共享 FamMEMO 面被刷尘
)

// DustSignal 单条粉尘信号，由 Detector.Record 写入。

type DustSignal struct {
	ID string `json:"id"`
	Chain string `json:"chain"`
	Address  string `json:"address"`
	FromAddr string `json:"from_addr"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	TxHash string `json:"tx_hash"`
	SignalKind SignalKind `json:"signal_kind"`
	SeenAt time.Time `json:"seen_at"`
}

// ─── 攻击分值 ─────────────────────────────────────────────────────────────
// AttackLevel 攻击等级，由综合分值推断。
type AttackLevel string

const (
	LevelNone       AttackLevel = "none"
	LevelSuspicious AttackLevel = "suspicious"
	LevelAttacking  AttackLevel = "attacking"
	LevelCritical AttackLevel = "critical"
)

// AttackScore 某链/地址的实时攻击分值快照。

type AttackScore struct {
	ID string `json:"id"`
	Chain string `json:"chain"`
	Address string `json:"address"`
	Family string `json:"family"`
	Score  int    `json:"score"` // 0-100
	Level AttackLevel `json:"level"`
	FloodCount int `json:"flood_count"` // 窗口内命中次数
	ClusterN   int       `json:"cluster_n"`   // 最大同 from-addr 命中数
	SurfaceHit bool      `json:"surface_hit"` // 共享面是否被刷
	UpdatedAt  time.Time `json:"updated_at"`
}

// AttackLevelFromScore 把数值分值映射到等级。
  func AttackLevelFromScore(score int) AttackLevel {
	  switch {
	   case score >= 80:
		  return LevelCritical
	   case score >= 60:
		  return LevelAttacking
	   case score >= 40:
		  return LevelSuspicious
	  default:
		   return LevelNone
	  }
  }

// ─── 隔离案 (QuarantineCase) ──────────────────────────────────────────────
// QuarantineState 隔离案状态。
// open → holding → released
// └→ burned
type QuarantineState string

const (
	QStateOpen QuarantineState = "open" // 攻击触发，地址被锁
	QStateHolding  QuarantineState = "holding"  // 进入取证窗口
	QStateReleased QuarantineState = "released" // 清白，可再分配
	QStateBurned QuarantineState = "burned" // 永久拉黑
)

// QuarantineCase 地址隔离案记录。

type QuarantineCase struct {
	ID    string `json:"id"`
	Chain string `json:"chain"`
	Address      string          `json:"address"`
	Family       string          `json:"family"`
	UserID       string          `json:"user_id"`
	State        QuarantineState `json:"state"`
	TriggerScore int             `json:"trigger_score"`
	TriggerLevel   AttackLevel `json:"trigger_level"`
	TriggerReason  string      `json:"trigger_reason"`
	ForensicsUntil time.Time   `json:"forensics_until"` // 取证窗口截止时间
	OpenedBy string `json:"opened_by"`
	ClosedBy string `json:"closed_by,omitempty"`
	RotatePlanID string `json:"rotate_plan_id,omitempty"`
	EventFactID  string `json:"event_fact_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// IsActive 案件是否仍在活跃隔离中（禁止分配）。

   func (c *QuarantineCase) IsActive() bool {
	   return c != nil && (c.State == QStateOpen || c.State == QStateHolding)
  }

// ─── 轮换计划 (RotatePlan) ────────────────────────────────────────────────
// RotateState 轮换计划状态。
type RotateState string

const (
	RStatePlanned RotateState = "planned" // 计划创建
	RStateApproved RotateState = "approved" // 双人复核通过（FamMEMO 必须）
	RStateSwapped RotateState = "swapped" // 地址已实际切换
	RStateSealed  RotateState = "sealed"  // 归档完毕
	RStateAborted RotateState = "aborted" // 中止
)

// TriggerKind 轮换触发来源。

type TriggerKind string

const (
	TriggerScore TriggerKind = "score" // 攻击分值超阈值
	TriggerWaterLevel TriggerKind = "waterlevel" // 地址池空闲水位低
	TriggerOps        TriggerKind = "ops"        // 运维手动触发
)

// RotatePlan 单次轮换计划。

type RotatePlan struct {
	ID string `json:"id"`
	Chain string `json:"chain"`
	OldAddress string `json:"old_address"`
	NewAddress string `json:"new_address,omitempty"`
	UserID  string      `json:"user_id"`
	CaseID  string      `json:"case_id,omitempty"`
	State   RotateState `json:"state"`
	Trigger TriggerKind `json:"trigger"`
	ApprovalID string `json:"approval_id,omitempty"`
	Reason     string    `json:"reason"`
	ExecutedBy string    `json:"executed_by,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	SealedAt  *time.Time `json:"sealed_at,omitempty"`
}

// ─── 守护策略 (PoolGuardPolicy) ──────────────────────────────────────────
// PoolGuardPolicy 每链（或全局）防护策略代次。
// 空 Chain 表示全局默认；链级策略先级高于全局。
type PoolGuardPolicy struct {
	ID string `json:"id"`
	Version int `json:"version"`
	Chain          string `json:"chain"`           // 空 = 全局
	FloodThreshold int    `json:"flood_threshold"` // 触发洪水信号所需窗口内命中数
	FloodWindowSec int    `json:"flood_window_sec"`
	ClusterN     int `json:"cluster_n"`     // 同 from-addr 命中≥N → 簇射
	ScoreAttack  int `json:"score_attack"`  // 分值≥此值 → 自动隔离
	ScoreSuspect int `json:"score_suspect"` // 分值≥此值 → suspicious
	WaterLevel     int  `json:"water_level"`   // 空闲地址槽占比（%）低于此值 → 智能轮换
	ForensicsSec   int  `json:"forensics_sec"` // 取证窗口长度（秒）
	AutoQuarantine bool `json:"auto_quarantine"`
	AutoRotate     bool `json:"auto_rotate"`
	Active bool `json:"active"`
	Note string `json:"note"`
	CreatedBy string `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DefaultPolicy 全局默认策略（无 SQL 时兜底）。
   func DefaultPolicy() PoolGuardPolicy {
	  return PoolGuardPolicy{
		  FloodThreshold: 50,
		  FloodWindowSec: 3600,
		   ClusterN:       10,
		   ScoreAttack: 75,
		  ScoreSuspect:   40,
		   WaterLevel:     20,
		  ForensicsSec:   86400,
		  AutoQuarantine: true,
		   AutoRotate: false,
		   Active: true,
	   }
  }

// ─── 防护单 (GuardTicket) ────────────────────────────────────────────────
// TicketState 防护单状态。

type TicketState string

const (
	TicketOpen     TicketState = "open"     // 待处理
	TicketBlocking TicketState = "blocking" // 贷记被拦截
	TicketReleased TicketState = "released" // 已解除拦截
)

// GuardTicket 针对污染地址观察的挡贷记工单。

type GuardTicket struct {
	ID string `json:"id"`
	Chain   string `json:"chain"`
	Address string `json:"address"`
	UserID  string `json:"user_id"`
	TxHash  string `json:"tx_hash"`
	Asset string `json:"asset"`
	Amount string      `json:"amount"`
	CaseID string      `json:"case_id"`
	State  TicketState `json:"state"`
	Reason string      `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ─── 审计记录 (GuardAudit) ───────────────────────────────────────────────
// AuditKind 审计记录所属类型。

type AuditKind string

const (
	AuditQuarantine AuditKind = "quarantine"
	AuditRotate AuditKind = "rotate"
	AuditPolicy AuditKind = "policy"
	AuditTicket AuditKind = "ticket"
)

// GuardAudit 操作审计行，写后不修改。

type GuardAudit struct {
	ID   string    `json:"id"`
	Kind AuditKind `json:"kind"`
	RefID string `json:"ref_id"`
	Actor string `json:"actor"`
	Event     string `json:"event"`
	FromState string `json:"from_state,omitempty"`
	ToState   string `json:"to_state,omitempty"`
	Note string `json:"note,omitempty"`
	FactID string    `json:"fact_id,omitempty"` // 供下游因果图消费者关联防护事件
	At     time.Time `json:"at"`
}

// ─── 挡贷记钩结果 ────────────────────────────────────────────────────────
// ObserveGuardResult 观察路径防护检查结果。
type ObserveGuardResult struct {
	Blocked bool `json:"blocked"`
	TicketID string `json:"ticket_id,omitempty"`
	CaseID   string `json:"case_id,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// ─── 轮换结果 ─────────────────────────────────────────────────────────────
// RotateResult 轮换执行结果。
type RotateResult struct {
	Plan RotatePlan `json:"plan"`
	NewAddress string `json:"new_address"`
	UserID  string `json:"user_id"`
	AuditID string `json:"audit_id"`
}
