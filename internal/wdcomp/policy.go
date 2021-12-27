package wdcomp

import (
	"sort"
	"strings"
	"time"
)

// AutoPolicy 一条失败码的自动策略。
type AutoPolicy struct {
	Code FailureCode `json:"code"`
	Version int `json:"version"`
	AutoAllowed bool `json:"auto_allowed"`
	ForceHuman  bool `json:"force_human"`
	DualConfirm bool `json:"dual_confirm"`
	TimeoutSec int `json:"timeout_sec"`
	Legs      []string  `json:"legs"`
	Note      string    `json:"note"`
	UpdatedBy string    `json:"updated_by,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// PolicyBook 进程内策略表。启动时装入默认行，人工更新只替换对应失败码。
type PolicyBook struct {
	rows map[FailureCode]AutoPolicy
}

   func DefaultPolicies() *PolicyBook {
	  b := &PolicyBook{rows: map[FailureCode]AutoPolicy{}}
	   for _, row := range defaultPolicyRows() {
		   b.rows[row.Code] = row
	  }
	   return b
  }

  func defaultLegs() []string {
	   return []string{
		   string(LegOccupancy), string(LegShard), string(LegUnfreeze), string(LegFee), string(LegSeal),
	   }
   }

   func defaultPolicyRows() []AutoPolicy {
	  note := map[FailureCode]string{
		   CodeSignFail:        "已签未广播且无哈希：解冻本金、释放占用和费预留。",
		   CodeBroadcastReject: "节点拒绝且无哈希：与签失败同一自动路径。",
		  CodeStuckAbandon: "卡单放弃且跟踪仍为 signed：允许自动回滚。",
		   CodePolicyReject: "策略在广播前拒绝：冻结尚未上链，允许退回。",
		   CodeHumanCancel:  "人工取消且无哈希：允许退回，仍走补偿分录。",
		  CodeShardPartial: "只编排失败分片；已成交金额由保护量钉住。",
		  CodeShardFail:    "整单分片失败且无成交、无哈希：全额可补偿部分退回。",
		  CodeFeeOrphan:    "本金已不在补偿范围，只释放残留费预留。",
		  CodeUnknownChain: "链上态不明，禁止自动解冻，只开事故。",
		   CodeLiveHash: "存在 live hash，禁止自动解冻，只开事故。",
	  }
	  auto := []FailureCode{
		  CodeSignFail, CodeBroadcastReject, CodeStuckAbandon, CodePolicyReject,
		   CodeHumanCancel, CodeShardPartial, CodeShardFail, CodeFeeOrphan,
	   }
	   rows := make([]AutoPolicy,
		  0, len(KnownCodes()))
	   for _, code := range auto {
		  rows = append(rows, AutoPolicy{
			   Code: code, Version: 1, AutoAllowed: true, ForceHuman: false, DualConfirm: false,
			   TimeoutSec: 900, Legs: defaultLegs(), Note: note[code],
		   })
	  }
	   rows = append(rows,
		   AutoPolicy{
			   Code: CodeUnknownChain, Version: 1, AutoAllowed: false, ForceHuman: true, DualConfirm: true,
			   TimeoutSec: 300, Legs: []string{string(LegAudit), string(LegIncident)}, Note: note[CodeUnknownChain],
		   },
		   AutoPolicy{
			  Code:    CodeLiveHash,
			   Version: 1, AutoAllowed: false, ForceHuman: true, DualConfirm: true,
			  TimeoutSec: 300, Legs: []string{string(LegAudit), string(LegIncident)}, Note: note[CodeLiveHash],
		  },
	   )
	   return rows
   }

  func (b *PolicyBook) Get(code FailureCode) (AutoPolicy, bool) {
	   if b == nil {
		   return AutoPolicy{}, false
	   }
	   row, ok := b.rows[code]
	  if !ok {
		   return AutoPolicy{}, false
	   }
	   row.Legs = append([]string(nil), row.Legs...)
	   return row, true
   }

  func (b *PolicyBook) List() []AutoPolicy {
	  if b == nil {
		  return nil
	   }
	  out := make([]AutoPolicy, 0, len(b.rows))
	  for _, row := range b.rows {
		  row.Legs = append([]string(nil), row.Legs...)
		   out = append(out, row)
	  }
	   sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	   return out
   }

// Put 替换一条策略。
// 功能：允许调整超时、双人确认和自动腿序。
// 不允许把 live hash 或不明链上态改成可自动解冻。
// 不变量：写入后的 AutoAllowed 在这两类码上仍为 false。
  func (b *PolicyBook) Put(in AutoPolicy) (AutoPolicy, error) {
	   if b == nil {
		  return AutoPolicy{}, ErrUnavailable
	  }
	   code := FailureCode(strings.TrimSpace(string(in.Code)))
	  cur, ok := b.Get(code)
	   if !ok {
		   return AutoPolicy{}, ErrPolicy
	   }
	   if (code == CodeLiveHash || code == CodeUnknownChain) && in.AutoAllowed {
		  return AutoPolicy{}, ErrPolicy
	   }
	  if code == CodeLiveHash || code == CodeUnknownChain {
		  in.AutoAllowed = false
		   in.ForceHuman = true
	   }
	  legs := in.Legs
	  if len(legs) == 0 {
		  legs = cur.Legs
	  }
	   clean := make([]string, 0, len(legs))
	  for _, leg := range legs {
		  k := LegKind(strings.TrimSpace(leg))
		  if !knownLeg(k) {
			  return AutoPolicy{}, ErrPolicy
		  }
		  clean = append(clean, string(k))
	   }
	  timeout := in.TimeoutSec
	  if timeout <= 0 {
		  timeout = cur.TimeoutSec
	  }
	  if timeout <= 0 {
		   timeout = 900
	   }
	   next := AutoPolicy{
		   Code: code, Version: cur.Version + 1, AutoAllowed: in.AutoAllowed, ForceHuman: in.ForceHuman,
		  DualConfirm: in.DualConfirm,
		   TimeoutSec:  timeout, Legs: clean, Note: strings.TrimSpace(in.Note),
		   UpdatedBy: strings.TrimSpace(in.UpdatedBy), UpdatedAt: in.UpdatedAt,
	  }
	  if next.Note == "" {
		  next.Note = cur.Note
	  }
	  if code == CodeLiveHash || code == CodeUnknownChain {
		   next.AutoAllowed = false
		   next.ForceHuman = true
	  }
	   b.rows[code] = next
	  cp := next
	  cp.Legs = append([]string(nil), next.Legs...)
	   return cp, nil
   }

   func knownLeg(k LegKind) bool {
	   switch k {
	  case LegOccupancy, LegShard, LegUnfreeze, LegFee, LegSeal, LegAudit, LegIncident:
		   return true
	  default:
		   return false
	   }
  }

type legSpec struct {
	Kind   LegKind
	Amount string
}

// assembleLegs 按策略腿序过滤零金额腿。自动路径始终以封账腿收尾。
   func assembleLegs(d Decision, template []string) []legSpec {
	  if !d.MayAuto {
		   return []legSpec{
			   {Kind: LegUnfreeze, Amount: d.Compensable},
			  {Kind: LegFee, Amount: d.Fee},
			  {Kind: LegOccupancy, Amount: d.OccRelease},
			  {Kind: LegShard, Amount: d.Compensable},
			  {Kind: LegAudit, Amount: "0"},
			  {Kind: LegIncident, Amount: "0"},
		  }
	  }
	  if len(template) == 0 {
		   template = defaultLegs()
	   }
	   var out []legSpec
	   seenSeal := false
	  for _, raw := range template {
		  k := LegKind(raw)
		   switch k {
		   case LegOccupancy:
			   if positive(d.OccRelease) {
				   out = append(out, legSpec{Kind: k, Amount: d.OccRelease})
			   }
		  case LegShard:
			   if d.Code == CodeShardPartial && positive(d.Compensable) {
				   out = append(out,
					  legSpec{Kind: k, Amount: d.Compensable})
			  }
		  case LegUnfreeze:
			  if positive(d.Compensable) {
				  out = append(out, legSpec{Kind: k, Amount: d.Compensable})
			   }
		   case LegFee:
			   if positive(d.Fee) {
				   out = append(out, legSpec{Kind: k, Amount: d.Fee})
			  }
		   case LegSeal:
			  out = append(out, legSpec{Kind: k, Amount: "0"})
			  seenSeal = true
		   case LegAudit, LegIncident:
			  continue
		   }
	  }
	  if !seenSeal {
		  out = append(out, legSpec{Kind: LegSeal, Amount: "0"})
	   }
	   return out
   }

   func positive(s string) bool {
	   a, err := parseOpt(s)
	  return err == nil && a.IsPositive()
   }
