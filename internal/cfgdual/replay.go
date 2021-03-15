package cfgdual

import (
	"context"
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/internal/cfgcenter"
)

// ─────────────────────────────────────────────────────────────────────────────
// ReplayReport 回放报告
// ─────────────────────────────────────────────────────────────────────────────
// ReplayStatus 单条决策回放状态枚举。
type ReplayStatus string

const (
	// ReplayMatch 回放结论与原始结论一致。
	ReplayMatch ReplayStatus = "match"
	// ReplayMismatch 回放结论与原始结论不一致（包配置已变更）。
	ReplayMismatch ReplayStatus = "mismatch"
	// ReplayMissingPack 指定代次的包已不存在于 cfgcenter，无法回放。
	ReplayMissingPack ReplayStatus = "missing_pack"
	// ReplaySkipped 跳过（生成时满足过滤条件但数据不完整）。
	ReplaySkipped ReplayStatus = "skipped"
)

// ReplayItem 单条决策回放结果。

type ReplayItem struct {
	// DecisionID 原始决策 ID。
	DecisionID string `json:"decision_id"`
	// UserID 请求用户。
	UserID string `json:"user_id"`
	// Asset 资产。
	Asset string `json:"asset"`
	// Amount 请求金额。
	Amount string `json:"amount"`
	// OriginalBaselineAllow 原始 baseline 结论。
	OriginalBaselineAllow bool `json:"original_baseline_allow"`
	// OriginalCanaryAllow 原始 canary 结论。
	OriginalCanaryAllow bool `json:"original_canary_allow"`
	// OriginalHasDiff 原始是否有差分。
	OriginalHasDiff bool `json:"original_has_diff"`
	// OriginalDiffKind 原始差分类型。
	OriginalDiffKind DiffKind `json:"original_diff_kind"`
	// ReplayBaselineAllow 回放 baseline 结论。
	ReplayBaselineAllow bool `json:"replay_baseline_allow"`
	// ReplayBaselineReason 回放 baseline 拒绝原因。
	ReplayBaselineReason string `json:"replay_baseline_reason,omitempty"`
	// ReplayCanaryAllow 回放 canary 结论（无 canary 代次时为 false）。
	ReplayCanaryAllow bool `json:"replay_canary_allow"`
	// ReplayCanaryReason 回放 canary 拒绝原因。
	ReplayCanaryReason string `json:"replay_canary_reason,omitempty"`
	// ReplayHasDiff 回放是否有差分。
	ReplayHasDiff bool `json:"replay_has_diff"`
	// ReplayDiffKind 回放差分类型。
	ReplayDiffKind DiffKind `json:"replay_diff_kind"`
	// Status 回放状态。
	Status ReplayStatus `json:"status"`
	// Note 附加说明。
	Note string `json:"note,omitempty"`
}

// ReplayReport 回放报告汇总。
type ReplayReport struct {
	// GrayReleaseID 对应发布 ID。
	GrayReleaseID string `json:"gray_release_id"`
	// Generation 回放的世代号（0 表示全部）。
	Generation int `json:"generation"`
	// PackName 策略包名。
	PackName string `json:"pack_name"`
	// BaselineVersion baseline 代次号。
	BaselineVersion int `json:"baseline_version"`
	// CanaryVersion canary 代次号（可选）。
	CanaryVersion int `json:"canary_version"`
	// TotalReplayed 实际回放条数。
	TotalReplayed int `json:"total_replayed"`
	// MatchCount 结论一致数。
	MatchCount int `json:"match_count"`
	// MismatchCount 结论不一致数。
	MismatchCount int `json:"mismatch_count"`
	// MissingPackCount 包缺失数。
	MissingPackCount int `json:"missing_pack_count"`
	// SkippedCount 跳过数。
	SkippedCount int `json:"skipped_count"`
	// Items 逐条回放结果（仅保留 Mismatch 与 MissingPack）。
	Items []ReplayItem `json:"items,omitempty"`
	// Consistent 所有已回放记录结论一致。
	Consistent bool `json:"consistent"`
	// GeneratedAt 报告生成时间。
	GeneratedAt time.Time `json:"generated_at"`
}

// ─────────────────────────────────────────────────────────────────────────────
// GenerationConsistencyProof 世代切流一致性证明
// ─────────────────────────────────────────────────────────────────────────────
// GenerationProofEntry 单个用户切流一致性记录。

type GenerationProofEntry struct {
	// UserID 用户 ID。
	UserID string `json:"user_id"`
	// Generation 世代号。
	Generation int `json:"generation"`
	// InCanary 该世代下 InCanary 结论（不变量）。
	InCanary bool `json:"in_canary"`
	// VerifiedAt 验证时间戳。
	VerifiedAt time.Time `json:"verified_at"`
}

// GenerationConsistencyProof 世代切流一致性证明。
// 证明：同一 userID + generation 三元组在多次独立调用中 InCanary 结论不变。
type GenerationConsistencyProof struct {
	// ReleaseID 对应发布 ID。
	ReleaseID string `json:"release_id"`
	// PackName 包名。
	PackName string `json:"pack_name"`
	// Generation 世代号。
	Generation int `json:"generation"`
	// Percent 切流比例。
	Percent int `json:"percent"`
	// SampleCount 抽样用户数。
	SampleCount int `json:"sample_count"`
	// VerifyRounds 每用户验证轮数。
	VerifyRounds int `json:"verify_rounds"`
	// AllConsistent 全部抽样用户在全部轮次中结论一致。
	AllConsistent bool `json:"all_consistent"`
	// Entries 不一致记录（AllConsistent=true 时为空）。
	Entries []GenerationProofEntry `json:"entries,omitempty"`
	// GeneratedAt 生成时间。
	GeneratedAt time.Time `json:"generated_at"`
}

// ─────────────────────────────────────────────────────────────────────────────
// ReplayEngine 审计回放引擎
// ─────────────────────────────────────────────────────────────────────────────
// ReplayEngine 对历史决策进行审计回放。

type ReplayEngine struct {
	store    Store
	cfgStore *cfgcenter.Store
	clock func() time.Time
}

// NewReplayEngine 构造回放引擎。
  func NewReplayEngine(store Store, cfgStore *cfgcenter.Store) *ReplayEngine {
	   return &ReplayEngine{
		   store:    store,
		   cfgStore: cfgStore,
		   clock:    func() time.Time { return time.Now().UTC() },
	  }
   }

// WithClock 注入时钟（用）。
  func (e *ReplayEngine) WithClock(c func() time.Time) *ReplayEngine {
	   if c != nil {
		   e.clock = c
	   }
	   return e
   }

   // Replay 对指定发布的历史决策执行回放校验。
   // 参数：
   // - generation: 0 = 回放所有世代；> 0 = 仅回放指定世代；
  // - diffOnly: true = 仅回放 HasDiff=true 的记录；
  // - limit: 最多回放条数（0 = 默认 500）。
   func (e *ReplayEngine) Replay(
	   ctx context.Context,
	   release *GrayRelease,
	  generation int,
	   diffOnly bool,
	  limit int,
   ) (*ReplayReport, error) {
	  if e == nil || release == nil {
		  return nil, ErrUnavailable
	   }
	  if limit <= 0 {
		  limit = 500
	   }
	  decisions, err := e.store.ListDecisions(ctx, release.ID, limit)
	   if err != nil {
		   return nil, fmt.Errorf("replay: list decisions: %w", err)
	   }
	  // 获取包代次（可能不存在）。
	   baselinePack,
		  baselineErr := e.cfgStore.VersionPack(release.PackName, release.BaselineVersion)
	   canaryPack, canaryErr := e.cfgStore.VersionPack(release.PackName, release.CanaryVersion)
	   report := &ReplayReport{
		  GrayReleaseID:   release.ID,
		   PackName:        release.PackName,
		  BaselineVersion: release.BaselineVersion,
		   CanaryVersion: release.CanaryVersion,
		  GeneratedAt: e.clock(),
	   }
	   if generation > 0 {
		  report.Generation = generation
	   }
	   for _, d := range decisions {
		  // 世代过滤。
		   if generation > 0 && d.Generation != generation {
			  continue
		   }
		  // 差分过滤。
		   if diffOnly && !d.HasDiff {
			   continue
		  }
		   item := ReplayItem{
			  DecisionID: d.ID,
			   UserID: d.UserID,
			  Asset:                 d.Asset,
			   Amount:                d.Amount,
			  OriginalBaselineAllow: d.BaselineAllow,
			   OriginalCanaryAllow:   d.CanaryAllow,
			   OriginalHasDiff:       d.HasDiff,
			  OriginalDiffKind: d.DiffKind,
		  }
		   if baselineErr != nil {
			   item.Status = ReplayMissingPack
			   item.Note = fmt.Sprintf("baseline pack v%d not found: %v", release.BaselineVersion, baselineErr)
			  report.Items = append(report.Items, item)
			  report.MissingPackCount++
			   report.TotalReplayed++
			   continue
		  }
		   // 回放 baseline 侧。
		  cfgReq := cfgcenter.Request{
			   Pack: release.PackName,
			   Asset:  d.Asset,
			   Amount: d.Amount,
		   }
		  baseDec, _ := cfgcenter.EvalPack(baselinePack, cfgReq)
		   item.ReplayBaselineAllow = baseDec.Allow
		   item.ReplayBaselineReason = baseDec.Reason
		   // 回放 canary 侧（可选）。
		  if canaryErr == nil {
			  canaryDec, _ := cfgcenter.EvalPack(canaryPack, cfgReq)
			   item.ReplayCanaryAllow = canaryDec.Allow
			   item.ReplayCanaryReason = canaryDec.Reason
			   _, item.ReplayHasDiff = classifyDiff(
				  EvalSide{Allow: item.ReplayBaselineAllow, Version: baselinePack.Version},
				   EvalSide{Allow: item.ReplayCanaryAllow, Version: canaryPack.Version},
			   )
			  item.ReplayDiffKind,
				   _ = classifyDiff(
				   EvalSide{Allow: item.ReplayBaselineAllow, Version: baselinePack.Version},
				   EvalSide{Allow: item.ReplayCanaryAllow, Version: canaryPack.Version},
			  )
		  }
		   // 结论。
		   originalMatch := item.OriginalBaselineAllow == item.ReplayBaselineAllow &&
			  item.OriginalCanaryAllow == item.ReplayCanaryAllow &&
			  item.OriginalHasDiff == item.ReplayHasDiff
		  if originalMatch {
			   item.Status = ReplayMatch
			  report.MatchCount++
		  } else {
			   item.Status = ReplayMismatch
			  item.Note = fmt.Sprintf(
				   "original(base=%v,canary=%v,diff=%v) != replay(base=%v,canary=%v,diff=%v)",
				  item.OriginalBaselineAllow,
				  item.OriginalCanaryAllow, item.OriginalHasDiff,
				   item.ReplayBaselineAllow, item.ReplayCanaryAllow, item.ReplayHasDiff,
			   )
			  report.Items = append(report.Items, item)
			  report.MismatchCount++
		  }
		   report.TotalReplayed++
	  }
	  report.Consistent = report.MismatchCount == 0 && report.MissingPackCount == 0
	  return report, nil
   }

// VerifyGenerationConsistency 抽样验证世代切流一致性。
// 对 userIDs 中的每个用户重复调用 cohort.InCanary rounds 次，
// 验证同一世代内结论不变。
   func VerifyGenerationConsistency(
	  cohort *Cohort,
	  userIDs []string,
	   asset string,
	  rounds int,
	  clock func() time.Time,
) *GenerationConsistencyProof {
	if cohort == nil {
		return nil
	}
	if rounds <= 0 {
		rounds = 10
	}
	now := clock()
	gen := cohort.Generation()
	snap := cohort.Snapshot()
	proof := &GenerationConsistencyProof{
		ReleaseID: snap.ReleaseID,
		PackName: snap.PackName,
		Generation: gen,
		Percent:    snap.Percent,
		SampleCount:  len(userIDs),
		VerifyRounds: rounds,
		AllConsistent: true,
		GeneratedAt:   now,
	}
	for _, uid := range userIDs {
		ref, _ := cohort.InCanary(uid, asset)
		for i := 1; i < rounds; i++ {
			got, gotGen := cohort.InCanary(uid, asset)
			if gotGen != gen || got != ref {
				proof.AllConsistent = false
				proof.Entries = append(proof.Entries, GenerationProofEntry{
					UserID: uid,
					Generation: gotGen,
					InCanary:   got,
					VerifiedAt: clock(),
				})
				break
			}
		}
	}
	return proof
}
