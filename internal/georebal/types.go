package georebal

import "time"

// PlanState 再平衡计划状态。
type PlanState string

const (
	FencePlanDraft PlanState = "draft"
	FencePlanOpen  PlanState = "open"
	FencePlanFinalised PlanState = "finalised"
	FencePlanAborted PlanState = "aborted"
)

// LegState 腿状态。failed 可重试。
type LegState string

const (
	FenceLegPending LegState = "pending"
	FenceLegRunning LegState = "running"
	FenceLegDone    LegState = "done"
	FenceLegFailed  LegState = "failed"
)

// RegionStatus 区域运行态。Halted 后旧 fence 必须 fail-closed。
type RegionStatus string

const (
	RegionLive   RegionStatus = "live"
	RegionHalted RegionStatus = "halted"
	RegionDrained RegionStatus = "drained"
)

// Plan 跨区再平衡计划。
type Plan struct {
	PlanID string    `json:"plan_id"`
	Asset  string    `json:"asset"`
	State  PlanState `json:"state"`
	Note string `json:"note,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Leg 单条再平衡腿：源区 → 目标区。
type Leg struct {
	LegID  string `json:"leg_id"`
	FailReason string    `json:"fail_reason,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Region 区域库存与运行态。

type Region struct {
	RegionID string       `json:"region_id"`
	Name     string       `json:"name"`
	Status   RegionStatus `json:"status"`
	HaltGen   int64            `json:"halt_gen"`
	Balances  map[string]int64 `json:"balances"`
	UpdatedAt time.Time        `json:"updated_at"`
}

// CollectIntent Enqueue 归集意图观察记录（不改写 sweeprun FSM）。

type CollectIntent struct {
	IntentID string `json:"intent_id"`
	PlanID string `json:"plan_id"`
	LegID string `json:"leg_id"`
	SrcRegion string `json:"src_region"`
	DstRegion string `json:"dst_region"`
	Asset     string `json:"asset"`
	Amount   int64  `json:"amount"`
	FenceID  string `json:"leg_move_fence_id,omitempty"`
	Enqueued bool   `json:"enqueued"`
	CreatedAt time.Time `json:"created_at"`
}

// InventoryMove 区域库存划转记录。

type InventoryMove struct {
	MoveID string `json:"move_id"`
	PlanID string `json:"plan_id"`
	LegID string `json:"leg_id"`
	FenceID string `json:"leg_move_fence_id"`
	SrcRegion string `json:"src_region"`
	DstRegion string `json:"dst_region"`
	Asset     string    `json:"asset"`
	Amount    int64     `json:"amount"`
	Success   bool      `json:"success"`
	Reason    string    `json:"reason,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// LegSnapshot 只读快照，供 fence 复验。
type LegSnapshot struct {
	Plan Plan
	Leg Leg
	Src    Region
	Dst    Region
	Intent *CollectIntent
	LastMove *InventoryMove
	ObservedAt time.Time
}

// AmountFingerprint 金额与路径指纹原料。
   func AmountFingerprint(planID, legID, asset, src, dst string, amount int64) string {
	  return planID + "|" + legID + "|" + asset + "|" + src + "|" + dst + "|" + itoa64(amount)
  }

  func itoa64(n int64) string {
	  if n == 0 {
		  return "0"
	  }
	  neg := n < 0
	  if neg {
		   n = -n
	   }
	   var buf [20]byte
	   i := len(buf)
	  for n > 0 {
		   i--
		  buf[i] = byte('0' + n%10)
		   n /= 10
	  }
	   if neg {
		  i--
		  buf[i] = '-'
	   }
	   return string(buf[i:])
  }
