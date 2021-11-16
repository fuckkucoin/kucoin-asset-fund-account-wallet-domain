package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// ─────────────────────────────────────────────────────────────────────────────
// DTO 类型
// ─────────────────────────────────────────────────────────────────────────────
// DualRelease 对应 gray_releases 行。
type DualRelease struct {
	ID string
	PackName        string
	BaselineVersion int
	CanaryVersion   int
	State            string
	CohortPercent    int
	Generation       int
	PolicyAllowDrift int
	Creator string
	CreatedAt time.Time
	UpdatedAt time.Time
	SealedAt     *time.Time
	RolledBackAt *time.Time
}

// DualDecisionRow 对应 dual_decisions 行。
type DualDecisionRow struct {
	ID string
	GrayReleaseID string
	Generation int
	UserID     string
	Asset      string
	Amount string
	PackName string
	BaselineAllow bool
	BaselineReason string
	CanaryAllow  bool
	CanaryReason string
	HasDiff bool
	DiffKind    string
	SplitDiffID string
	Applied     string
	Sealed    bool
	CreatedAt time.Time
}

// SplitDiffRow 对应 split_diffs 行。
type SplitDiffRow struct {
	ID            string
	GrayReleaseID string
	Generation  int
	WindowStart time.Time
	WindowEnd   time.Time
	TotalDecisions int64
	DiffCount      int64
	DiffRatio      float64
	AllowDenyCount int64
	DenyAllowCount  int64
	LimitDriftCount int64
	KillSwitchCount int64
	ExceedsPolicy bool
	CreatedAt time.Time
}

// SealActionRow 对应 cfg_seal_actions 行。
type SealActionRow struct {
	ID            string
	GrayReleaseID string
	SplitDiffID string
	FromState   string
	ToState     string
	Reason      string
	Actor       string
	CreatedAt time.Time
}

// RollbackProofRow 对应 cfg_rollback_proofs 行。
type RollbackProofRow struct {
	ID            string
	GrayReleaseID string
	PackName          string
	RolledBackVersion int
	RestoredVersion   int
	GenerationBefore   int
	GenerationAfter    int
	TriggerSplitDiffID string
	Actor string
	Reason    string
	CreatedAt time.Time
}

// ─────────────────────────────────────────────────────────────────────────────
// DualStore SQL 仓储
// ─────────────────────────────────────────────────────────────────────────────
// DualStore 封装五张表的读写操作。
type DualStore struct{ db *persist.SQLDB }

// NewDualStore 构造 DualStore。
   func NewDualStore(db *persist.SQLDB) *DualStore { return &DualStore{db: db} }

  func (r *DualStore) on() bool { return r != nil && r.db != nil && r.db.DB != nil }

// ─── GrayRelease ─────────────────────────────────────────────────────────────
// SaveRelease INSERT OR UPDATE gray_releases。

  func (r *DualStore) SaveRelease(ctx context.Context, rel DualRelease) error {
	  if !r.on() {
		   return ErrUnavailable
	  }
	  var sealedAt, rolledBackAt interface{}
	   if rel.SealedAt != nil {
		   sealedAt = rel.SealedAt.UTC()
	   }
	   if rel.RolledBackAt != nil {
		  rolledBackAt = rel.RolledBackAt.UTC()
	  }
	  var q string
	  if r.db.Dialect == persist.Postgres {
		   q = `INSERT INTO gray_releases
			(id, pack_name, baseline_version, canary_version, state, cohort_percent, generation,
			 policy_allow_drift, creator, created_at, updated_at, sealed_at, rolled_back_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT (id) DO UPDATE SET
			state=EXCLUDED.state, cohort_percent=EXCLUDED.cohort_percent,
			generation=EXCLUDED.generation, policy_allow_drift=EXCLUDED.policy_allow_drift,
			updated_at=EXCLUDED.updated_at, sealed_at=EXCLUDED.sealed_at,
			rolled_back_at=EXCLUDED.rolled_back_at`
	  } else {
		   q = `INSERT INTO gray_releases
			(id, pack_name, baseline_version, canary_version, state, cohort_percent, generation,
			 policy_allow_drift, creator, created_at, updated_at, sealed_at, rolled_back_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE
			state=VALUES(state), cohort_percent=VALUES(cohort_percent),
			generation=VALUES(generation), policy_allow_drift=VALUES(policy_allow_drift),
			updated_at=VALUES(updated_at), sealed_at=VALUES(sealed_at),
			rolled_back_at=VALUES(rolled_back_at)`
	  }
	   _, err := r.db.Conn(ctx).ExecContext(ctx, q,
		   rel.ID, rel.PackName, rel.BaselineVersion, rel.CanaryVersion,
		  rel.State, rel.CohortPercent, rel.Generation, rel.PolicyAllowDrift,
		   rel.Creator, rel.CreatedAt.UTC(), rel.UpdatedAt.UTC(),
		   sealedAt, rolledBackAt,
	  )
	  return err
   }

// GetRelease 按 id 查询。
  func (r *DualStore) GetRelease(ctx context.Context, id string) (DualRelease, error) {
	   if !r.on() {
		   return DualRelease{}, ErrUnavailable
	   }
	   q := `SELECT id, pack_name, baseline_version, canary_version, state,
		cohort_percent, generation, policy_allow_drift, creator, created_at, updated_at,
		sealed_at, rolled_back_at FROM gray_releases WHERE id=?`
	  var rel DualRelease
	  var sealedAt,
		   rolledBackAt sql.NullTime
	  err := r.db.Conn(ctx).QueryRowContext(ctx,
		  q, id).Scan(
		   &rel.ID, &rel.PackName, &rel.BaselineVersion, &rel.CanaryVersion, &rel.State,
		  &rel.CohortPercent, &rel.Generation, &rel.PolicyAllowDrift, &rel.Creator,
		  &rel.CreatedAt, &rel.UpdatedAt, &sealedAt, &rolledBackAt,
	  )
	  if errors.Is(err, sql.ErrNoRows) {
		   return DualRelease{}, ErrNotFound
	  }
	  if err != nil {
		  return DualRelease{}, err
	  }
	  if sealedAt.Valid {
		  t := sealedAt.Time
		   rel.SealedAt = &t
	  }
	   if rolledBackAt.Valid {
		   t := rolledBackAt.Time
		   rel.RolledBackAt = &t
	  }
	   return rel, nil
  }

// ListReleases 列出发布列表，state="" 时返回全部。
   func (r *DualStore) ListReleases(ctx context.Context, state string) ([]DualRelease, error) {
	  if !r.on() {
		  return nil, ErrUnavailable
	   }
	   state = strings.TrimSpace(state)
	  var (
		   dbRows *sql.Rows
		   err    error
	   )
	   if state == "" {
		   dbRows, err = r.db.Conn(ctx).QueryContext(ctx,
			  `SELECT id, pack_name, baseline_version, canary_version, state,
			cohort_percent, generation, policy_allow_drift, creator, created_at, updated_at,
			sealed_at, rolled_back_at FROM gray_releases ORDER BY created_at DESC`)
	  } else {
		  dbRows, err = r.db.Conn(ctx).QueryContext(ctx,
			   `SELECT id, pack_name, baseline_version, canary_version, state, 
			cohort_percent, generation, policy_allow_drift, creator, created_at, updated_at,
			sealed_at, rolled_back_at FROM gray_releases WHERE state=? ORDER BY created_at DESC`,
			   state)
	   }
	  if err != nil {
		  return nil, err
	  }
	  defer dbRows.Close()
	   out := make([]DualRelease, 0)
	  for dbRows.Next() {
		   var rel DualRelease
		   var sealedAt, rolledBackAt sql.NullTime
		  if err := dbRows.Scan(
			  &rel.ID, &rel.PackName, &rel.BaselineVersion, &rel.CanaryVersion, &rel.State,
			   &rel.CohortPercent, &rel.Generation, &rel.PolicyAllowDrift, &rel.Creator,
			  &rel.CreatedAt, &rel.UpdatedAt, &sealedAt, &rolledBackAt,
		   ); err != nil {
			  return nil, err
		  }
		   if sealedAt.Valid {
			  t := sealedAt.Time
			  rel.SealedAt = &t
		   }
		   if rolledBackAt.Valid {
			   t := rolledBackAt.Time
			  rel.RolledBackAt = &t
		  }
		   out = append(out, rel)
	   }
	  return out, dbRows.Err()
  }

// ─── DualDecision ─────────────────────────────────────────────────────────────
// SaveDecision INSERT dual_decisions（只增不改）。

   func (r *DualStore) SaveDecision(ctx context.Context, d DualDecisionRow) error {
	   if !r.on() {
		   return ErrUnavailable
	   }
	   _,
		  err := r.db.Conn(ctx).ExecContext(ctx,
		  `INSERT INTO dual_decisions 
		(id, gray_release_id, generation, user_id, asset, amount, pack_name,
		 baseline_allow, baseline_reason, canary_allow, canary_reason,
		 has_diff, diff_kind, split_diff_id, applied, sealed, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		  d.ID,
		  d.GrayReleaseID, d.Generation, d.UserID, d.Asset, d.Amount, d.PackName,
		   boolInt(d.BaselineAllow), d.BaselineReason,
		   boolInt(d.CanaryAllow),
		  d.CanaryReason,
		   boolInt(d.HasDiff), d.DiffKind, d.SplitDiffID,
		  d.Applied, boolInt(d.Sealed), d.CreatedAt.UTC(),
	  )
	   return err
  }

// ListDecisions 按发布 ID 倒序查最近 limit 条。
   func (r *DualStore) ListDecisions(ctx context.Context, releaseID string, limit int) ([]DualDecisionRow, error) {
	  if !r.on() {
		  return nil, ErrUnavailable
	  }
	   if limit <= 0 {
		  limit = 100
	  }
	   dbRows, err := r.db.Conn(ctx).QueryContext(ctx,
		   `SELECT id, gray_release_id, generation, user_id, asset, amount, pack_name, 
		baseline_allow, baseline_reason, canary_allow, canary_reason,
		has_diff, diff_kind, split_diff_id, applied, sealed, created_at
		FROM dual_decisions WHERE gray_release_id=? ORDER BY created_at DESC LIMIT ?`,
		   releaseID, limit)
	  if err != nil {
		  return nil, err
	   }
	  defer dbRows.Close()
	  out := make([]DualDecisionRow, 0, limit)
	  for dbRows.Next() {
		  var d DualDecisionRow
		  var baselineAllow, canaryAllow, hasDiff, sealed int
		  if err := dbRows.Scan(
			   &d.ID, &d.GrayReleaseID, &d.Generation, &d.UserID, &d.Asset, &d.Amount, &d.PackName,
			   &baselineAllow, &d.BaselineReason, &canaryAllow, &d.CanaryReason,
			  &hasDiff, &d.DiffKind, &d.SplitDiffID, &d.Applied, &sealed, &d.CreatedAt,
		   ); err != nil {
			  return nil, err
		   }
		   d.BaselineAllow = baselineAllow != 0
		  d.CanaryAllow = canaryAllow != 0
		  d.HasDiff = hasDiff != 0
		   d.Sealed = sealed != 0
		  out = append(out, d)
	  }
	  return out, dbRows.Err()
   }

// ─── SplitDiff ────────────────────────────────────────────────────────────────
// SaveSplitDiff INSERT OR UPDATE split_diffs。

   func (r *DualStore) SaveSplitDiff(ctx context.Context, d SplitDiffRow) error {
	   if !r.on() {
		  return ErrUnavailable
	   }
	   var q string
	  if r.db.Dialect == persist.Postgres {
		   q = `INSERT INTO split_diffs
			(id, gray_release_id, generation, window_start, window_end,
			 total_decisions, diff_count, diff_ratio,
			 allow_deny_count, deny_allow_count, limit_drift_count, kill_switch_count,
			 exceeds_policy, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT (id) DO UPDATE SET
			total_decisions=EXCLUDED.total_decisions, diff_count=EXCLUDED.diff_count,
			diff_ratio=EXCLUDED.diff_ratio, allow_deny_count=EXCLUDED.allow_deny_count,
			deny_allow_count=EXCLUDED.deny_allow_count,
			limit_drift_count=EXCLUDED.limit_drift_count,
			kill_switch_count=EXCLUDED.kill_switch_count,
			exceeds_policy=EXCLUDED.exceeds_policy`
	   } else {
		  q = `INSERT INTO split_diffs
			(id, gray_release_id, generation, window_start, window_end,
			 total_decisions, diff_count, diff_ratio,
			 allow_deny_count, deny_allow_count, limit_drift_count, kill_switch_count,
			 exceeds_policy, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE
			total_decisions=VALUES(total_decisions), diff_count=VALUES(diff_count),
			diff_ratio=VALUES(diff_ratio), allow_deny_count=VALUES(allow_deny_count),
			deny_allow_count=VALUES(deny_allow_count),
			limit_drift_count=VALUES(limit_drift_count),
			kill_switch_count=VALUES(kill_switch_count),
			exceeds_policy=VALUES(exceeds_policy)`
	  }
	   _,
		   err := r.db.Conn(ctx).ExecContext(ctx, q,
		   d.ID, d.GrayReleaseID, d.Generation,
		   d.WindowStart.UTC(), d.WindowEnd.UTC(),
		   d.TotalDecisions, d.DiffCount, d.DiffRatio,
		  d.AllowDenyCount, d.DenyAllowCount, d.LimitDriftCount, d.KillSwitchCount,
		   boolInt(d.ExceedsPolicy),
		  d.CreatedAt.UTC(),
	   )
	   return err
   }

// GetSplitDiff 按 id 查询。

  func (r *DualStore) GetSplitDiff(ctx context.Context, id string) (SplitDiffRow, error) {
	   if !r.on() {
		  return SplitDiffRow{}, ErrUnavailable
	   }
	  var d SplitDiffRow
	  var exceedsPolicy int
	   err := r.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT id, gray_release_id, generation, window_start, window_end,
		total_decisions, diff_count, diff_ratio,
		allow_deny_count, deny_allow_count, limit_drift_count, kill_switch_count,
		exceeds_policy, created_at
		FROM split_diffs WHERE id=?`, id,
	  ).Scan(
		   &d.ID, &d.GrayReleaseID, &d.Generation, &d.WindowStart, &d.WindowEnd,
		   &d.TotalDecisions, &d.DiffCount, &d.DiffRatio,
		  &d.AllowDenyCount, &d.DenyAllowCount, &d.LimitDriftCount, &d.KillSwitchCount,
		  &exceedsPolicy, &d.CreatedAt,
	  )
	  if errors.Is(err, sql.ErrNoRows) {
		  return SplitDiffRow{}, ErrNotFound
	  }
	   if err != nil {
		  return SplitDiffRow{}, err
	   }
	  d.ExceedsPolicy = exceedsPolicy != 0
	   return d, nil
  }

// ListSplitDiffs 按发布 ID 查最近 limit 条。
  func (r *DualStore) ListSplitDiffs(ctx context.Context, releaseID string, limit int) ([]SplitDiffRow, error) {
	   if !r.on() {
		  return nil, ErrUnavailable
	  }
	   if limit <= 0 {
		  limit = 50
	   }
	  dbRows, err := r.db.Conn(ctx).QueryContext(ctx,
		  `SELECT id, gray_release_id, generation, window_start, window_end,
		total_decisions, diff_count, diff_ratio,
		allow_deny_count, deny_allow_count, limit_drift_count, kill_switch_count,
		exceeds_policy, created_at
		FROM split_diffs WHERE gray_release_id=? ORDER BY created_at DESC LIMIT ?`,
		  releaseID, limit)
	   if err != nil {
		   return nil, err
	  }
	  defer dbRows.Close()
	   out := make([]SplitDiffRow, 0, limit)
	  for dbRows.Next() {
		   var d SplitDiffRow
		  var exceedsPolicy int
		   if err := dbRows.Scan(
			  &d.ID, &d.GrayReleaseID, &d.Generation, &d.WindowStart, &d.WindowEnd,
			  &d.TotalDecisions, &d.DiffCount, &d.DiffRatio,
			  &d.AllowDenyCount, &d.DenyAllowCount, &d.LimitDriftCount, &d.KillSwitchCount,
			   &exceedsPolicy, &d.CreatedAt,
		  ); err != nil {
			   return nil, err
		  }
		  d.ExceedsPolicy = exceedsPolicy != 0
		   out = append(out, d)
	   }
	   return out, dbRows.Err()
   }

// ─── SealAction ───────────────────────────────────────────────────────────────
// SaveSealAction INSERT cfg_seal_actions（只增不改）。
   func (r *DualStore) SaveSealAction(ctx context.Context, a SealActionRow) error {
	   if !r.on() {
		  return ErrUnavailable
	  }
	  _, err := r.db.Conn(ctx).ExecContext(ctx,
		  `INSERT INTO cfg_seal_actions
		(id, gray_release_id, split_diff_id, from_state, to_state, reason, actor, created_at)
		VALUES (?,?,?,?,?,?,?,?)`,
		   a.ID, a.GrayReleaseID, a.SplitDiffID,
		   a.FromState, a.ToState, a.Reason, a.Actor, a.CreatedAt.UTC(),
	   )
	   return err
  }

// ListSealActions 按发布 ID 查所有动作（时序升序）。
  func (r *DualStore) ListSealActions(ctx context.Context, releaseID string) ([]SealActionRow, error) {
	   if !r.on() {
		  return nil, ErrUnavailable
	  }
	  dbRows, err := r.db.Conn(ctx).QueryContext(ctx,
		   `SELECT id, gray_release_id, split_diff_id, from_state, to_state,
		reason, actor, created_at
		FROM cfg_seal_actions WHERE gray_release_id=? ORDER BY created_at`,
		  releaseID)
	  if err != nil {
		   return nil, err
	   }
	   defer dbRows.Close()
	  out := make([]SealActionRow, 0)
	   for dbRows.Next() {
		   var a SealActionRow
		  if err := dbRows.Scan(
			   &a.ID,
			  &a.GrayReleaseID, &a.SplitDiffID,
			  &a.FromState, &a.ToState, &a.Reason, &a.Actor, &a.CreatedAt,
		  ); err != nil {
			   return nil, err
		  }
		   out = append(out, a)
	   }
	  return out, dbRows.Err()
   }

// ─── RollbackProof ────────────────────────────────────────────────────────────
// SaveRollbackProof INSERT cfg_rollback_proofs（只增不改）。
   func (r *DualStore) SaveRollbackProof(ctx context.Context, p RollbackProofRow) error {
	   if !r.on() {
		   return ErrUnavailable
	   }
	   _, err := r.db.Conn(ctx).ExecContext(ctx,
		  `INSERT INTO cfg_rollback_proofs
		(id, gray_release_id, pack_name, rolled_back_version, restored_version,
		 generation_before, generation_after, trigger_split_diff_id, actor, reason, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		   p.ID, p.GrayReleaseID, p.PackName,
		   p.RolledBackVersion, p.RestoredVersion,
		   p.GenerationBefore,
		   p.GenerationAfter,
		  p.TriggerSplitDiffID, p.Actor, p.Reason, p.CreatedAt.UTC(),
	   )
	  return err
   }

// GetRollbackProof 按 gray_release_id 查询（一个发布最多一条证明）。
  func (r *DualStore) GetRollbackProof(ctx context.Context, releaseID string) (RollbackProofRow, error) {
	   if !r.on() {
		  return RollbackProofRow{}, ErrUnavailable
	  }
	  var p RollbackProofRow
	  err := r.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT id, gray_release_id, pack_name, rolled_back_version, restored_version,
		generation_before, generation_after, trigger_split_diff_id, actor, reason, created_at
		FROM cfg_rollback_proofs WHERE gray_release_id=? LIMIT 1`,
		  releaseID,
	   ).Scan(
		  &p.ID,
		  &p.GrayReleaseID, &p.PackName,
		  &p.RolledBackVersion, &p.RestoredVersion,
		   &p.GenerationBefore, &p.GenerationAfter,
		  &p.TriggerSplitDiffID, &p.Actor, &p.Reason, &p.CreatedAt,
	   )
	  if errors.Is(err, sql.ErrNoRows) {
		  return RollbackProofRow{}, ErrNotFound
	   }
	   return p, err
   }

// ─────────────────────────────────────────────────────────────────────────────
// 辅助
// ─────────────────────────────────────────────────────────────────────────────

   func boolInt(b bool) int {
	   if b {
		  return 1
	   }
	   return 0
   }
