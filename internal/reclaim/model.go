package reclaim

import "time"

// ─────────────────────────────── OutputKey ──────────────────────────────────
// OutputKey 链上输出唯一标识。
// 三元组（chain, tx_hash, output_index）共同标识一枚 UTXO / 合约输出。

type OutputKey struct {
	Chain       string `json:"chain"`
	TxHash      string `json:"tx_hash"`
	OutputIndex int    `json:"output_index"`
}

// String 返回 "chain|tx_hash|output_index" 复合键，用作 map 键与日志打印。
   func (k OutputKey) String() string {
	   if k.Chain == "" || k.TxHash == "" {
		  return ""
	  }
	  b := make([]byte, 0, len(k.Chain)+len(k.TxHash)+24)
	   b = append(b, k.Chain...)
	  b = append(b, '|')
	   b = append(b, k.TxHash...)
	   b = append(b, '|')
	   b = appendInt(b, k.OutputIndex)
	  return string(b)
   }

  func appendInt(buf []byte, n int) []byte {
	   if n == 0 {
		  return append(buf, '0')
	  }
	   tmp := [20]byte{}
	   pos := len(tmp)
	   neg := n < 0
	   if neg {
		  n = -n
	   }
	  for n > 0 {
		  pos--
		  tmp[pos] = byte('0' + n%10)
		   n /= 10
	  }
	  if neg {
		   pos--
		   tmp[pos] = '-'
	  }
	   return append(buf, tmp[pos:]...)
  }

// ─────────────────────────────── CaseView ────────────────────────────────
// ReclaimCaseState 回收工单只读视图状态（不修改 编排主链）。
type ReclaimCaseState string

const (
	ReclaimCasePending ReclaimCaseState = "pending"
	ReclaimCaseOpen ReclaimCaseState = "open"
	ReclaimCaseReserved ReclaimCaseState = "reserved"
	ReclaimCaseReclaimed ReclaimCaseState = "reclaimed"
	ReclaimCaseClosed    ReclaimCaseState = "closed"
	ReclaimCaseAborted ReclaimCaseState = "aborted"
)

// CaseView 错链回收工单只读视图（ latecreditgate 挂钩）。
// 仅供 latecreditgate 检查当前 OutputKey 是否处于活跃回收流程中，
// 不得对编排主记录 ReclaimCase 执行任何状态写操作。
type CaseView struct {
	CaseID    string    `json:"case_id"`
	OutputKey OutputKey `json:"output_key"`
	OriginalChain string `json:"original_chain"`
	TargetChain   string `json:"target_chain"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	State     ReclaimCaseState `json:"state"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	ClosedAt  *time.Time `json:"closed_at,omitempty"`
}

// IsActive 返回工单是否处于活跃回收阶段（非终态）。

   func (c CaseView) IsActive() bool {
	   switch c.State {
	  case ReclaimCasePending, ReclaimCaseOpen, ReclaimCaseReserved:
		   return true
	   default:
		  return false
	   }
   }

// IsTerminal 返回工单是否已终止（无法再回收）。

   func (c CaseView) IsTerminal() bool {
	   return c.State == ReclaimCaseClosed || c.State == ReclaimCaseAborted
  }

// ─────────────────────────────── OccupancyView ───────────────────────────
// OccupancyState 占用状态（与 latecreditgate.GenState 协同但独立）。
type OccupancyState string

const (
	OccupancyActive   OccupancyState = "active"
	OccupancyReleased OccupancyState = "released"
	OccupancyExpired OccupancyState = "expired"
)

// OccupancyView 回收占用记录只读视图。
// 当 reclaim 流程处于 Reserved 状态时写入；latecreditgate
// 通过 OccupyWithGen 与此记录进行世代关联，防止迟贷记并发写入。
type OccupancyView struct {
	OccupancyID string    `json:"occupancy_id"`
	CaseID      string    `json:"case_id"`
	OutputKey   OutputKey `json:"output_key"`
	UserID      string    `json:"user_id"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	State OccupancyState `json:"state"`
	OccupiedAt time.Time  `json:"occupied_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	ReleasedAt *time.Time `json:"released_at,omitempty"`
	LateGenID string `json:"late_gen_id,omitempty"` // 关联 late_credit_gens.id
}

// IsOccupying 返回占用是否仍然有效。
  func (o OccupancyView) IsOccupying(now time.Time) bool {
	   return o.State == OccupancyActive && now.Before(o.ExpiresAt)
  }

// ─────────────────────────────── ViewTimeline ───────────────────────────────────
// ViewTimelineEvent 回收流程时间线事件（ 只读挂钩，异于编排 TimelineEvent）。
type ViewTimelineEvent struct {
	EventID string `json:"event_id"`
	CaseID string `json:"case_id"`
	OutputKey OutputKey `json:"output_key"`
	Kind  string `json:"kind"` // "occupy" | "credit_check" | "auto_credit" | "reclaimed" | "race_open" | "race_resolved"
	Actor string `json:"actor"`
	Detail     string    `json:"detail,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

// ViewTimeline 回收流程时间线（只读快照）。
type ViewTimeline struct {
	OutputKey OutputKey           `json:"output_key"`
	Events    []ViewTimelineEvent `json:"events"`
}

// ─────────────────────────────── Reader 接口 ─────────────────────────────────
// ReclaimCaseReader 只读消费 CaseView（由 reclaim 包或适配层实现）。
type ReclaimCaseReader interface {
	GetActiveCase(outputKey OutputKey) (*CaseView, error)
}

// ReclaimOccupancyReader 只读消费 OccupancyView（由 reclaim 包或适配层实现）。
type ReclaimOccupancyReader interface {
	GetActiveOccupancy(outputKey OutputKey) (*OccupancyView,
		error)
}

// TimelineAppender 向 挂钩时间线追加事件。
type TimelineAppender interface {
	AppendEvent(event ViewTimelineEvent) error
}

// NopTimelineAppender 空实现，用于或无时间线场景。
type NopTimelineAppender struct{}

  func (NopTimelineAppender) AppendEvent(ViewTimelineEvent) error { return nil }

// NopCaseReader 空实现，始终返回无活跃工单。
type NopCaseReader struct{}

  func (NopCaseReader) GetActiveCase(OutputKey) (*CaseView, error) { return nil, nil }

// NopOccupancyReader 空实现，始终返回无活跃占用。
type NopOccupancyReader struct{}

  func (NopOccupancyReader) GetActiveOccupancy(OutputKey) (*OccupancyView, error) { return nil, nil }
