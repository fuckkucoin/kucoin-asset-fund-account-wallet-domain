package invdrift

import (
	"context"
	"sort"
	"strings"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/custody"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

// ThresholdConfig 偏离阈值配置（按资产键，不含资产名时回退 default）
type ThresholdConfig struct {
	// WarnPct: 偏离百分比达到此值升为 warn（0~100 的字符串，如 "1.5"）
	WarnPct map[string]string
	// CritPct: 偏离百分比达到此值升为 crit
	CritPct map[string]string
	// WarnAbs: 绝对偏离额超过此值也升为 warn（与 WarnPct 取更严格的一侧）
	WarnAbs map[string]string
	// CritAbs: 绝对偏离额超过此值也升为 crit
	CritAbs map[string]string
}

// DefaultThresholds 默认阈值：主流资产超 1% 或 5000 USDT 等值升 warn，5% 或 50000 升 crit。
var DefaultThresholds = ThresholdConfig{
	WarnPct: map[string]string{"default": "1"},
	CritPct: map[string]string{"default": "5"},
	WarnAbs: map[string]string{"default": "5000", "BTC": "0.1", "ETH": "2"},
	CritAbs: map[string]string{"default": "50000", "BTC": "1", "ETH": "20"},
}

// LedgerReader 最小账本读接口：invdrift 只需要 ListUserBooks。
// 由 ledger.Store 和 ledger.MemStore 自动满足。
type LedgerReader interface {
	ListUserBooks(ctx context.Context, userID string) ([]ledger.Account, error)
}

type ledgerPair struct {
	hot, warm, cold decimal.Decimal
}

// LedgerSnapshot 用于 DriftProof 序列化的账本快照

type LedgerSnapshot struct {
	Asset string `json:"asset"`
	Hot   string `json:"hot"`
	Warm  string `json:"warm"`
	Cold  string `json:"cold"`
}

// CustodySnapshot 托管侧快照
type CustodySnapshot struct {
	Asset string `json:"asset"`
	Hot   string `json:"hot"`
	Cold  string `json:"cold"`
}

// HotInvSnapshot 热账侧快照
type HotInvSnapshot struct {
	Asset string `json:"asset"`
	Hot string `json:"hot"`
	Cold   string `json:"cold"`
	HotMax string `json:"hot_max,omitempty"`
	Excess string `json:"excess,omitempty"`
}

// SnapBundle 单资产三侧快照捆绑
type SnapBundle struct {
	Asset   string
	Ledger  LedgerSnapshot
	Custody CustodySnapshot
	HotInv  HotInvSnapshot
}

// ScanInput 扫描管线的输入（已从各侧读取完毕的原始数据）

type ScanInput struct {
	LedgerRows []ledger.Account
	CustRows   []custody.InventoryRow
	InvSnaps   []hotinv.Snapshot
	DataCenter string
	ThreshCfg ThresholdConfig
}

// ScanOutput 扫描管线输出
type ScanOutput struct {
	Items   []DriftItem
	Bundles []SnapBundle // 供 DriftProof 使用
}

// RunScan 执行增量偏离扫描，返回分级后的 DriftItem 列表与快照捆绑。
// 本函数纯计算，不访问数据库。

  func RunScan(batchID string, in ScanInput, clock func() time.Time) ScanOutput {
	   if clock == nil {
		   clock = func() time.Time { return time.Now().UTC() }
	}
	now := clock()
	// 聚合账本侧三科目
	ledBy := map[string]ledgerPair{}
	for _, a := range in.LedgerRows {
		as := strings.ToUpper(strings.TrimSpace(a.Asset))
		cur := ledBy[as]
		switch a.AccountType {
		case ledger.AccountBridge:
			cur.hot = a.Available
		case ledger.AccountWarm:
			cur.warm = a.Available
		case ledger.AccountCold:
			cur.cold = a.Available
		default:
			continue
		}
		ledBy[as] = cur
	}
	// 聚合托管侧
	custHot := map[string]decimal.Decimal{}
	custCold := map[string]decimal.Decimal{}
	for _, r := range in.CustRows {
		as := strings.ToUpper(strings.TrimSpace(r.Asset))
		v, err := decimal.NewFromString(orZero(r.Available))
		if err != nil {
			continue
		}
		switch r.Side {
		case "hot":
			custHot[as] = custHot[as].Add(v)
		case "cold":
			custCold[as] = custCold[as].Add(v)
		}
	}
	// 聚合热账侧
	invHot := map[string]decimal.Decimal{}
	invCold := map[string]decimal.Decimal{}
	invHotMax := map[string]string{}
	invExcess := map[string]string{}
	for _, s := range in.InvSnaps {
		as := strings.ToUpper(strings.TrimSpace(s.Asset))
		h,
			_ := decimal.NewFromString(orZero(s.HotAvail))
		c, _ := decimal.NewFromString(orZero(s.ColdAvail))
		invHot[as] = invHot[as].Add(h)
		invCold[as] = invCold[as].Add(c)
		invHotMax[as] = s.HotMax
		invExcess[as] = s.Excess
	}
	// 收集全部资产并排序（幂等输出顺序）
	assetSet := map[string]struct{}{}
	for k := range ledBy {
		assetSet[k] = struct{}{}
	}
	for k := range custHot {
		assetSet[k] = struct{}{}
	}
	for k := range invHot {
		assetSet[k] = struct{}{}
	}
	assets := make([]string,
		0, len(assetSet))
	for k := range assetSet {
		assets = append(assets, k)
	}
	sort.Strings(assets)
	items := make([]DriftItem,
		0, len(assets))
	bundles := make([]SnapBundle, 0, len(assets))
	thresh := in.ThreshCfg
	for _, as := range assets {
		l := ledBy[as]
		cH := custHot[as]
		cC := custCold[as]
		iH := invHot[as]
		iC := invCold[as]
		// 使用 custHot 作为主要实物观察侧（hotinv 作辅助）
		// delta = ledger - custody，正值表示账面大于实物（穿仓风险）
		dH := l.hot.Sub(cH)
		dC := l.cold.Sub(cC)
		grade := gradeItem(as, l.hot, dH, thresh)
		// cold 侧取更严级
		gradeC := gradeItem(as, l.cold, dC, thresh)
		if gradeSeverity(gradeC) > gradeSeverity(grade) {
			grade = gradeC
		}
		// 偏离百分比（取 hot 侧，基数为 ledger_hot；基数为零则用 cold）
		pct := computePct(dH, l.hot)
		if l.hot.IsZero() {
			pct = computePct(dC, l.cold)
		}
		reason := buildReason(dH, dC, grade)
		gradedAt := now
		item := DriftItem{
			ID:      uuid.NewString(),
			BatchID: batchID,
			Asset:   as,
			DataCenter: in.DataCenter,
			LedgerHot:  l.hot.String(),
			LedgerCold: l.cold.String(),
			LedgerWarm: l.warm.String(),
			CustHot:    cH.String(),
			CustCold:   cC.String(),
			InvHot:     iH.String(),
			InvCold:    iC.String(),
			DeltaHot:   dH.String(),
			DeltaCold:  dC.String(),
			DeltaPct: pct,
			Grade:     grade,
			GradedAt:  &gradedAt,
			Reason:    reason,
			CreatedAt: now,
		}
		items = append(items, item)
		bundles = append(bundles, SnapBundle{
			Asset: as,
			Ledger: LedgerSnapshot{
				Asset: as,
				Hot:  l.hot.String(),
				Warm: l.warm.String(),
				Cold: l.cold.String(),
			},
			Custody: CustodySnapshot{
				Asset: as,
				Hot:   cH.String(),
				Cold:  cC.String(),
			},
			HotInv: HotInvSnapshot{
				Asset: as,
				Hot:    iH.String(),
				Cold:   iC.String(),
				HotMax: invHotMax[as],
				Excess: invExcess[as],
			},
		})
	}
	return ScanOutput{Items: items, Bundles: bundles}
}

// LoadLedgerRows 从账本 Store 读取 system 用户的 bridge/cold/warm 账户行。
   func LoadLedgerRows(ctx context.Context, st ledger.Store) ([]ledger.Account, error) {
	  if st == nil {
		   return nil, ErrStore
	  }
	  accs, err := st.ListUserBooks(ctx, "system")
	   if err != nil {
		   return nil, err
	   }
	   out := make([]ledger.Account, 0, len(accs))
	  for _, a := range accs {
		   switch a.AccountType {
		   case ledger.AccountBridge, ledger.AccountCold, ledger.AccountWarm:
			  out = append(out, a)
		   }
	   }
	   return out, nil
   }

// gradeItem 对单侧（hot 或 cold）偏离进行分级。
  func gradeItem(asset string, base, delta decimal.Decimal, cfg ThresholdConfig) DriftGrade {
	   absDelta := delta.Abs()
	  if absDelta.IsZero() {
		   return GradeInfo
	   }
	   warnPct := threshVal(cfg.WarnPct, asset, "1")
	  critPct := threshVal(cfg.CritPct, asset, "5")
	   warnAbs := threshVal(cfg.WarnAbs, asset, "5000")
	   critAbs := threshVal(cfg.CritAbs, asset, "50000")
	  warnP,
		  _ := decimal.NewFromString(warnPct)
	  critP, _ := decimal.NewFromString(critPct)
	   warnA, _ := decimal.NewFromString(warnAbs)
	   critA, _ := decimal.NewFromString(critAbs)
	   // 绝对额或百分比任一达到 crit 即 crit
	   if absDelta.GreaterThanOrEqual(critA) {
		  return GradeCrit
	  }
	   if !base.IsZero() {
		  pct := absDelta.Mul(decimal.NewFromInt(100)).Div(base.Abs())
		  if pct.GreaterThanOrEqual(critP) {
			   return GradeCrit
		  }
		  if pct.GreaterThanOrEqual(warnP) {
			   return GradeWarn
		   }
	  }
	   if absDelta.GreaterThanOrEqual(warnA) {
		   return GradeWarn
	  }
	   return GradeInfo
  }

   func gradeSeverity(g DriftGrade) int {
	  switch g {
	   case GradeCrit:
		   return 2
	  case GradeWarn:
		  return 1
	  default:
		  return 0
	  }
   }

   func threshVal(m map[string]string, asset, fallback string) string {
	   if v, ok := m[asset]; ok && v != "" {
		   return v
	   }
	   if v, ok := m["default"]; ok && v != "" {
		  return v
	  }
	  return fallback
  }

   func computePct(delta, base decimal.Decimal) string {
	  if base.IsZero() {
		   return "0"
	   }
	  pct := delta.Abs().Mul(decimal.NewFromInt(100)).Div(base.Abs())
	   return pct.StringFixed(4)
  }

   func buildReason(dH, dC decimal.Decimal, grade DriftGrade) string {
	  switch grade {
	  case GradeCrit:
		   return "critical drift: hot=" + dH.String() + " cold=" + dC.String()
	   case GradeWarn:
		   return "warn drift: hot=" + dH.String() + " cold=" + dC.String()
	  default:
		   return ""
	  }
   }

// CountByGrade 按等级统计条目数

  func CountByGrade(items []DriftItem) (crit, warn, info int) {
	   for _, it := range items {
		   switch it.Grade {
		   case GradeCrit:
			  crit++
		   case GradeWarn:
			  warn++
		   default:
			  info++
		  }
	   }
	   return
   }

// FilterCrit 过滤出 crit 级条目

   func FilterCrit(items []DriftItem) []DriftItem {
	   out := make([]DriftItem, 0)
	  for _, it := range items {
		   if it.Grade == GradeCrit {
			   out = append(out, it)
		  }
	  }
	   return out
  }

// BuildSealAdvices 为每个 crit 条目生成封堵建议（三种类型各一条）。

   func BuildSealAdvices(batchID string, critItems []DriftItem, ttlSec int, clock func() time.Time) []SealAdvice {
	   if clock == nil {
		  clock = func() time.Time { return time.Now().UTC() }
	}
	now := clock()
	if ttlSec <= 0 {
		ttlSec = 1800 // 默认 30 分钟
	}
	kinds := []SealKind{SealKindBlockSign, SealKindBlockSweepOver, SealKindBlockLargeWdPre}
	out := make([]SealAdvice, 0, len(critItems)*len(kinds))
	for _, it := range critItems {
		for _, k := range kinds {
			out = append(out, SealAdvice{
				ID:      uuid.NewString(),
				BatchID: batchID,
				ItemID:  it.ID,
				Asset:   it.Asset,
				Kind:  k,
				State: AdviceStateProposed,
				Grade: GradeCrit,
				ProposedAt: now,
				TTLSeconds: ttlSec,
				Note:      "auto-proposed by drift pipeline: " + it.Reason,
				CreatedAt: now,
			})
		}
	}
	return out
}

// ValidateAsset 使用 validate 包校验资产名
  func ValidateAsset(s string) (string, error) {
	   a, err := validate.Asset(s)
	   if err != nil {
		  return "", ErrAsset
	   }
	  return a, nil
   }

// ValidateActor 使用 validate 包校验操作者

  func ValidateActor(s string) (string, error) {
	   a, err := validate.Actor(s)
	   if err != nil {
		  return "", ErrActor
	   }
	  return a, nil
   }

// BuildBatchFromItems 根据扫描结果构建 DriftBatch 对象
  func BuildBatchFromItems(id, dc, actor string, items []DriftItem, clock func() time.Time) DriftBatch {
	   if clock == nil {
		   clock = func() time.Time { return time.Now().UTC() }
	}
	now := clock()
	crit, warn, info := CountByGrade(items)
	state := BatchStateGraded
	if crit > 0 {
		state = BatchStateSealed
	}
	gradedAt := now
	var sealedAt *time.Time
	if crit > 0 {
		t := now
		sealedAt = &t
	}
	return DriftBatch{
		ID:        id,
		State:     state,
		TriggerBy: actor,
		DataCenter: dc,
		ScanAt: now,
		GradedAt:  &gradedAt,
		SealedAt:  sealedAt,
		CritCount: crit,
		WarnCount: warn,
		InfoCount: info,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// ParseAmountSafe 解析金额字符串，失败返回 ErrAmount

   func ParseAmountSafe(s string) (money.Amount, error) {
	   s = orZero(s)
	   for _, r := range s {
		  if r == 'e' || r == 'E' {
			   return money.Zero(), ErrAmount
		   }
	  }
	   got, err := money.ParseBook(s)
	  if err != nil {
		  return money.Zero(), ErrAmount
	  }
	   return got, nil
   }

   func orZero(s string) string {
	   s = strings.TrimSpace(s)
	   if s == "" {
		  return "0"
	   }
	  return s
  }
