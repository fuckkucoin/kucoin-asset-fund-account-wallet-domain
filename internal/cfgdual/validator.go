package cfgdual

import (
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/cfgcenter"
	"account-assets.kucoin.net/funding/pkg/money"
)

// ─────────────────────────────────────────────────────────────────────────────
// ValidationError 校验错误
// ─────────────────────────────────────────────────────────────────────────────
// ValidationError 发布前置校验失败。
type ValidationError struct {
	Field string
	Message string
}

   func (e *ValidationError) Error() string {
	  if e.Field != "" {
		   return fmt.Sprintf("cfgdual validation: %s: %s", e.Field, e.Message)
	  }
	   return fmt.Sprintf("cfgdual validation: %s", e.Message)
   }

// ValidationResult 前置校验汇总结果。

type ValidationResult struct {
	// Valid 全部校验通过。
	Valid bool `json:"valid"`
	// Errors 校验错误列表。
	Errors []*ValidationError `json:"errors,omitempty"`
	// Warnings 警告（不阻断激活，但建议处理）。
	Warnings []string `json:"warnings,omitempty"`
	// PackDiff 包差异摘要（仅在 ValidateActivation 时填充）。
	PackDiff *PackDiffSummary `json:"pack_diff,omitempty"`
}

   func (r *ValidationResult) addErr(field, msg string) {
	   r.Errors = append(r.Errors, &ValidationError{Field: field, Message: msg})
	  r.Valid = false
   }

   func (r *ValidationResult) addWarn(msg string) {
	   r.Warnings = append(r.Warnings,
		  msg)
  }

// ─────────────────────────────────────────────────────────────────────────────
// PackDiffSummary 包差异摘要
// ─────────────────────────────────────────────────────────────────────────────
// PackDiffSummary baseline 与 canary 包的字段差异摘要。

type PackDiffSummary struct {
	// PackName 包名。
	PackName string `json:"pack_name"`
	// BaselineVersion baseline 代次。
	BaselineVersion int `json:"baseline_version"`
	// CanaryVersion canary 代次。
	CanaryVersion int `json:"canary_version"`
	// MaxSingleChanged 单笔上限变更。
	MaxSingleChanged bool `json:"max_single_changed"`
	// MaxSingleBaseline baseline 单笔上限。
	MaxSingleBaseline string `json:"max_single_baseline,omitempty"`
	// MaxSingleCanary canary 单笔上限。
	MaxSingleCanary string `json:"max_single_canary,omitempty"`
	// MaxSingleTightened canary 收紧（< baseline）→ 更容易产生 allow_deny 脑裂。
	MaxSingleTightened bool `json:"max_single_tightened"`
	// DailyWithdrawChanged 日额度变更。
	DailyWithdrawChanged bool `json:"daily_withdraw_changed"`
	// DailyWithdrawTightened canary 日额度收紧。
	DailyWithdrawTightened bool `json:"daily_withdraw_tightened"`
	// HourlyVelChanged 小时速率变更。
	HourlyVelChanged bool `json:"hourly_vel_changed"`
	// HourlyVelTightened 小时速率收紧（< baseline）。
	HourlyVelTightened bool `json:"hourly_vel_tightened"`
	// DailyVelChanged 日速率变更。
	DailyVelChanged bool `json:"daily_vel_changed"`
	// DailyVelTightened 日速率收紧。
	DailyVelTightened bool `json:"daily_vel_tightened"`
	// AssetMaxChanged 资产专项上限变更（key 集合差或值差）。
	AssetMaxChanged bool `json:"asset_max_changed"`
	// TighteningDetected canary 相对 baseline 有收紧项（提示 allow_deny 风险）。
	TighteningDetected bool `json:"tightening_detected"`
	// LooseningDetected canary 相对 baseline 有放宽项（提示 deny_allow 风险）。
	LooseningDetected bool `json:"loosening_detected"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Validator 发布前置校验器
// ─────────────────────────────────────────────────────────────────────────────
// Validator 执行 GrayRelease 创建与激活的前置校验。

type Validator struct {
	cfgStore *cfgcenter.Store
}

// NewValidator 构造校验器。

   func NewValidator(cfgStore *cfgcenter.Store) *Validator {
	  return &Validator{cfgStore: cfgStore}
   }

// ValidateCreateRequest 校验创建发布参数。

   func (v *Validator) ValidateCreateRequest(req CreateReleaseRequest) *ValidationResult {
	  r := &ValidationResult{Valid: true}
	  if strings.TrimSpace(req.PackName) == "" {
		   r.addErr("pack_name", "不能为空")
	   }
	  if req.BaselineVersion <= 0 {
		  r.addErr("baseline_version", "必须大于零")
	   }
	   if req.CanaryVersion <= 0 {
		  r.addErr("canary_version", "必须大于零")
	   }
	   if req.BaselineVersion > 0 && req.CanaryVersion > 0 && req.CanaryVersion <= req.BaselineVersion {
		  r.addErr("canary_version",
			   fmt.Sprintf("canary_version(%d) 必须大于 baseline_version(%d)",
				  req.CanaryVersion, req.BaselineVersion))
	   }
	   if req.CohortPercent < 0 || req.CohortPercent > 100 {
		   r.addErr("cohort_percent",
			   fmt.Sprintf("切流比例必须在 [0,100]，实际 %d", req.CohortPercent))
	  }
	  if req.PolicyAllowDrift < 0 || req.PolicyAllowDrift > 100 {
		  r.addErr("policy_allow_drift",
			  fmt.Sprintf("允许漂移比例必须在 [0,100]，实际 %d", req.PolicyAllowDrift))
	   }
	  if req.CohortPercent > 50 {
		   r.addWarn(fmt.Sprintf("切流比例 %d%% > 50%%，建议先以 5–10%% 观察，逐步扩量", req.CohortPercent))
	  }
	   if req.PolicyAllowDrift == 0 {
		  r.addWarn("policy_allow_drift=0：任意差分立即触发封堵，适合严格场景；请确认为预期行为")
	  }
	  return r
   }

// ValidateActivation 激活前校验：包存在性 + 差异摘要 + 收紧警告。
  func (v *Validator) ValidateActivation(release *GrayRelease) *ValidationResult {
	  r := &ValidationResult{Valid: true}
	  if v.cfgStore == nil {
		  r.addErr("cfg_store", "cfgcenter.Store 未初始化")
		  return r
	  }
	  if release == nil {
		   r.addErr("release", "release 为空")
		   return r
	   }
	   // 校验 baseline 包存在。
	   baselinePack, bErr := v.cfgStore.VersionPack(release.PackName, release.BaselineVersion)
	   if bErr != nil {
		   r.addErr("baseline_version",
			  fmt.Sprintf("包 %s v%d 不存在于 cfgcenter：%v",
				   release.PackName, release.BaselineVersion, bErr))
	   }
	   // 校验 canary 包存在。
	   canaryPack, cErr := v.cfgStore.VersionPack(release.PackName, release.CanaryVersion)
	   if cErr != nil {
		   r.addErr("canary_version",
			  fmt.Sprintf("包 %s v%d 不存在于 cfgcenter：%v",
				  release.PackName, release.CanaryVersion, cErr))
	  }
	  // 若两侧均存在，生成差异摘要。
	   if bErr == nil && cErr == nil {
		   diff := computePackDiff(baselinePack, canaryPack)
		  r.PackDiff = diff
		   if diff.TighteningDetected {
			   r.addWarn("canary 包相对 baseline 存在收紧项（max_single/daily_withdraw/velocity 降低）" +
				   "：灰度期间 allow_deny 脑裂风险较高，建议降低 cohort_percent 并缩短观察窗口")
		   }
		   if diff.LooseningDetected {
			   r.addWarn("canary 包相对 baseline 存在放宽项：灰度期间可能产生 deny_allow 差分")
		  }
		  // 验证两侧均可 Eval（不报错）——用极小金额探测。
		   probeReq := cfgcenter.Request{Pack: release.PackName, Asset: "USDT", Amount: "1"}
		   if _, err := cfgcenter.EvalPack(baselinePack, probeReq); err != nil {
			   r.addErr("baseline_pack", fmt.Sprintf("baseline 包 Eval 探测失败：%v", err))
		  }
		  if _, err := cfgcenter.EvalPack(canaryPack, probeReq); err != nil {
			   r.addErr("canary_pack", fmt.Sprintf("canary 包 Eval 探测失败：%v", err))
		  }
	  }
	   return r
   }

// computePackDiff 计算两包差异。
   func computePackDiff(base, canary cfgcenter.LimitPack) *PackDiffSummary {
	   d := &PackDiffSummary{
		  PackName:        base.Name,
		   BaselineVersion: base.Version,
		  CanaryVersion:   canary.Version,
	  }
	  // 单笔上限。
	   if base.MaxSingle != canary.MaxSingle {
		   d.MaxSingleChanged = true
		   d.MaxSingleBaseline = base.MaxSingle
		  d.MaxSingleCanary = canary.MaxSingle
		   if base.MaxSingle != "" && canary.MaxSingle != "" {
			  bv, _ := money.Parse(base.MaxSingle)
			   cv,
				  _ := money.Parse(canary.MaxSingle)
			  if cv.Cmp(bv) < 0 {
				  d.MaxSingleTightened = true
				  d.TighteningDetected = true
			  } else if cv.Cmp(bv) > 0 {
				  d.LooseningDetected = true
			  }
		   }
	   }
	   // 日额度。
	  if base.DailyWithdraw != canary.DailyWithdraw {
		   d.DailyWithdrawChanged = true
		   if base.DailyWithdraw != "" && canary.DailyWithdraw != "" {
			   bv, _ := money.Parse(base.DailyWithdraw)
			  cv, _ := money.Parse(canary.DailyWithdraw)
			   if cv.Cmp(bv) < 0 {
				  d.DailyWithdrawTightened = true
				  d.TighteningDetected = true
			   } else if cv.Cmp(bv) > 0 {
				   d.LooseningDetected = true
			  }
		   }
	   }
	  // 小时速率。
	   if base.HourlyVel != canary.HourlyVel {
		  d.HourlyVelChanged = true
		  if canary.HourlyVel > 0 && (base.HourlyVel == 0 || canary.HourlyVel < base.HourlyVel) {
			  d.HourlyVelTightened = true
			   d.TighteningDetected = true
		   } else if base.HourlyVel > 0 && (canary.HourlyVel == 0 || canary.HourlyVel > base.HourlyVel) {
			   d.LooseningDetected = true
		  }
	  }
	   // 日速率。
	  if base.DailyVel != canary.DailyVel {
		   d.DailyVelChanged = true
		   if canary.DailyVel > 0 && (base.DailyVel == 0 || canary.DailyVel < base.DailyVel) {
			   d.DailyVelTightened = true
			   d.TighteningDetected = true
		  } else if base.DailyVel > 0 && (canary.DailyVel == 0 || canary.DailyVel > base.DailyVel) {
			  d.LooseningDetected = true
		   }
	  }
	  // 资产专项上限（简单比较键集合）。
	  if !assetMaxEqual(base.AssetMax, canary.AssetMax) {
		   d.AssetMaxChanged = true
	   }
	  return d
   }

// assetMaxEqual 比较两个 assetMax map。
  func assetMaxEqual(a, b map[string]string) bool {
	  if len(a) != len(b) {
		   return false
	  }
	  for k, av := range a {
		  bv, ok := b[k]
		   if !ok || av != bv {
			  return false
		   }
	  }
	  return true
   }

// ─────────────────────────────────────────────────────────────────────────────
// ConflictDetector 高危差分模式检测器
// ─────────────────────────────────────────────────────────────────────────────
// ConflictPattern 高危差分模式。

type ConflictPattern string

const (
	// ConflictAllowDenyBurst allow_deny 在短窗口内集中爆发（可能触发资金损失）。
	ConflictAllowDenyBurst ConflictPattern = "allow_deny_burst"
	// ConflictKillSwitchFlap kill switch 在短时间内反复开关（配置不稳定）。
	ConflictKillSwitchFlap ConflictPattern = "kill_switch_flap"
	// ConflictHighDiffRatio 差分比例持续高位（灰度包质量差）。
	ConflictHighDiffRatio ConflictPattern = "high_diff_ratio"
	// ConflictGenerationLeak 评估记录的 Generation 与当前 Cohort Generation 不匹配。
	ConflictGenerationLeak ConflictPattern = "generation_leak"
)

// ConflictAlert 检测到的高危差分模式告警。

type ConflictAlert struct {
	// Pattern 模式类型。
	Pattern ConflictPattern `json:"pattern"`
	// ReleaseID 对应发布 ID。
	ReleaseID string `json:"release_id"`
	// Detail 详情。
	Detail string `json:"detail"`
	// Severity P1/P2。
	Severity string `json:"severity"`
}

// ConflictDetector 基于 SplitDiff 列表检测高危模式。
type ConflictDetector struct {
	// AllowDenyBurstThreshold 短窗口 allow_deny 爆发阈值（默认 3）。
	AllowDenyBurstThreshold int64
	// HighDiffRatioThreshold 持续高差分比例阈值（默认 30%）。
	HighDiffRatioThreshold float64
}

// DefaultConflictDetector 默认检测参数。
  func DefaultConflictDetector() *ConflictDetector {
	  return &ConflictDetector{
		   AllowDenyBurstThreshold: 3,
		  HighDiffRatioThreshold: 30.0,
	  }
   }

// Detect 对给定 SplitDiff 列表检测高危模式，返回告警列表。

   func (d *ConflictDetector) Detect(releaseID string, diffs []*SplitDiff, currentGen int) []ConflictAlert {
	   if len(diffs) == 0 {
		   return nil
	   }
	   var alerts []ConflictAlert
	  var totalAllowDeny int64
	   var highRatioCount int
	   var genLeakCount int
	  for _, diff := range diffs {
		   totalAllowDeny += diff.AllowDenyCount
		   if diff.DiffRatio >= d.HighDiffRatioThreshold {
			  highRatioCount++
		  }
		   if currentGen > 0 && diff.Generation != currentGen {
			   genLeakCount++
		   }
	   }
	  // allow_deny 爆发。
	   if totalAllowDeny >= d.AllowDenyBurstThreshold {
		   alerts = append(alerts, ConflictAlert{
			   Pattern:   ConflictAllowDenyBurst,
			   ReleaseID: releaseID,
			  Detail: fmt.Sprintf("累计 allow_deny=%d 超过阈值 %d，存在资金脑裂风险",
				  totalAllowDeny, d.AllowDenyBurstThreshold),
			  Severity: "P1",
		   })
	   }
	   // 持续高差分比例（超过一半窗口）。
	  if highRatioCount > len(diffs)/2 && len(diffs) >= 2 {
		   alerts = append(alerts, ConflictAlert{
			  Pattern:   ConflictHighDiffRatio,
			  ReleaseID: releaseID,
			   Detail: fmt.Sprintf("超过半数窗口差分比例 >= %.1f%%（%d/%d 窗口），canary 包质量不达标",
				   d.HighDiffRatioThreshold, highRatioCount, len(diffs)),
			   Severity: "P2",
		   })
	   }
	  // 世代泄漏。
	   if genLeakCount > 0 {
		   alerts = append(alerts, ConflictAlert{
			  Pattern:   ConflictGenerationLeak,
			  ReleaseID: releaseID,
			   Detail: fmt.Sprintf("发现 %d 条 SplitDiff 世代号与当前 Cohort Generation(%d) 不匹配，可能存在世代泄漏",
				  genLeakCount, currentGen),
			  Severity: "P2",
		  })
	  }
	   return alerts
  }
