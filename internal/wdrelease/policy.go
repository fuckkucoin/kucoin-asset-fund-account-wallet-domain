package wdrelease

import (
	"fmt"
	"sort"
	"time"
	"github.com/shopspring/decimal"
)

// Band 一档金额。Max 空表示无上限。Ratios 为万分比，和必须是 10000。

type Band struct {
	Asset string `json:"asset"`
	Min   string `json:"min"`
	Max   string `json:"max,omitempty"`
	Depth int    `json:"depth"`
	Ratios  []int  `json:"ratios_bps"`
	SLASec  int64  `json:"sla_sec"`
	Timeout string `json:"timeout_action"`
	Skip       bool   `json:"skip"`
	SegmentCap string `json:"segment_cap,omitempty"`
	DailyCap   string `json:"daily_cap,omitempty"`
	DestCap    string `json:"dest_cap,omitempty"`
}

// RiskRule 风险分修正。按 MinScore 升序应用，命中即叠加。
type RiskRule struct {
	MinScore int `json:"min_score"`
	AddDepth    int  `json:"add_depth,omitempty"`
	MinDepth    int  `json:"min_depth,omitempty"`
	DisableSkip bool `json:"disable_skip,omitempty"`
	Deny bool   `json:"deny,omitempty"`
	Code string `json:"code,omitempty"`
}

// DestRule 目的地类别修正。未知类别失败关闭。
type DestRule struct {
	Class string `json:"class"`
	AddDepth int `json:"add_depth,omitempty"`
	MinDepth    int  `json:"min_depth,omitempty"`
	DisableSkip bool `json:"disable_skip,omitempty"`
	Deny        bool `json:"deny,omitempty"`
	Code       string `json:"code,omitempty"`
	SLAPercent int    `json:"sla_percent,omitempty"`
}

// Version 一份不可变策略。Effective 之后才能被时钟选中。

type Version struct {
	ID        string    `json:"id"`
	Effective time.Time `json:"effective"`
	Supersedes string `json:"supersedes,omitempty"`
	MaxDepth   int    `json:"max_depth"`
	EscalateExtraSec int64 `json:"escalate_extra_sec"`
	SamePersonHard bool `json:"same_person_hard"`
	MaxOpen int        `json:"max_open"`
	Bands   []Band     `json:"bands"`
	Risk    []RiskRule `json:"risk"`
	Dest    []DestRule `json:"dest"`
}

// PolicyBook 进程内代次表。
type PolicyBook struct {
	vers map[string]*Version
	ord []string
}

  func newBook(vs []Version) (*PolicyBook, error) {
	   b := &PolicyBook{vers: map[string]*Version{}}
	  for i := range vs {
		  cp := vs[i]
		  if err := validateVersion(&cp); err != nil {
			   return nil, err
		  }
		  if _, ok := b.vers[cp.ID]; ok {
			   return nil, fmt.Errorf("%w: duplicate %s", ErrPolicy, cp.ID)
		   }
		   b.vers[cp.ID] = &cp
		  b.ord = append(b.ord, cp.ID)
	  }
	   sort.Slice(b.ord, func(i, j int) bool {
		   return b.vers[b.ord[i]].Effective.Before(b.vers[b.ord[j]].Effective)
	  })
	   return b, nil
  }

  func (b *PolicyBook) Get(id string) (Version, bool) {
	   if b == nil {
		   return Version{}, false
	   }
	   v, ok := b.vers[id]
	   if !ok || v == nil {
		  return Version{}, false
	   }
	  return *v, true
  }

  func (b *PolicyBook) IDs() []string {
	   if b == nil {
		   return nil
	  }
	   out := append([]string(nil), b.ord...)
	  return out
  }

// EffectiveID 取生效时间不晚于 now 的最新代次。没有则空。

  func (b *PolicyBook) EffectiveID(now time.Time) string {
	  if b == nil {
		  return ""
	   }
	  var pick string
	  var at time.Time
	  for _, id := range b.ord {
		   v := b.vers[id]
		   if v.Effective.After(now) {
			   continue
		   }
		   if pick == "" || !v.Effective.Before(at) {
			  pick = id
			   at = v.Effective
		   }
	   }
	  return pick
   }

   func (b *PolicyBook) install(v Version) error {
	  if err := validateVersion(&v); err != nil {
		   return err
	  }
	   if _, ok := b.vers[v.ID]; ok {
		  return WithCode(ErrVersion, CodeVersion)
	   }
	  b.vers[v.ID] = &v
	   b.ord = append(b.ord, v.ID)
	  sort.Slice(b.ord, func(i, j int) bool {
		  return b.vers[b.ord[i]].Effective.Before(b.vers[b.ord[j]].Effective)
	   })
	  return nil
  }

  func validateVersion(v *Version) error {
	   if v == nil || v.ID == "" {
		  return WithCode(ErrPolicy, CodePolicy)
	   }
	   if v.MaxDepth < 1 || v.MaxDepth > 4 {
		  return WithCode(ErrPolicy, CodePolicy)
	  }
	   if v.EscalateExtraSec < 0 {
		   return WithCode(ErrPolicy, CodePolicy)
	   }
	   if v.MaxOpen <= 0 {
		  v.MaxOpen = 20
	  }
	  if len(v.Bands) == 0 {
		  return WithCode(ErrPolicy, CodePolicy)
	   }
	  byAsset := map[string][]Band{}
	  for _, band := range v.Bands {
		  if err := validateBand(band); err != nil {
			  return err
		  }
		  byAsset[band.Asset] = append(byAsset[band.Asset], band)
	  }
	  if _, ok := byAsset["*"]; !ok {
		   return fmt.Errorf("%w: missing default asset", ErrPolicy)
	  }
	   for asset, rows := range byAsset {
		  if err := contiguous(asset, rows); err != nil {
			  return err
		   }
	  }
	   if len(v.Dest) == 0 {
		  return WithCode(ErrPolicy, CodePolicy)
	  }
	  seen := map[string]bool{}
	   for _, d := range v.Dest {
		   if d.Class == "" || seen[d.Class] {
			  return WithCode(ErrPolicy, CodePolicy)
		   }
		   seen[d.Class] = true
		  if d.Deny && d.Code == "" {
			  return WithCode(ErrPolicy, CodePolicy)
		   }
		  if d.SLAPercent < 0 {
			  return WithCode(ErrPolicy,
				   CodePolicy)
		   }
	  }
	  if !seen[ClassExternal] {
		   return fmt.Errorf("%w: dest external required", ErrPolicy)
	   }
	   for _, r := range v.Risk {
		  if r.MinScore < 0 || r.MinScore > 100 {
			  return WithCode(ErrPolicy, CodePolicy)
		  }
		   if r.Deny && r.Code == "" {
			  return WithCode(ErrPolicy, CodePolicy)
		   }
	   }
	  return nil
  }

  func validateBand(b Band) error {
	  if b.Asset == "" {
		  return WithCode(ErrPolicy, CodePolicy)
	   }
	   min,
		   err := decimal.NewFromString(b.Min)
	   if err != nil || min.IsNegative() {
		  return WithCode(ErrPolicy, CodePolicy)
	  }
	  if b.Max != "" {
		   max, err := decimal.NewFromString(b.Max)
		  if err != nil || !max.GreaterThan(min) {
			   return WithCode(ErrPolicy, CodePolicy)
		   }
	  }
	   if b.Depth < 0 || (b.Skip && b.Depth != 0) || (!b.Skip && b.Depth < 1) {
		   return WithCode(ErrPolicy,
			  CodePolicy)
	  }
	   if b.Skip && (b.SLASec != 0 || b.Timeout != TimeoutNone) {
		   return WithCode(ErrPolicy, CodePolicy)
	   }
	   switch b.Timeout {
	   case TimeoutNone, TimeoutEscalate, TimeoutReject:
	  default:
		  return WithCode(ErrPolicy, CodePolicy)
	   }
	   if b.SLASec < 0 {
		   return WithCode(ErrPolicy, CodePolicy)
	  }
	   if !b.Skip && b.SLASec <= 0 {
		   return WithCode(ErrPolicy, CodePolicy)
	  }
	  if err := checkRatios(b.Ratios, b.Depth); err != nil {
		  return err
	   }
	  if b.SegmentCap != "" {
		  cap, err := decimal.NewFromString(b.SegmentCap)
		   if err != nil || !cap.IsPositive() {
			  return WithCode(ErrPolicy, CodePolicy)
		  }
	   }
	   return nil
   }

  func checkRatios(ratios []int, depth int) error {
	   if depth == 0 {
		  if len(ratios) != 1 || ratios[0] != 10000 {
			   return WithCode(ErrPolicy, CodePolicy)
		   }
		  return nil
	  }
	  if len(ratios) != depth {
		   return WithCode(ErrPolicy, CodePolicy)
	  }
	   sum := 0
	   for _, r := range ratios {
		   if r <= 0 {
			   return WithCode(ErrPolicy,
				   CodePolicy)
		  }
		   sum += r
	   }
	  if sum != 10000 {
		  return WithCode(ErrPolicy, CodePolicy)
	   }
	   return nil
   }

  func contiguous(asset string, rows []Band) error {
	  sort.Slice(rows, func(i, j int) bool {
		   ai, _ := decimal.NewFromString(rows[i].Min)
		  aj, _ := decimal.NewFromString(rows[j].Min)
		   return ai.LessThan(aj)
	  })
	  if rows[0].Min != "0" {
		   return fmt.Errorf("%w: %s must start at 0", ErrPolicy, asset)
	   }
	  for i := 0; i < len(rows); i++ {
		  if i == len(rows)-1 {
			  if rows[i].Max != "" {
				   return fmt.Errorf("%w: %s open end", ErrPolicy, asset)
			   }
			  break
		  }
		  if rows[i].Max == "" || rows[i].Max != rows[i+1].Min {
			   return fmt.Errorf("%w: %s gap", ErrPolicy, asset)
		   }
	  }
	  return nil
  }

  func matchBand(v Version, asset string, amt decimal.Decimal) (Band, bool) {
	  var rows []Band
	   for _, b := range v.Bands {
		  if b.Asset == asset {
			   rows = append(rows, b)
		  }
	   }
	  if len(rows) == 0 {
		  for _, b := range v.Bands {
			  if b.Asset == "*" {
				  rows = append(rows, b)
			   }
		   }
	   }
	  for _, b := range rows {
		   min, err := decimal.NewFromString(b.Min)
		   if err != nil || amt.LessThan(min) {
			   continue
		  }
		  if b.Max != "" {
			   max, err := decimal.NewFromString(b.Max)
			  if err != nil || !amt.LessThan(max) {
				   continue
			   }
		  }
		  return b, true
	   }
	   return Band{}, false
  }
