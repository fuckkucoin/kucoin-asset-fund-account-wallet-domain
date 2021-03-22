package clearing

import (
	"strings"
	"time"
	"github.com/shopspring/decimal"
)

// DispositionKind 处置建议类型。

type DispositionKind string

const (
	// DispositionResolve 建议解决（平衡已确认）。
	DispositionResolve DispositionKind = "resolve"
	// DispositionReject 建议拒绝（无法对账）。
	DispositionReject DispositionKind = "reject"
	// DispositionEscalate 建议升级（超阈值，需人工介入）。
	DispositionEscalate DispositionKind = "escalate"
	// DispositionHold 建议保留（等待链上确认）。
	DispositionHold DispositionKind = "hold"
)

// Recommendation 处置建议。
type Recommendation struct {
	EntryID string `json:"entry_id"`
	Kind DispositionKind `json:"kind"`
	Reason     string  `json:"reason"`
	Confidence float64 `json:"confidence"` // 0~1，置信度
	RuleCode   string  `json:"rule_code"`
}

// RuleConfig 规则配置。

type RuleConfig struct {
	// SmallAmountThreshold 小额阈值（USDT 等值）：低于此值自动建议 resolve。
	SmallAmountThreshold string
	// LargeAmountThreshold 大额阈值：超过此值强制升级。
	LargeAmountThreshold string
	// MaxHoldDuration 最大保留时长：超过后建议 reject。
	MaxHoldDuration time.Duration
}

// DefaultRuleConfig 默认规则配置。
   func DefaultRuleConfig() RuleConfig {
	  return RuleConfig{
		   SmallAmountThreshold: "1",
		   LargeAmountThreshold: "100000",
		   MaxHoldDuration: 7 * 24 * time.Hour,
	  }
   }

// RulesEngine 挂账处置规则引擎。
type RulesEngine struct {
	cfg RuleConfig
	clock func() time.Time
}

// NewRulesEngine 初始化规则引擎。

   func NewRulesEngine(cfg RuleConfig) *RulesEngine {
	   return &RulesEngine{
		  cfg:   cfg,
		  clock: func() time.Time { return time.Now().UTC() },
	  }
  }

// WithClock 注入时钟（用）。

   func (re *RulesEngine) WithClock(c func() time.Time) *RulesEngine {
	  if re != nil && c != nil {
		  re.clock = c
	   }
	   return re
   }

   // Recommend 对指定挂账分录生成处置建议。
  func (re *RulesEngine) Recommend(entry SuspenseEntry) Recommendation {
	  if re == nil {
		  return Recommendation{EntryID: entry.ID, Kind: DispositionHold, Reason: "no rules engine"}
	  }
	  now := re.clock()
	   amt,
		  _ := decimal.NewFromString(entry.Amount)
	  small, _ := decimal.NewFromString(re.cfg.SmallAmountThreshold)
	   large, _ := decimal.NewFromString(re.cfg.LargeAmountThreshold)
	   // 规则 1：超保留时长 → 建议拒绝
	   if !entry.CreatedAt.IsZero() && now.Sub(entry.CreatedAt) > re.cfg.MaxHoldDuration {
		   return Recommendation{
			  EntryID: entry.ID,
			   Kind:    DispositionReject,
			   Reason:     "suspense entry held beyond max duration, recommend reject",
			  Confidence: 0.85,
			  RuleCode:   "R001_MAX_HOLD",
		   }
	   }
	   // 规则 2：金额超大额阈值 → 强制升级
	  if amt.GreaterThan(large) {
		  return Recommendation{
			  EntryID: entry.ID,
			  Kind:    DispositionEscalate,
			   Reason:  "amount exceeds large-amount threshold, mandatory escalation",
			   Confidence: 1.0,
			   RuleCode:   "R002_LARGE_AMOUNT",
		   }
	  }
	   // 规则 3：小额 → 自动建议解决
	  if amt.LessThanOrEqual(small) {
		  return Recommendation{
			   EntryID: entry.ID,
			   Kind: DispositionResolve,
			  Reason:     "small-amount suspense, auto-resolve recommended",
			   Confidence: 0.9,
			  RuleCode: "R003_SMALL_AMOUNT",
		  }
	   }
	  // 规则 4：recon 差异单已关联 → 建议解决
	   if strings.TrimSpace(entry.ReconDiffID) != "" {
		  return Recommendation{
			   EntryID: entry.ID,
			  Kind:   DispositionResolve,
			   Reason: "recon diff reference present, resolve after verification",
			   Confidence: 0.75,
			   RuleCode: "R004_RECON_REF",
		   }
	  }
	   // 默认：保留等待人工
	  return Recommendation{
		   EntryID: entry.ID,
		   Kind: DispositionHold,
		  Reason:     "no matching rule, hold for manual review",
		   Confidence: 0.5,
		  RuleCode: "R000_DEFAULT",
	  }
   }

// RecommendAll 对 SuspenseBook 中所有 open 挂账生成建议。
   func (re *RulesEngine) RecommendAll(sb *SuspenseBook) []Recommendation {
	  if sb == nil {
		  return nil
	   }
	   openEntries := sb.ListOpen()
	  out := make([]Recommendation, 0, len(openEntries))
	   for _, e := range openEntries {
		  out = append(out,
			  re.Recommend(e))
	  }
	   return out
  }
