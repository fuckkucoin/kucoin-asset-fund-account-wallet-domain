package georebal

import (
	"encoding/json"
	"strings"
	"time"
	"github.com/google/uuid"
)

// Plan 状态机: draft → armed → running → done | aborted
// 区 halt Engage 后拒绝新建，正在运行的计划在当前腿完成后暂停。
const (
	PlanDraft = "draft"
	PlanArmed = "armed"
	PlanRunning = "running"
	PlanDone    = "done"
	PlanAborted = "aborted"
	ApprovalPending = "pending"
	ApprovalApproved = "approved"
	ApprovalRejected = "rejected"
	LegQueued = "queued"
	LegRunning = "running"
	LegDone = "done"
	LegFailed = "failed"
	LegAborted = "aborted"
)

var planTransitions = map[string][]string{
	PlanDraft: {PlanArmed, PlanAborted},
	PlanArmed: {PlanRunning,
		PlanAborted},
	PlanRunning: {PlanDone, PlanAborted},
	PlanDone:    {},
	PlanAborted: {},
}

var legTransitions = map[string][]string{
	LegQueued: {LegRunning, LegAborted},
	LegRunning: {LegDone, LegFailed},
	LegDone:    {},
	LegFailed:  {LegAborted},
	LegAborted: {},
}

  func AllowPlanTransition(from, to string) bool {
	  for _, ok := range planTransitions[from] {
		  if ok == to {
			   return true
		  }
	   }
	   return false
  }

  func AllowLegTransition(from, to string) bool {
	  for _, ok := range legTransitions[from] {
		  if ok == to {
			   return true
		  }
	   }
	  return false
   }

// GeoRebalPlan 跨区再平衡计划。Amount 是计划总量；Legs 逐条执行。
type GeoRebalPlan struct {
	ID   string `json:"georebal_plan_id"`
	Idem string `json:"idem"`
	Asset string `json:"asset"`
	Chain string `json:"chain"`
	Tier  string `json:"tier"`
	SrcRegion string `json:"src_region"`
	DstRegion string `json:"dst_region"`
	Amount string `json:"amount"`
	State    string `json:"state"`
	Approval string `json:"approval"`
	Actor    string `json:"actor"`
	Reason string `json:"reason"`
	Code   string `json:"code,omitempty"`
	Hooks     map[string]string `json:"hooks,omitempty"`
	Legs      []GeoRebalLeg     `json:"legs,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

  func (p GeoRebalPlan) Clone() GeoRebalPlan {
	   cp := p
	  if p.Hooks != nil {
		   cp.Hooks = make(map[string]string, len(p.Hooks))
		  for k, v := range p.Hooks {
			   cp.Hooks[k] = v
		   }
	  }
	   if p.Legs != nil {
		  cp.Legs = make([]GeoRebalLeg, len(p.Legs))
		  copy(cp.Legs, p.Legs)
	  }
	  return cp
   }

  func (p GeoRebalPlan) Terminal() bool {
	  return p.State == PlanDone || p.State == PlanAborted
   }

// GeoRebalLeg 是计划内单条执行腿。SweepIntentID 只读引用。
type GeoRebalLeg struct {
	ID string `json:"id"`
	PlanID    string `json:"plan_id"`
	Seq       int    `json:"seq"`
	SrcRegion string `json:"src_region"`
	DstRegion string `json:"dst_region"`
	Tier      string `json:"tier"`
	Asset         string `json:"asset"`
	Chain         string `json:"chain"`
	Amount        string `json:"amount"`
	State         string `json:"state"`
	SweepIntentID string `json:"sweep_intent_id,omitempty"`
	Reason string `json:"reason,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreatePlanReq 新建跨区再平衡计划请求。

type CreatePlanReq struct {
	Idem string `json:"idem"`
	Asset string `json:"asset"`
	Chain string `json:"chain"`
	Tier  string `json:"tier"`
	SrcRegion string `json:"src_region"`
	DstRegion string `json:"dst_region"`
	Amount    string `json:"amount"`
	Actor     string `json:"actor"`
	Reason    string `json:"reason"`
	Hooks map[string]string `json:"hooks,omitempty"`
}

// ApprovePlanReq 审批挂钩。

type ApprovePlanReq struct {
	PlanID string `json:"plan_id"`
	Actor  string `json:"actor"`
	Approved bool   `json:"approved"`
	Reason   string `json:"reason"`
}

// ArmPlanReq 将 draft → armed（跳过审批或审批已通过）。

type ArmPlanReq struct {
	PlanID string `json:"plan_id"`
	Actor  string `json:"actor"`
}

// AbortPlanReq 中止计划。

type AbortPlanReq struct {
	PlanID string `json:"plan_id"`
	Actor  string `json:"actor"`
	Reason string `json:"reason"`
}

// LegDoneReq 标记某条腿完成（调用方已执行归集意图）。

type LegDoneReq struct {
	PlanID string `json:"plan_id"`
	LegSeq        int    `json:"leg_seq"`
	SweepIntentID string `json:"sweep_intent_id"`
	Actor         string `json:"actor"`
}

// ExecResult 是执行一步后的计划与当前腿快照。
type ExecResult struct {
	Plan GeoRebalPlan `json:"plan"`
	Leg     *GeoRebalLeg `json:"leg,omitempty"`
	Advance bool         `json:"advance"`
}

// HooksJSON 序列化 hooks。

   func hooksJSON(h map[string]string) string {
	   if len(h) == 0 {
		  return "{}"
	   }
	  b, _ := json.Marshal(h)
	   return string(b)
   }

   func parseHooks(s string) map[string]string {
	  s = strings.TrimSpace(s)
	  if s == "" || s == "{}" {
		  return nil
	   }
	  var m map[string]string
	   _ = json.Unmarshal([]byte(s), &m)
	   return m
   }

   func newPlanID() string {
	  return "grp" + strings.ReplaceAll(uuid.NewString(), "-", "")
  }

   func newLegID() string {
	   return "grl" + strings.ReplaceAll(uuid.NewString(), "-", "")
  }
