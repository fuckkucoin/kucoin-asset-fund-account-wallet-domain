package cfgdual

import (
	"context"
	"fmt"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// AdminAuditRecord 管理员操作审计记录
// ─────────────────────────────────────────────────────────────────────────────
// AdminOpKind 管理员操作类型枚举。

type AdminOpKind string

const (
	AdminOpBatchPin AdminOpKind = "batch_pin"
	AdminOpEmergencySeal  AdminOpKind = "emergency_seal"
	AdminOpBatchUnpin     AdminOpKind = "batch_unpin"
	AdminOpExportSnapshot AdminOpKind = "export_snapshot"
	AdminOpUpdatePolicy AdminOpKind = "update_policy"
	AdminOpSetPercent AdminOpKind = "set_percent"
)

// AdminAuditRecord 管理员操作审计日志。

type AdminAuditRecord struct {
	// ID 记录唯一标识。
	ID string `json:"id"`
	// Op 操作类型。
	Op AdminOpKind `json:"op"`
	// ReleaseID 对应发布 ID。
	ReleaseID string `json:"release_id"`
	// Actor 操作者（运营 UID）。
	Actor string `json:"actor"`
	// Detail 操作详情（JSON 形态）。
	Detail string `json:"detail"`
	// CreatedAt 操作时间。
	CreatedAt time.Time `json:"created_at"`
}

// ─────────────────────────────────────────────────────────────────────────────
// BatchPinResult 批量 Pin 结果
// ─────────────────────────────────────────────────────────────────────────────
// BatchPinResult 批量 Pin 操作结果。

type BatchPinResult struct {
	// TotalRequested 请求 Pin 的用户数。
	TotalRequested int `json:"total_requested"`
	// Succeeded 成功 Pin 数。
	Succeeded int `json:"succeeded"`
	// Failed 失败 Pin 数（通常是 userID 为空或 side 非法）。
	Failed int `json:"failed"`
	// FailedUsers 失败的用户 ID 列表。
	FailedUsers []string `json:"failed_users,omitempty"`
	// AuditID 对应审计记录 ID。
	AuditID string `json:"audit_id"`
}

// ─────────────────────────────────────────────────────────────────────────────
// ReleaseSnapshot 发布状态快照
// ─────────────────────────────────────────────────────────────────────────────
// ReleaseSnapshot 发布完整状态快照（用于审计和跨进程恢复）。

type ReleaseSnapshot struct {
	// Release 发布元数据。
	Release *GrayRelease `json:"release"`
	// Cohort 切流参数快照。
	Cohort *CohortSnapshot `json:"cohort,omitempty"`
	// SealState 当前封堵状态。
	SealState SealState `json:"seal_state"`
	// SealActions 所有封堵动作记录。
	SealActions []*SealAction `json:"seal_actions,omitempty"`
	// RollbackProof 回滚证明（已回滚时有值）。
	RollbackProof *RollbackProof `json:"rollback_proof,omitempty"`
	// LatestSplitDiff 最新差分记录。
	LatestSplitDiff *SplitDiff `json:"latest_split_diff,omitempty"`
	// SnapshotAt 快照生成时间。
	SnapshotAt time.Time `json:"snapshot_at"`
}

// ─────────────────────────────────────────────────────────────────────────────
// RuntimeStats 运行时统计
// ─────────────────────────────────────────────────────────────────────────────
// ReleaseStats 单个发布的运行统计。
type ReleaseStats struct {
	// ReleaseID 发布 ID。
	ReleaseID string `json:"release_id"`
	// PackName 包名。
	PackName string `json:"pack_name"`
	// State 当前状态。
	State ReleaseState `json:"state"`
	// Generation 当前世代。
	Generation int `json:"generation"`
	// CohortPercent 切流比例。
	CohortPercent int `json:"cohort_percent"`
	// SealState 封堵状态。
	SealState SealState `json:"seal_state"`
	// LatestDiffRatio 最新差分比例（%）。
	LatestDiffRatio float64 `json:"latest_diff_ratio"`
	// LatestAllowDenyCount 最新 allow_deny 累计数。
	LatestAllowDenyCount int64 `json:"latest_allow_deny_count"`
}

// RuntimeStats 进程内所有活跃发布的运行统计。

type RuntimeStats struct {
	// ActiveReleaseCount 活跃发布数。
	ActiveReleaseCount int `json:"active_release_count"`
	// Releases 各发布统计。
	Releases []ReleaseStats `json:"releases"`
	// EngineBufLen 双评缓冲区积压。
	EngineBufLen int `json:"engine_buf_len"`
	// CollectedAt 采集时间。
	CollectedAt time.Time `json:"collected_at"`
}

// ─────────────────────────────────────────────────────────────────────────────
// AdminOps 管理员操作集
// ─────────────────────────────────────────────────────────────────────────────
// AdminOps 封装所有管理员操作，依赖 Manager 提供的进程级状态。

type AdminOps struct {
	mgr    *Manager
	audits []*AdminAuditRecord // 进程内缓存（应落仓）
	idgen  func() string
	clock func() time.Time
}

// NewAdminOps 构造管理员操作集。
  func NewAdminOps(mgr *Manager) *AdminOps {
	   return &AdminOps{
		  mgr:   mgr,
		  idgen: mgr.idgen,
		   clock: mgr.clock,
	   }
  }

// BatchPin 批量将指定用户 Pin 到指定切流侧。
// side 必须为 "canary" 或 "baseline"。
// 典型用场：紧急场景下将 VIP 用户强制切回 baseline。
  func (a *AdminOps) BatchPin(releaseID, actor, side string, userIDs []string) (*BatchPinResult, error) {
	  cohort,
		  err := a.mgr.Cohorts.Get(releaseID)
	  if err != nil {
		   return nil, fmt.Errorf("BatchPin: %w", err)
	   }
	  result := &BatchPinResult{TotalRequested: len(userIDs)}
	  for _, uid := range userIDs {
		  if err := cohort.PinUser(uid, side); err != nil {
			  result.Failed++
			  result.FailedUsers = append(result.FailedUsers, uid)
		   } else {
			  result.Succeeded++
		   }
	  }
	  auditID := a.idgen()
	  result.AuditID = auditID
	   a.recordAudit(&AdminAuditRecord{
		  ID: auditID,
		   Op: AdminOpBatchPin,
		   ReleaseID: releaseID,
		  Actor: actor,
		  Detail: fmt.Sprintf(`{"side":"%s","count":%d,"succeeded":%d,"failed":%d}`,
			   side, result.TotalRequested, result.Succeeded, result.Failed),
		   CreatedAt: a.clock(),
	   })
	   return result, nil
  }

// BatchUnpin 批量移除用户的显式 Pin，还原为比例切流。

   func (a *AdminOps) BatchUnpin(releaseID, actor string, userIDs []string) (int, error) {
	  cohort,
		   err := a.mgr.Cohorts.Get(releaseID)
	  if err != nil {
		  return 0, fmt.Errorf("BatchUnpin: %w", err)
	   }
	   for _, uid := range userIDs {
		  cohort.UnpinUser(uid)
	  }
	   a.recordAudit(&AdminAuditRecord{
		   ID: a.idgen(),
		  Op: AdminOpBatchUnpin,
		   ReleaseID: releaseID,
		   Actor: actor,
		  Detail:    fmt.Sprintf(`{"count":%d}`, len(userIDs)),
		  CreatedAt: a.clock(),
	  })
	  return len(userIDs), nil
   }

// EmergencySeal 紧急封堵：立即对指定发布触发 Seal，不等聚合 Worker。
// 适用场景：运营发现高危交易模式，需在下一次聚合周期前立即阻断。

  func (a *AdminOps) EmergencySeal(
	   ctx context.Context,
	  releaseID,
	   splitDiffID, actor, reason string,
  ) (*SealAction, error) {
	  gate := a.mgr.Seals.GetOrCreate(releaseID)
	   act, err := gate.Enforce(splitDiffID, "[EMERGENCY] "+reason, actor)
	   if err != nil {
		   return nil, fmt.Errorf("EmergencySeal: %w", err)
	   }
	  if act != nil {
		   if err := a.mgr.Store.SaveSealAction(ctx, act); err != nil {
			   return nil, fmt.Errorf("EmergencySeal: save action: %w", err)
		   }
		   a.recordAudit(&AdminAuditRecord{
			  ID: a.idgen(),
			  Op:        AdminOpEmergencySeal,
			  ReleaseID: releaseID,
			  Actor: actor,
			   Detail: fmt.Sprintf(`{"reason":"%s","split_diff_id":"%s"}`, reason, splitDiffID),
			  CreatedAt: a.clock(),
		   })
	   }
	  return act, nil
   }

// UpdatePolicy 更新封堵策略（不重建 Manager，热更新 SealPolicy）。
  func (a *AdminOps) UpdatePolicy(releaseID, actor string, policy SealPolicy) error {
	  if policy.MaxAllowDenyCount < 0 || policy.MaxDiffRatio < 0 || policy.MaxDiffRatio > 100 {
		  return fmt.Errorf("%w: policy params invalid", ErrConflict)
	   }
	  a.mgr.Policy = policy
	  a.recordAudit(&AdminAuditRecord{
		  ID: a.idgen(),
		   Op: AdminOpUpdatePolicy,
		  ReleaseID: releaseID,
		   Actor: actor,
		  Detail: fmt.Sprintf(`{"max_allow_deny":%d,"max_diff_ratio":%.1f}`,
			   policy.MaxAllowDenyCount, policy.MaxDiffRatio),
		  CreatedAt: a.clock(),
	   })
	   return nil
  }

// SetPercent 变更指定发布的切流比例（并记录审计）。

  func (a *AdminOps) SetPercent(releaseID, actor string, percent int) (int, error) {
	   cohort,
		  err := a.mgr.Cohorts.Get(releaseID)
	   if err != nil {
		   return 0, fmt.Errorf("SetPercent: %w", err)
	   }
	   newGen, err := cohort.SetPercent(percent)
	  if err != nil {
		   return 0, err
	   }
	  a.recordAudit(&AdminAuditRecord{
		   ID: a.idgen(),
		  Op:        AdminOpSetPercent,
		  ReleaseID: releaseID,
		  Actor: actor,
		   Detail:    fmt.Sprintf(`{"percent":%d,"new_generation":%d}`, percent, newGen),
		  CreatedAt: a.clock(),
	   })
	  return newGen, nil
  }

// ExportSnapshot 导出指定发布的完整状态快照。
  func (a *AdminOps) ExportSnapshot(ctx context.Context, releaseID string) (*ReleaseSnapshot, error) {
	  release, err := a.mgr.Store.GetRelease(ctx, releaseID)
	  if err != nil {
		   return nil, fmt.Errorf("ExportSnapshot: %w", err)
	   }
	  snap := &ReleaseSnapshot{
		  Release:   release,
		   SealState: SealOpen,
		  SnapshotAt: a.clock(),
	   }
	   // Cohort。
	  if cohort, err := a.mgr.Cohorts.Get(releaseID); err == nil {
		   cs := cohort.Snapshot()
		   snap.Cohort = &cs
	   }
	  // Seal。
	  if gate := a.mgr.Seals.Get(releaseID); gate != nil {
		  snap.SealState = gate.State()
	   }
	   // Seal actions。
	   if actions, err := a.mgr.Store.ListSealActions(ctx, releaseID); err == nil {
		   snap.SealActions = actions
	  }
	   // Rollback proof。
	  if proof, err := a.mgr.Rollback.GetProof(ctx, releaseID); err == nil {
		  snap.RollbackProof = proof
	   }
	   // Latest split diff。
	   if diffs, err := a.mgr.Store.ListSplitDiffs(ctx, releaseID, 1); err == nil && len(diffs) > 0 {
		  snap.LatestSplitDiff = diffs[0]
	  }
	   a.recordAudit(&AdminAuditRecord{
		   ID: a.idgen(),
		  Op:        AdminOpExportSnapshot,
		   ReleaseID: releaseID,
		   Actor: "system",
		  Detail:    fmt.Sprintf(`{"state":"%s"}`, release.State),
		  CreatedAt: a.clock(),
	  })
	  return snap, nil
   }

// Stats 采集进程内所有活跃发布的运行统计。
   func (a *AdminOps) Stats(ctx context.Context) *RuntimeStats {
	  cohortSnapshots := a.mgr.Cohorts.All()
	  stats := &RuntimeStats{
		  ActiveReleaseCount: len(cohortSnapshots),
		   CollectedAt:        a.clock(),
	  }
	  if a.mgr.Engine != nil {
		  stats.EngineBufLen = a.mgr.Engine.BufLen()
	  }
	  for _, cs := range cohortSnapshots {
		   rs := ReleaseStats{
			  ReleaseID: cs.ReleaseID,
			  PackName:   cs.PackName,
			  Generation: cs.Generation,
			   CohortPercent: cs.Percent,
			  State:         StateCanary,
		  }
		   if gate := a.mgr.Seals.Get(cs.ReleaseID); gate != nil {
			   rs.SealState = gate.State()
		  } else {
			  rs.SealState = SealOpen
		   }
		   if diffs, err := a.mgr.Store.ListSplitDiffs(ctx, cs.ReleaseID, 1); err == nil && len(diffs) > 0 {
			  rs.LatestDiffRatio = diffs[0].DiffRatio
			  rs.LatestAllowDenyCount = diffs[0].AllowDenyCount
		  }
		  stats.Releases = append(stats.Releases, rs)
	   }
	   return stats
  }

// AuditLog 返回所有管理员操作审计记录（进程内缓存）。

  func (a *AdminOps) AuditLog() []*AdminAuditRecord {
	   out := make([]*AdminAuditRecord, len(a.audits))
	  copy(out, a.audits)
	  return out
  }

  func (a *AdminOps) recordAudit(r *AdminAuditRecord) {
	  if r != nil {
		  a.audits = append(a.audits, r)
	  }
   }
