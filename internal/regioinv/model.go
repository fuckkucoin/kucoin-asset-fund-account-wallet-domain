package regioinv

import (
	"strings"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const (
	TierHot = "hot"
	TierWarm = "warm"
	TierCold = "cold"
	FWOpen = "open"
	FWDegraded = "degraded"
	FWBlocked = "blocked"
	RolePrimary = "primary"
	RoleStandby = "standby"
	RoleDC      = "dc"
	BreachBelow = "below_low"
	BreachAbove = "above_high"
	BreachDrift = "drift"
)

// Region 是库存域里的一个机房。CatalogCode 只读绑定 mwtopo 的 sg/tyo。

type Region struct {
	ID   string `json:"id"`
	Code string `json:"code"`
	CatalogCode string `json:"catalog_code"`
	Name        string `json:"name"`
	Role string `json:"role"`
	Firewall       string    `json:"firewall"`
	FirewallReason string    `json:"firewall_reason,omitempty"`
	Halted         bool      `json:"halted"`
	HaltActor      string    `json:"halt_actor,omitempty"`
	HaltReason     string    `json:"halt_reason,omitempty"`
	HaltAt         time.Time `json:"halt_at,omitempty"`
	Ready          bool      `json:"ready"`
	ReadyDetail string    `json:"ready_detail,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

   func (r Region) clone() Region { return r }

// RegionInv 是 region×tier×asset×chain 的库存快照。Observed = Available + Frozen。

type RegionInv struct {
	ID       string `json:"region_inv_id"`
	RegionID string `json:"region_id"`
	Tier     string `json:"tier"`
	Asset string `json:"asset"`
	Chain     string `json:"chain"`
	Available string `json:"available"`
	Frozen string `json:"frozen"`
	Observed string `json:"observed"`
	Seq int64 `json:"seq"`
	Source string `json:"source"`
	ObservedAt time.Time `json:"observed_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

   func (r RegionInv) clone() RegionInv { return r }

// Watermark 是单区单层的水位与配额。窗口起止相等表示全天。

type Watermark struct {
	ID       string `json:"id"`
	RegionID string `json:"region_id"`
	Tier     string `json:"tier"`
	Asset    string `json:"asset"`
	Chain    string `json:"chain"`
	Low      string `json:"low"`
	Target string `json:"target"`
	High     string `json:"high"`
	MinLeg   string `json:"min_leg"`
	MaxLeg   string `json:"max_leg"`
	DailyCap string `json:"daily_cap"`
	Used     string `json:"used"`
	UsedDay string `json:"used_day"`
	WindowStart int `json:"window_start"`
	WindowEnd int  `json:"window_end"`
	MaxLegs   int  `json:"max_legs"`
	Enabled   bool `json:"enabled"`
	Actor     string    `json:"actor"`
	UpdatedAt time.Time `json:"updated_at"`
}

  func (w Watermark) clone() Watermark { return w }

// IsoPolicy 是区级隔离策略。FailTo 只在探测失败时使用，不代替人工 halt。
type IsoPolicy struct {
	ID string `json:"id"`
	RegionID      string `json:"region_id"`
	BlockCrossHot bool   `json:"block_cross_hot"`
	BlockWrongRebal bool   `json:"block_wrong_rebal"`
	DriftBPS        int    `json:"drift_bps"`
	FailTo          string `json:"fail_to"`
	Actor string `json:"actor"`
	UpdatedAt time.Time `json:"updated_at"`
}

  func (p IsoPolicy) clone() IsoPolicy { return p }

// Observation 是某一来源的读数。汇聚时同来源取最新，不同来源相加。
type Observation struct {
	ID       string `json:"id"`
	RegionID string `json:"region_id"`
	Tier     string `json:"tier"`
	Asset    string `json:"asset"`
	Chain    string `json:"chain"`
	Source     string    `json:"source"`
	Available  string    `json:"available"`
	Frozen     string    `json:"frozen"`
	Seq        int64     `json:"seq"`
	ObservedAt time.Time `json:"observed_at"`
}

  func (o Observation) clone() Observation { return o }

// Transfer 在两个区的同一层之间搬可用。冻结不动。
type Transfer struct {
	SrcRegionID string
	DstRegionID string
	Tier        string
	Asset  string
	Chain  string
	Amount string
}

// HotWithdraw 是从某区热库存取出的出金意图。防火墙只判定，不广播。
type HotWithdraw struct {
	RegionCode string `json:"region_code"`
	RegionID   string `json:"region_id"`
	Asset      string `json:"asset"`
	Chain  string `json:"chain"`
	Amount string `json:"amount"`
	Actor string `json:"actor"`
	CrossRegion bool   `json:"cross_region"`
	PeerCode    string `json:"peer_code"`
}

// RegisterReq 登记一个库存区。CatalogCode 必须落在 sg/tyo。
type RegisterReq struct {
	Code        string `json:"code"`
	CatalogCode string `json:"catalog_code"`
	Name        string `json:"name"`
	Role string `json:"role"`
	Actor string `json:"actor"`
}

// ObserveReq 写入一个来源读数并立刻汇聚快照。

type ObserveReq struct {
	RegionCode string `json:"region_code"`
	RegionID   string `json:"region_id"`
	Tier       string `json:"tier"`
	Asset string `json:"asset"`
	Chain     string `json:"chain"`
	Source    string `json:"source"`
	Available string `json:"available"`
	Frozen string `json:"frozen"`
}

// MarkReq 写入水位。Low ≤ Target ≤ High。
type MarkReq struct {
	RegionCode string `json:"region_code"`
	RegionID   string `json:"region_id"`
	Tier string `json:"tier"`
	Asset  string `json:"asset"`
	Chain  string `json:"chain"`
	Low    string `json:"low"`
	Target string `json:"target"`
	High   string `json:"high"`
	MinLeg string `json:"min_leg"`
	MaxLeg      string `json:"max_leg"`
	DailyCap    string `json:"daily_cap"`
	WindowStart int    `json:"window_start"`
	WindowEnd   int    `json:"window_end"`
	MaxLegs     int    `json:"max_legs"`
	Enabled *bool `json:"enabled"`
	Actor string `json:"actor"`
}

// PolicyReq 写入隔离策略。

type PolicyReq struct {
	RegionCode      string `json:"region_code"`
	RegionID        string `json:"region_id"`
	BlockCrossHot   *bool  `json:"block_cross_hot"`
	BlockWrongRebal *bool  `json:"block_wrong_rebal"`
	DriftBPS        int    `json:"drift_bps"`
	FailTo string `json:"fail_to"`
	Actor  string `json:"actor"`
}

// ProbeReport 是区故障探测结论。Ready=false 按策略把防火墙打到 degraded 或 blocked。

type ProbeReport struct {
	RegionCode string `json:"region_code"`
	Ready      bool   `json:"ready"`
	Detail string `json:"detail"`
}

// Breach 是水位巡检的一条越界。
type Breach struct {
	RegionID string `json:"region_id"`
	RegionCode string `json:"region_code"`
	Tier       string `json:"tier"`
	Asset      string `json:"asset"`
	Chain      string `json:"chain"`
	Kind       string `json:"kind"`
	Available  string `json:"available"`
	Bound    string `json:"bound"`
	DriftBPS int    `json:"drift_bps"`
}

// BoardRow 是水位看板的一行。
type BoardRow struct {
	RegionCode string `json:"region_code"`
	Tier string `json:"tier"`
	Asset string `json:"asset"`
	Chain string `json:"chain"`
	Available string `json:"available"`
	Frozen    string `json:"frozen"`
	Low      string `json:"low"`
	Target   string `json:"target"`
	High     string `json:"high"`
	Firewall string `json:"firewall"`
	Halted   bool   `json:"halted"`
	Breach   string `json:"breach,omitempty"`
	DriftBPS int    `json:"drift_bps"`
}

// Board 是全部区的水位看板。
type Board struct {
	Rows []BoardRow `json:"rows"`
	At   time.Time  `json:"at"`
}

// MoveQuote 是一次跨区搬运的报价。Amount 已按水位裁剪，尚未落计划。
type MoveQuote struct {
	Amount string
	Src Region
	Dst Region
	SrcInv RegionInv
	DstInv  RegionInv
	SrcMark Watermark
	DstMark Watermark
}

// ClearResult 是区级 Clear 的结果。CustodyHaltUntouched 恒为 true：本路径不解除全局停提。
type ClearResult struct {
	Region               Region `json:"region"`
	CustodyHaltUntouched bool   `json:"custody_halt_untouched"`
}

  func newID(prefix string) string {
	  return prefix + strings.ReplaceAll(uuid.NewString(), "-", "")
  }

  func NormTier(s string) (string, error) {
	   switch strings.ToLower(strings.TrimSpace(s)) {
	  case TierHot, "h":
		  return TierHot, nil
	   case TierWarm, "w":
		  return TierWarm, nil
	   case TierCold, "c":
		  return TierCold, nil
	  default:
		   return "", ErrInvalid
	   }
  }

  func NormFirewall(s string) (string, error) {
	   switch strings.ToLower(strings.TrimSpace(s)) {
	  case FWOpen, "":
		  return FWOpen, nil
	  case FWDegraded:
		   return FWDegraded, nil
	  case FWBlocked:
		   return FWBlocked, nil
	   default:
		  return "", ErrInvalid
	   }
  }

  func bookOf(s string) (money.Amount, error) {
	   s = strings.TrimSpace(s)
	  if s == "" {
		   s = "0"
	   }
	  return money.ParseBook(s)
  }

   func mustBook(s string) money.Amount {
	   a, err := bookOf(s)
	   if err != nil {
		   return money.Zero()
	   }
	  return a
  }

// InWindow 起止小时相等表示全天。跨日窗口用 start > end 表示。
  func InWindow(start, end int, at time.Time) bool {
	  if start == end {
		   return true
	   }
	  if start < 0 || start > 23 || end < 0 || end > 24 {
		   return false
	  }
	   h := at.UTC().Hour()
	   if start < end {
		  return h >= start && h < end
	  }
	   return h >= start || h < end
  }

   func minAmt(a, b money.Amount) money.Amount {
	  if a.Cmp(b) <= 0 {
		   return a
	   }
	  return b
   }

// Conserved 要求源减少量 = 目标增加量 = amount。
   func Conserved(srcBefore, srcAfter, dstBefore, dstAfter, amount string) error {
	  sb, err := bookOf(srcBefore)
	  if err != nil {
		  return ErrConserve
	   }
	   sa, err := bookOf(srcAfter)
	   if err != nil {
		  return ErrConserve
	   }
	   db, err := bookOf(dstBefore)
	  if err != nil {
		  return ErrConserve
	  }
	  da, err := bookOf(dstAfter)
	  if err != nil {
		   return ErrConserve
	  }
	  amt, err := money.Parse(amount)
	  if err != nil {
		   return ErrConserve
	  }
	  drop, err := sb.Sub(sa)
	   if err != nil || !drop.Equal(amt) {
		  return ErrConserve
	   }
	   gain, err := da.Sub(db)
	  if err != nil || !gain.Equal(amt) {
		   return ErrConserve
	   }
	  return nil
   }

// NextPair 计算划转后的两行快照。可用不足或守恒失败时不改入参。

   func NextPair(src, dst RegionInv, amount string, now time.Time) (RegionInv, RegionInv, error) {
	   amt, err := money.Parse(amount)
	   if err != nil {
		  return RegionInv{}, RegionInv{}, ErrInvalid
	   }
	  sa, err := bookOf(src.Available)
	  if err != nil {
		  return RegionInv{}, RegionInv{}, err
	  }
	   da, err := bookOf(dst.Available)
	   if err != nil {
		  return RegionInv{}, RegionInv{}, err
	  }
	   sf, err := bookOf(src.Frozen)
	   if err != nil {
		  return RegionInv{}, RegionInv{}, err
	   }
	   df, err := bookOf(dst.Frozen)
	   if err != nil {
		  return RegionInv{}, RegionInv{}, err
	   }
	   ns, err := sa.Sub(amt)
	  if err != nil {
		  return RegionInv{}, RegionInv{}, ErrInsufficient
	   }
	   nd, err := da.Add(amt)
	  if err != nil {
		   return RegionInv{}, RegionInv{}, err
	   }
	  so, err := ns.Add(sf)
	   if err != nil {
		   return RegionInv{}, RegionInv{}, err
	  }
	   do, err := nd.Add(df)
	   if err != nil {
		   return RegionInv{}, RegionInv{}, err
	  }
	   outS := src.clone()
	  outD := dst.clone()
	  outS.Available = ns.String()
	   outS.Observed = so.String()
	   outS.Seq++
	  outS.UpdatedAt = now
	  outD.Available = nd.String()
	   outD.Observed = do.String()
	  outD.Seq++
	  outD.UpdatedAt = now
	  if err := Conserved(sa.String(), outS.Available, da.String(), outD.Available, amt.String()); err != nil {
		  return RegionInv{}, RegionInv{}, err
	  }
	   return outS, outD, nil
   }

// DriftBPS 相对目标的偏离，单位万分比。目标为 0 且可用非 0 时记 10000。
  func DriftBPS(available, target string) (int, error) {
	   av, err := bookOf(available)
	   if err != nil {
		  return 0, err
	  }
	  tg, err := bookOf(target)
	   if err != nil {
		  return 0, err
	   }
	  if tg.IsZero() {
		  if av.IsZero() {
			   return 0, nil
		   }
		   return 10000, nil
	   }
	  diff := av.Dec().Sub(tg.Dec()).Abs()
	  bps := diff.Mul(decimal.NewFromInt(10000)).Div(tg.Dec())
	   if bps.GreaterThan(decimal.NewFromInt(1_000_000)) {
		  return 1_000_000, nil
	  }
	  return int(bps.IntPart()), nil
   }

  func dayOf(t time.Time) string {
	  if t.IsZero() {
		  t = time.Now().UTC()
	  }
	  return t.UTC().Format("2006-01-02")
   }

   func fmtTS(t time.Time) string {
	  if t.IsZero() {
		  return ""
	  }
	   return t.UTC().Format("2006-01-02 15:04:05.000")
  }

   func parseTS(s string) time.Time {
	  s = strings.TrimSpace(s)
	  if s == "" {
		   return time.Time{}
	  }
	  for _, layout := range []string{
		  "2006-01-02 15:04:05.000",
		   "2006-01-02 15:04:05",
		  time.RFC3339Nano,
		   time.RFC3339,
	  } {
		  if t, err := time.Parse(layout, s); err == nil {
			   return t.UTC()
		  }
	   }
	   return time.Time{}
  }

  func b2i(b bool) int {
	   if b {
		   return 1
	  }
	   return 0
  }

  func i2b(n int) bool { return n != 0 }
