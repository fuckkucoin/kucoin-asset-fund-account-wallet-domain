package occupbook

import (
	"time"
	"github.com/shopspring/decimal"
)

// RiskClass 观察风险档。仅作占用桶标签，不驱动 r4 比例内核。

type RiskClass string

const (
	RiskNormal RiskClass = "normal"
	RiskT1     RiskClass = "t1"
	RiskT2     RiskClass = "t2"
	RiskT3     RiskClass = "t3"
	RiskT4 RiskClass = "t4"
	RiskMixer RiskClass = "mixer"
)

// RequiresFullOccupancy T4/mixer 必须全额占用且不得旁路围栏。

   func (c RiskClass) RequiresFullOccupancy() bool {
	   return c == RiskT4 || c == RiskMixer
   }

// Valid 风险档是否在观察集合内。
  func (c RiskClass) Valid() bool {
	   switch c {
	   case RiskNormal, RiskT1, RiskT2, RiskT3, RiskT4, RiskMixer:
		   return true
	   default:
		  return false
	  }
  }

// LotState 占用批次状态。

type LotState string

const (
	LotObserved LotState = "observed"
	LotHeld LotState = "held"
	LotOpen LotState = "open"
	LotClosed LotState = "closed"
	LotBroken LotState = "broken"
)

// Terminal 批次是否终态。

   func (s LotState) Terminal() bool {
	   return s == LotClosed || s == LotBroken
   }

// Bucket 三桶余额快照。
// Inflight=观察后在途；Available=确认后可用；Void=升档作废。

type Bucket struct {
	Inflight  decimal.Decimal `json:"inflight"`
	Available decimal.Decimal `json:"available"`
	Void decimal.Decimal `json:"void"`
}

// Sum 三桶合计。
  func (b Bucket) Sum() decimal.Decimal {
	   return b.Inflight.Add(b.Available).Add(b.Void)
  }

// Clone 复制快照，避免调用方改写内部余额。

   func (b Bucket) Clone() Bucket {
	   return Bucket{
		  Inflight:  b.Inflight.Copy(),
		   Available: b.Available.Copy(),
		  Void:      b.Void.Copy(),
	  }
  }

// Zero 三桶是否全零。

   func (b Bucket) Zero() bool {
	  return b.Inflight.IsZero() && b.Available.IsZero() && b.Void.IsZero()
   }

// Lot 充值占用批次。

type Lot struct {
	LotID string `json:"lot_id"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	OccupiedTotal decimal.Decimal `json:"occupied_total"`
	ObservedAmt decimal.Decimal `json:"observed_amt"`
	RiskClass   RiskClass       `json:"risk_class"`
	State LotState `json:"state"`
	Buckets    Bucket `json:"buckets"`
	ObserveRef string `json:"observe_ref"`
	HalfOpen   bool   `json:"half_open"`
	Generation int64 `json:"generation"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	ClosedAt  *time.Time `json:"closed_at,omitempty"`
}

// ConserveOK 三桶合计是否等于占用总额。

   func (l *Lot) ConserveOK() bool {
	  if l == nil {
		  return false
	   }
	  return l.Buckets.Sum().Equal(l.OccupiedTotal)
  }

// Hold 在途占用单据。观察后记在途。
type Hold struct {
	HoldID string `json:"hold_id"`
	LotID  string `json:"lot_id"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount decimal.Decimal `json:"amount"`
	IdemKey string `json:"idem_key"`
	RiskClass RiskClass `json:"risk_class"`
	Released  bool `json:"released"`
	Reclassed bool `json:"reclassed"`
	PersistOK bool `json:"persist_ok"`
	CreatedAt time.Time `json:"created_at"`
	ReleasedAt *time.Time `json:"released_at,omitempty"`
}

// Terminal Hold 是否已离开在途（释放或作废）。

  func (h *Hold) Terminal() bool {
	  if h == nil {
		   return false
	  }
	   return h.Released || h.Reclassed
   }

// Release 确认后从在途转到可用。

type Release struct {
	ReleaseID string `json:"release_id"`
	HoldID    string `json:"hold_id"`
	LotID  string          `json:"lot_id"`
	Amount decimal.Decimal `json:"amount"`
	IdemKey string `json:"idem_key"`
	CreatedAt time.Time `json:"created_at"`
}

// Reclass 风险升档：从在途转入作废，禁止与 Release 交错双释。
type Reclass struct {
	ReclassID string    `json:"reclass_id"`
	HoldID    string    `json:"hold_id"`
	LotID     string    `json:"lot_id"`
	FromClass RiskClass `json:"from_class"`
	ToClass   RiskClass `json:"to_class"`
	Amount  decimal.Decimal `json:"amount"`
	IdemKey string          `json:"idem_key"`
	ReasonCode string `json:"reason_code"`
	CreatedAt time.Time `json:"created_at"`
}

// AuditEvent 占用桶审计。失败不得掩盖击穿。
type AuditEvent struct {
	AuditID string `json:"audit_id"`
	LotID   string `json:"lot_id"`
	Action string `json:"action"`
	Reason string `json:"reason"`
	OK bool `json:"ok"`
	Inflight  string `json:"inflight"`
	Available string `json:"available"`
	Void      string `json:"void"`
	Total     string    `json:"total"`
	CreatedAt time.Time `json:"created_at"`
}

// ObserveReq 观察入账请求（只记观察额，不改比例公式）。

type ObserveReq struct {
	LotID  string `json:"lot_id"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount    string `json:"amount"`
	RiskClass string `json:"risk_class"`
	ObserveRef string `json:"observe_ref"`
}

// HoldReq 在途占用请求。
type HoldReq struct {
	LotID   string `json:"lot_id"`
	HoldID  string `json:"hold_id"`
	Amount  string `json:"amount"`
	IdemKey string `json:"idem_key"`
	RiskClass string `json:"risk_class"`
}

// ReleaseReq 确认释放请求。
type ReleaseReq struct {
	LotID string `json:"lot_id"`
	HoldID string `json:"hold_id"`
	ReleaseID string `json:"release_id"`
	Amount    string `json:"amount"`
	IdemKey string `json:"idem_key"`
}

// ReclassReq 升档作废请求。

type ReclassReq struct {
	LotID string `json:"lot_id"`
	HoldID string `json:"hold_id"`
	ReclassID string `json:"reclass_id"`
	Amount    string `json:"amount"`
	ToClass string `json:"to_class"`
	ReasonCode string `json:"reason_code"`
	IdemKey string `json:"idem_key"`
}
