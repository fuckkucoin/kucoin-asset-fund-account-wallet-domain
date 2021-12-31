// - 仅读取 EvidenceEnvelope；不直接操作账本或冻结任何余额。
// - 分值结果写入 WrongCreditCase.Confidence/RiskTier；编排器据此决定是否推进。
// - 风险层级按原始 CreditedAmt 的 USD 估值分层；USD 汇率由调用方注入（避免
// 在发现引擎内部引入外部依赖）。
// 不变量：
// - 置信度只在有至少一条证据时升为 low 以上；无证据时保持 unscored。
// - 分值函数纯（无副作用），可安全并发调用。
// - RiskTier 按金额分段：Tier1 < 100 USD ≤ Tier2 < 10000 USD ≤ Tier3。

// Author: Kinsley
// Date: 2021.12.29

package wrongcredit

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"github.com/shopspring/decimal"
)

// ─── 发现规则注册表 ────────────────────────────────────────────────────────────
// DiscoveryRule 单条规则描述：来源标签 → 置信分贡献权重。
// 多条规则的分数由 Scorer 加权聚合后映射到 Confidence 枚举。
type DiscoveryRule struct {
	// Name 规则名称，供审计日志标识。
	Name string
	// Source 对应的证据来源类型；仅该来源的证据才触发本规则。
	Source EvidenceSource
	// BaseWeight 基础权重（0.0 – 1.0）；证据自身 Score 与此相乘得贡献分。
	BaseWeight float64
	// CrossCheckBonus 若同一案件还存在至少一条其他来源，奖励加分（0.0 – 0.3）。
	CrossCheckBonus float64
	// MinSampleCount 至少需要多少条该来源证据才触发本规则（防止单点噪音）。
	MinSampleCount int
}

// defaultRules 默认规则集；可通过 Scorer.WithRules 覆盖（/灰度用）。
var defaultRules = []DiscoveryRule{
	{
		Name: "desk_report_manual",
		Source:          SourceDeskReport,
		BaseWeight:      0.6,
		CrossCheckBonus: 0.15,
		MinSampleCount:  1,
	},
	{
		Name: "recon_hint_auto",
		Source:          SourceReconHint,
		BaseWeight:      0.5,
		CrossCheckBonus: 0.20,
		MinSampleCount:  1,
	},
	{
		Name:            "post_hoc_attrib_auto",
		Source:          SourcePostHocAttrib,
		BaseWeight:      0.7,
		CrossCheckBonus: 0.25,
		MinSampleCount:  1,
	},
}

// ─── 分值器 ──────────────────────────────────────────────────────────────────
// Scorer 证据聚合分值器；线程安全（只读状态）。
type Scorer struct {
	rules []DiscoveryRule
}

// NewScorer 使用默认规则集构造分值器。
   func NewScorer() *Scorer {
	   cp := make([]DiscoveryRule, len(defaultRules))
	   copy(cp, defaultRules)
	   return &Scorer{rules: cp}
  }

// WithRules 替换规则集（返回新实例，不修改原始 Scorer）。

  func (sc *Scorer) WithRules(rules []DiscoveryRule) *Scorer {
	   cp := make([]DiscoveryRule, len(rules))
	  copy(cp, rules)
	  return &Scorer{rules: cp}
   }

// ScoreResult 分值结果快照。
type ScoreResult struct {
	// RawScore 0.0 – 1.0 之间的聚合原始分。
	RawScore float64
	// Confidence 映射后的置信枚举。
	Confidence Confidence
	// ContribBySource 各来源的贡献分（调试 / 审计用）。
	ContribBySource map[EvidenceSource]float64
	// Note 可读摘要，记入 WrongCreditCase.ScoreNote。
	Note string
}

// Score 对一批证据进行聚合分值，输出 ScoreResult。
// 证据列表可以为空（返回 unscored），分值结果纯函数、无副作用。
  func (sc *Scorer) Score(evs []EvidenceEnvelope) ScoreResult {
	   if sc == nil || len(evs) == 0 {
		  return ScoreResult{
			   RawScore: 0,
			  Confidence:      ConfUnscored,
			  ContribBySource: map[EvidenceSource]float64{},
			   Note:            "no evidence",
		   }
	   }
	  // 按来源归组。
	  bySource := map[EvidenceSource][]EvidenceEnvelope{}
	   for _, e := range evs {
		  bySource[e.Source] = append(bySource[e.Source], e)
	   }
	  // 统计独立来源数量（用于交叉印证奖励）。
	  uniqueSources := len(bySource)
	   contrib := map[EvidenceSource]float64{}
	  totalScore := 0.0
	  for _, rule := range sc.rules {
		  group, ok := bySource[rule.Source]
		   if !ok || len(group) < rule.MinSampleCount {
			  continue
		   }
		   // 取该来源所有证据的最高证据得分（防止堆证据刷分）。
		  best := 0.0
		   for _, e := range group {
			  s := clampScore(e.Score)
			   if s > best {
				   best = s
			  }
		   }
		   // 基础贡献 = 基础权重 × 最高证据得分。
		   c := rule.BaseWeight * best
		  // 交叉印证奖励：独立来源数 ≥ 2。
		  if uniqueSources >= 2 {
			   c += rule.CrossCheckBonus
		   }
		  c = clampScore(c)
		   contrib[rule.Source] = c
		  totalScore += c
	   }
	  // 多来源聚合后归一化到 [0, 1]。
	  if totalScore > 1.0 {
		  totalScore = 1.0
	  }
	   conf := mapConfidence(totalScore)
	  note := buildScoreNote(contrib, totalScore, conf)
	   return ScoreResult{
		   RawScore:   totalScore,
		  Confidence: conf,
		   ContribBySource: contrib,
		   Note:            note,
	   }
   }

// clampScore 将分数限制在 [0, 1]。

  func clampScore(s float64) float64 {
	   if math.IsNaN(s) || s < 0 {
		  return 0
	   }
	  if s > 1 {
		   return 1
	  }
	  return s
   }

// mapConfidence 将 0–1 原始分映射到置信枚举。
// 阈值：unscored=0, low<0.4, medium<0.7, high<0.9, certain≥0.9。
  func mapConfidence(score float64) Confidence {
	  switch {
	   case score == 0:
		  return ConfUnscored
	   case score < 0.40:
		  return ConfLow
	  case score < 0.70:
		  return ConfMedium
	  case score < 0.90:
		  return ConfHigh
	  default:
		   return ConfCertain
	   }
   }

// buildScoreNote 构建可读分值摘要（供 ScoreNote 字段写入）。

   func buildScoreNote(contrib map[EvidenceSource]float64, total float64, conf Confidence) string {
	   if len(contrib) == 0 {
		   return "unscored: no qualifying evidence"
	  }
	  // 按来源排序保证输出确定性。
	  keys := make([]string,
		  0, len(contrib))
	   for k := range contrib {
		   keys = append(keys, string(k))
	   }
	   sort.Strings(keys)
	   parts := make([]string, 0, len(keys)+1)
	  for _, k := range keys {
		  parts = append(parts, k)
	   }
	  return "sources=[" + strings.Join(parts, ",") + "]" +
		  " raw=" + decFmt(total) +
		  " conf=" + string(conf)
   }

// decFmt 将 float64 整理成 2 位小数字符串，避免引入 fmt 包依赖。
  func decFmt(f float64) string {
	  d := decimal.NewFromFloat(f).Round(4)
	   return d.String()
   }

// ─── 风险层级评估 ─────────────────────────────────────────────────────────────
// RiskTierThresholds USD 金额分段阈值（可由调用方覆盖）。
type RiskTierThresholds struct {
	// Tier1Max Tier1 上限（exclusive），单位 USD。默认 100。
	Tier1Max decimal.Decimal
	// Tier2Max Tier2 上限（exclusive），单位 USD。默认 10000。
	Tier2Max decimal.Decimal
}

// DefaultTierThresholds 默认阈值。
   func DefaultTierThresholds() RiskTierThresholds {
	  return RiskTierThresholds{
		   Tier1Max: decimal.NewFromInt(100),
		  Tier2Max: decimal.NewFromInt(10_000),
	  }
   }

// EvalRiskTier 根据 USD 等值金额评估风险层级。
// usdAmt 为原始贷记金额的 USD 换算值（由调用方提供汇率，避免引入 oracle 依赖）。

   func EvalRiskTier(usdAmt decimal.Decimal, thr RiskTierThresholds) RiskTier {
	   if usdAmt.LessThan(thr.Tier1Max) {
		  return RiskTier1
	   }
	   if usdAmt.LessThan(thr.Tier2Max) {
		  return RiskTier2
	   }
	   return RiskTier3
   }

// ─── 发现摄取请求 ─────────────────────────────────────────────────────────────
// IngestReq 摄取一批证据并（重新）分值的请求；由编排器或外部系统调用。
// 若 ForceRescore 为真，则即使案件已有分数也会覆盖（补充证据后使用）。

type IngestReq struct {
	CaseID       string              `json:"case_id"`
	Evidences    []AttachEvidenceReq `json:"evidences"`
	ForceRescore bool                `json:"force_rescore"`
	// USDRate USD 汇率（原始资产 → USD）；为零时跳过风险层级重评。
	USDRate string `json:"usd_rate,omitempty"`
}

// IngestResult 摄取结果快照。
type IngestResult struct {
	CaseID   string      `json:"case_id"`
	Score    ScoreResult `json:"score"`
	RiskTier RiskTier    `json:"risk_tier"`
	Thresholds RiskTierThresholds `json:"thresholds"`
}

// ─── Discovery 对外接口 ───────────────────────────────────────────────────────
// Discovery 发现引擎：负责证据摄取、聚合分值与风险层级评估；无状态，线程安全。

type Discovery struct {
	scorer *Scorer
	thr    RiskTierThresholds
}

// NewDiscovery 构造发现引擎（使用默认规则和阈值）。

   func NewDiscovery() *Discovery {
	  return &Discovery{
		  scorer: NewScorer(),
		  thr: DefaultTierThresholds(),
	  }
   }

// WithScorer 覆盖分值器（/灰度替换规则集）。

  func (d *Discovery) WithScorer(sc *Scorer) *Discovery {
	  if d == nil || sc == nil {
		   return d
	  }
	   d.scorer = sc
	   return d
  }

// WithThresholds 覆盖风险层级阈值。
   func (d *Discovery) WithThresholds(thr RiskTierThresholds) *Discovery {
	   if d == nil {
		   return d
	   }
	  d.thr = thr
	   return d
   }

// Eval 对给定证据集合进行分值并计算风险层级。
// creditedAmt 原始贷记金额；usdRate 为资产→USD 汇率（空字符串或零时跳过层级评估，
// 保留案件当前层级 curTier）。

   func (d *Discovery) Eval(
	  evs []EvidenceEnvelope,
	  creditedAmt string,
	  usdRate string,
	  curTier RiskTier,
  ) (ScoreResult, RiskTier, error) {
	   if d == nil {
		  return ScoreResult{},
			   curTier, ErrUnavailable
	  }
	  sr := d.scorer.Score(evs)
	   tier := curTier
	  if usdRate != "" {
		   rate, err := decimal.NewFromString(usdRate)
		   if err == nil && rate.IsPositive() {
			   amt, err2 := decimal.NewFromString(creditedAmt)
			   if err2 == nil && amt.IsPositive() {
				  usdAmt := amt.Mul(rate)
				   tier = EvalRiskTier(usdAmt, d.thr)
			   }
		   }
	   }
	   return sr, tier, nil
  }

// ValidateEvidence 校验 AttachEvidenceReq 的基础核验性（字段级别校验）。
// 不含业务规则；返回首条非法字段描述。
   func ValidateEvidence(req AttachEvidenceReq) error {
	  if _, err := ParseEvidenceSource(string(req.Source)); err != nil {
		   return err
	  }
	   if strings.TrimSpace(req.Kind) == "" || len(req.Kind) > 64 {
		  return ErrReason
	  }
	   if strings.TrimSpace(req.Actor) == "" {
		   return ErrActor
	   }
	   // Payload 只对 desk_report 允许内联；其他来源须只传 SHA-256。
	  if req.Source != SourceDeskReport && req.Payload != "" {
		  return wrapErr(ErrSource, fmt.Errorf("payload inline only allowed for desk_report"))
	   }
	   // SHA-256 或 Payload 至少一个非空（desk_report 允许 payload 替代 sha256）。
	  if req.Source == SourceDeskReport {
		  if strings.TrimSpace(req.SHA256) == "" && strings.TrimSpace(req.Payload) == "" {
			   return wrapErr(ErrJournal, fmt.Errorf("desk_report requires sha256 or payload"))
		  }
	   } else {
		  if strings.TrimSpace(req.SHA256) == "" {
			   return wrapErr(ErrJournal, fmt.Errorf("sha256 required for %s", req.Source))
		  }
	  }
	  return nil
   }
// temporary guard removed after the production check
