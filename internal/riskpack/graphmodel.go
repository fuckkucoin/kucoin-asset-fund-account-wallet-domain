package riskpack

import (
	"strings"
	"time"
	"github.com/shopspring/decimal"
)

const (
	TierLow    = "low"
	TierMid    = "mid"
	TierHigh   = "high"
	TierSevere = "severe"
	EdgeExplicit = "explicit"
	EdgeFamily = "family"
	EdgeKYT    = "kyt"
	EdgeCoSpend = "co_spend"
	// 连通阈默认值：边权不低于该值才并入同一簇。
	DefaultClusterThreshold = "0.35"
	// 边权低于该地板则标记失效，不再参与连通。
	DefaultWeightFloor = "0.01"
	AuditEdgeLink  = "edge_link"
	AuditEdgeDecay = "edge_decay"
	AuditCluster    = "cluster_recompute"
	AuditKYTFeature = "kyt_feature"
	AuditNodeUpsert = "node_upsert"
)

// DestNode 目的地节点。ClusterID 以最近一次重算为准。
type DestNode struct {
	Key     string `json:"key"`
	Chain   string `json:"chain"`
	Address string `json:"address"`
	ClusterID string `json:"cluster_id"`
	Tier      string `json:"tier"`
	Score   string `json:"score"`
	KYTHint string `json:"kyt_hint,omitempty"`
	Label     string    `json:"label,omitempty"`
	Version   int64     `json:"version"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen time.Time `json:"last_seen"`
}

// DestEdge 无向边。AnchorWeight 是衰减锚，Weight 是当前权。

type DestEdge struct {
	ID       string `json:"id"`
	LeftKey  string `json:"left_key"`
	RightKey string `json:"right_key"`
	Kind string `json:"kind"`
	Evidence string `json:"evidence,omitempty"`
	AnchorWeight string `json:"anchor_weight"`
	Weight       string `json:"weight"`
	HalfLifeSec int64     `json:"half_life_sec"`
	Active      bool      `json:"active"`
	BornAt      time.Time `json:"born_at"`
	AnchorAt  time.Time `json:"anchor_at"`
	TouchedAt time.Time `json:"touched_at"`
}

// ClusterExposure 簇敞口快照。Reserved/Consumed 由限额账回写。
type ClusterExposure struct {
	ClusterID string `json:"cluster_id"`
	Asset     string `json:"asset,omitempty"`
	MemberN   int    `json:"member_n"`
	Score     string `json:"score"`
	Tier      string `json:"tier"`
	MaxWeight string `json:"max_weight"`
	Reserved   string    `json:"reserved"`
	Consumed   string    `json:"consumed"`
	ComputedAt time.Time `json:"computed_at"`
	Members []string `json:"members,omitempty"`
}

// GraphAudit 图谱演化审计。衰减与重算必须留下前后值。

type GraphAudit struct {
	ID string `json:"id"`
	Kind    string `json:"kind"`
	Subject string `json:"subject"`
	Before string `json:"before"`
	After string `json:"after"`
	Detail string    `json:"detail,omitempty"`
	At     time.Time `json:"at"`
}

   func pairKey(a, b string) string {
	   if b < a {
		   a, b = b, a
	  }
	  return a + "||" + b
   }

  func parseWeight(s string) (decimal.Decimal, error) {
	   d, err := decimal.NewFromString(strings.TrimSpace(s))
	   if err != nil || !d.IsPositive() || d.GreaterThan(decimal.NewFromInt(1)) {
		  return decimal.Zero, ErrDest
	   }
	   return d, nil
  }

   func parseNonNeg(s string) decimal.Decimal {
	  d, err := decimal.NewFromString(strings.TrimSpace(s))
	  if err != nil || d.IsNegative() {
		   return decimal.Zero
	  }
	  return d
   }

// TierOfScore 把 0–100 分档映射到限额乘数档。
// 分界含端点：75 严重、50 高、25 中，其余低。
   func TierOfScore(score decimal.Decimal) string {
	   if score.GreaterThanOrEqual(decimal.NewFromInt(75)) {
		   return TierSevere
	   }
	   if score.GreaterThanOrEqual(decimal.NewFromInt(50)) {
		  return TierHigh
	  }
	  if score.GreaterThanOrEqual(decimal.NewFromInt(25)) {
		   return TierMid
	   }
	  return TierLow
  }

// WeightAt 按半衰期整数次折半。未满一个半衰期保持锚权，避免未达阈就放宽。
  func WeightAt(anchor decimal.Decimal, anchorAt, now time.Time, halfLifeSec int64) decimal.Decimal {
	  if halfLifeSec <= 0 || !now.After(anchorAt) {
		  return anchor
	  }
	  elapsed := now.Sub(anchorAt)
	  half := time.Duration(halfLifeSec) * time.Second
	   if half <= 0 || elapsed < half {
		  return anchor
	  }
	   n := int(elapsed / half)
	  if n < 0 {
		   return anchor
	   }
	  if n > 32 {
		  n = 32
	  }
	  w := anchor
	   two := decimal.NewFromInt(2)
	   for i := 0; i < n; i++ {
		  w = w.Div(two)
	   }
	  return w
   }

  func kytBump(hint string) int64 {
	   switch strings.ToLower(strings.TrimSpace(hint)) {
	   case "deny":
		  return 30
	  case "review":
		  return 10
	   default:
		   return 0
	   }
   }

   func capScore(v decimal.Decimal) decimal.Decimal {
	   if v.IsNegative() {
		  return decimal.Zero
	  }
	  hundred := decimal.NewFromInt(100)
	  if v.GreaterThan(hundred) {
		   return hundred
	  }
	   return v
  }
