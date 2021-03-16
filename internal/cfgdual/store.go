package cfgdual

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/cfgcenter"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

// ─────────────────────────────────────────────────────────────────────────────
// Store 接口
// ─────────────────────────────────────────────────────────────────────────────
// Store 持久化接口，由 MemStore 与 SQLStore 实现。
type Store interface {
	// ── GrayRelease ──────────────────────────────────────────────────────
	SaveRelease(ctx context.Context, r *GrayRelease) error
	GetRelease(ctx context.Context, id string) (*GrayRelease, error)
	ListReleases(ctx context.Context) ([]*GrayRelease, error)
	ListActiveReleases(ctx context.Context) ([]*GrayRelease, error)
	// ── DualDecision ─────────────────────────────────────────────────────
	SaveDecision(ctx context.Context, d *DualDecision) error
	ListDecisions(ctx context.Context, releaseID string, limit int) ([]*DualDecision, error)
	// ── SplitDiff ────────────────────────────────────────────────────────
	SaveSplitDiff(ctx context.Context, d *SplitDiff) error
	GetSplitDiff(ctx context.Context, id string) (*SplitDiff, error)
	ListSplitDiffs(ctx context.Context, releaseID string, limit int) ([]*SplitDiff, error)
	// ── SealAction ───────────────────────────────────────────────────────
	SaveSealAction(ctx context.Context,
		a *SealAction) error
	ListSealActions(ctx context.Context, releaseID string) ([]*SealAction, error)
	// ── RollbackProof ────────────────────────────────────────────────────
	SaveRollbackProof(ctx context.Context, p *RollbackProof) error
	GetRollbackProof(ctx context.Context,
		releaseID string) (*RollbackProof, error)
}

// ─────────────────────────────────────────────────────────────────────────────
// MemStore 内存实现（ / 单机模式）
// ─────────────────────────────────────────────────────────────────────────────
// MemStore 所有数据存内存，进程重启丢失。仅用于与 Mem 模式。
type MemStore struct {
	mu sync.Mutex
	releases  map[string]*GrayRelease
	decisions []*DualDecision
	diffs     map[string]*SplitDiff
	actions   []*SealAction
	proofs    map[string]*RollbackProof
}

// NewMemStore 构造空内存 Store。
   func NewMemStore() *MemStore {
	  return &MemStore{
		   releases: map[string]*GrayRelease{},
		   diffs:  map[string]*SplitDiff{},
		  proofs: map[string]*RollbackProof{},
	   }
   }

  func (s *MemStore) SaveRelease(_ context.Context, r *GrayRelease) error {
	  if r == nil || strings.TrimSpace(r.ID) == "" {
		  return fmt.Errorf("%w: release id required", ErrConflict)
	  }
	   s.mu.Lock()
	  s.releases[r.ID] = r
	  s.mu.Unlock()
	  return nil
   }

   func (s *MemStore) GetRelease(_ context.Context, id string) (*GrayRelease, error) {
	  s.mu.Lock()
	  r, ok := s.releases[id]
	   s.mu.Unlock()
	   if !ok {
		  return nil, ErrNotFound
	  }
	  return r, nil
  }

  func (s *MemStore) ListReleases(_ context.Context) ([]*GrayRelease, error) {
	  s.mu.Lock()
	  out := make([]*GrayRelease, 0, len(s.releases))
	  for _, r := range s.releases {
		   out = append(out, r)
	  }
	  s.mu.Unlock()
	   return out, nil
  }

   func (s *MemStore) ListActiveReleases(_ context.Context) ([]*GrayRelease, error) {
	  s.mu.Lock()
	  out := make([]*GrayRelease, 0)
	  for _, r := range s.releases {
		  if r.State == StateCanary {
			  out = append(out, r)
		   }
	  }
	  s.mu.Unlock()
	  return out, nil
   }

   func (s *MemStore) SaveDecision(_ context.Context, d *DualDecision) error {
	   if d == nil {
		  return fmt.Errorf("%w: decision required", ErrConflict)
	   }
	   s.mu.Lock()
	  s.decisions = append(s.decisions, d)
	   s.mu.Unlock()
	  return nil
  }

   func (s *MemStore) ListDecisions(_ context.Context, releaseID string, limit int) ([]*DualDecision, error) {
	   if limit <= 0 {
		   limit = 100
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   out := make([]*DualDecision, 0, limit)
	  for i := len(s.decisions) - 1; i >= 0 && len(out) < limit; i-- {
		   if s.decisions[i].GrayReleaseID == releaseID {
			  out = append(out, s.decisions[i])
		   }
	   }
	   return out, nil
  }

   func (s *MemStore) SaveSplitDiff(_ context.Context, d *SplitDiff) error {
	  if d == nil || strings.TrimSpace(d.ID) == "" {
		  return fmt.Errorf("%w: split diff id required", ErrConflict)
	   }
	  s.mu.Lock()
	  s.diffs[d.ID] = d
	   s.mu.Unlock()
	  return nil
  }

   func (s *MemStore) GetSplitDiff(_ context.Context, id string) (*SplitDiff, error) {
	  s.mu.Lock()
	  d, ok := s.diffs[id]
	   s.mu.Unlock()
	  if !ok {
		  return nil, ErrNotFound
	   }
	  return d, nil
  }

  func (s *MemStore) ListSplitDiffs(_ context.Context, releaseID string, limit int) ([]*SplitDiff, error) {
	  if limit <= 0 {
		  limit = 50
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   out := make([]*SplitDiff, 0, limit)
	  for _, d := range s.diffs {
		  if d.GrayReleaseID == releaseID {
			  out = append(out, d)
			   if len(out) >= limit {
				   break
			  }
		  }
	  }
	  return out, nil
  }

   func (s *MemStore) SaveSealAction(_ context.Context, a *SealAction) error {
	  if a == nil {
		  return fmt.Errorf("%w: seal action required", ErrConflict)
	  }
	   s.mu.Lock()
	   s.actions = append(s.actions, a)
	   s.mu.Unlock()
	  return nil
   }

  func (s *MemStore) ListSealActions(_ context.Context, releaseID string) ([]*SealAction, error) {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  out := make([]*SealAction, 0)
	  for _, a := range s.actions {
		   if a.GrayReleaseID == releaseID {
			  out = append(out, a)
		  }
	  }
	   return out, nil
  }

  func (s *MemStore) SaveRollbackProof(_ context.Context, p *RollbackProof) error {
	   if p == nil || strings.TrimSpace(p.ID) == "" {
		  return fmt.Errorf("%w: rollback proof id required", ErrConflict)
	   }
	  s.mu.Lock()
	  s.proofs[p.GrayReleaseID] = p
	  s.mu.Unlock()
	   return nil
  }

   func (s *MemStore) GetRollbackProof(_ context.Context, releaseID string) (*RollbackProof, error) {
	   s.mu.Lock()
	   p, ok := s.proofs[releaseID]
	  s.mu.Unlock()
	  if !ok {
		  return nil, ErrNotFound
	  }
	  return p, nil
   }

// ─────────────────────────────────────────────────────────────────────────────
// SQLStore SQL 实现（模式）
// ─────────────────────────────────────────────────────────────────────────────
// SQLStore 以 sqlrepo.DualStore 为后端。

type SQLStore struct {
	repo *sqlrepo.DualStore
}

// NewSQLStore 构造 SQLStore。
   func NewSQLStore(db *persist.SQLDB) *SQLStore {
	   return &SQLStore{repo: sqlrepo.NewDualStore(db)}
   }

  func (s *SQLStore) SaveRelease(ctx context.Context, r *GrayRelease) error {
	   return s.repo.SaveRelease(ctx,
		  toDBRelease(r))
  }

  func (s *SQLStore) GetRelease(ctx context.Context, id string) (*GrayRelease, error) {
	  row, err := s.repo.GetRelease(ctx, id)
	   if err != nil {
		  return nil, mapRepoErr(err)
	  }
	  return fromDBRelease(row), nil
  }

   func (s *SQLStore) ListReleases(ctx context.Context) ([]*GrayRelease, error) {
	   rows, err := s.repo.ListReleases(ctx, "")
	  if err != nil {
		   return nil, err
	  }
	  return fromDBReleases(rows), nil
   }

  func (s *SQLStore) ListActiveReleases(ctx context.Context) ([]*GrayRelease, error) {
	  rows,
		  err := s.repo.ListReleases(ctx, string(StateCanary))
	  if err != nil {
		  return nil, err
	   }
	  return fromDBReleases(rows), nil
   }

  func (s *SQLStore) SaveDecision(ctx context.Context, d *DualDecision) error {
	  return s.repo.SaveDecision(ctx, toDBDecision(d))
  }

  func (s *SQLStore) ListDecisions(ctx context.Context, releaseID string, limit int) ([]*DualDecision, error) {
	   rows,
		   err := s.repo.ListDecisions(ctx, releaseID, limit)
	  if err != nil {
		  return nil, err
	   }
	  out := make([]*DualDecision, len(rows))
	   for i, r := range rows {
		  out[i] = fromDBDecision(r)
	   }
	  return out, nil
  }

  func (s *SQLStore) SaveSplitDiff(ctx context.Context, d *SplitDiff) error {
	   return s.repo.SaveSplitDiff(ctx,
		  toDBSplitDiff(d))
   }

   func (s *SQLStore) GetSplitDiff(ctx context.Context, id string) (*SplitDiff, error) {
	  row, err := s.repo.GetSplitDiff(ctx, id)
	   if err != nil {
		  return nil, mapRepoErr(err)
	  }
	   return fromDBSplitDiff(row), nil
  }

  func (s *SQLStore) ListSplitDiffs(ctx context.Context, releaseID string, limit int) ([]*SplitDiff, error) {
	  rows, err := s.repo.ListSplitDiffs(ctx, releaseID, limit)
	  if err != nil {
		  return nil, err
	   }
	   out := make([]*SplitDiff, len(rows))
	  for i, r := range rows {
		   out[i] = fromDBSplitDiff(r)
	  }
	   return out, nil
   }

  func (s *SQLStore) SaveSealAction(ctx context.Context, a *SealAction) error {
	   return s.repo.SaveSealAction(ctx, toDBSealAction(a))
  }

   func (s *SQLStore) ListSealActions(ctx context.Context, releaseID string) ([]*SealAction, error) {
	   rows, err := s.repo.ListSealActions(ctx, releaseID)
	   if err != nil {
		  return nil, err
	  }
	   out := make([]*SealAction, len(rows))
	  for i, r := range rows {
		  out[i] = fromDBSealAction(r)
	   }
	   return out, nil
   }

  func (s *SQLStore) SaveRollbackProof(ctx context.Context, p *RollbackProof) error {
	  return s.repo.SaveRollbackProof(ctx, toDBProof(p))
  }

   func (s *SQLStore) GetRollbackProof(ctx context.Context,
	   releaseID string) (*RollbackProof, error) {
	  row, err := s.repo.GetRollbackProof(ctx, releaseID)
	   if err != nil {
		   return nil, mapRepoErr(err)
	  }
	  return fromDBProof(row), nil
  }

// ─────────────────────────────────────────────────────────────────────────────
// 类型转换辅助函数
// ─────────────────────────────────────────────────────────────────────────────

   func mapRepoErr(err error) error {
	  if err == sqlrepo.ErrNotFound {
		   return ErrNotFound
	  }
	  return err
  }

  func toDBRelease(r *GrayRelease) sqlrepo.DualRelease {
	   if r == nil {
		  return sqlrepo.DualRelease{}
	  }
	   return sqlrepo.DualRelease{
		   ID:              r.ID,
		  PackName:        r.PackName,
		  BaselineVersion: r.BaselineVersion,
		  CanaryVersion: r.CanaryVersion,
		   State:         string(r.State),
		   CohortPercent: r.CohortPercent,
		  Generation:    r.Generation,
		  PolicyAllowDrift: r.PolicyAllowDrift,
		  Creator:   r.Creator,
		   CreatedAt: r.CreatedAt,
		   UpdatedAt: r.UpdatedAt,
		  SealedAt:     r.SealedAt,
		  RolledBackAt: r.RolledBackAt,
	  }
   }

  func fromDBRelease(r sqlrepo.DualRelease) *GrayRelease {
	   return &GrayRelease{
		   ID: r.ID,
		  PackName: r.PackName,
		   BaselineVersion: r.BaselineVersion,
		   CanaryVersion:   r.CanaryVersion,
		  State:         ReleaseState(r.State),
		   CohortPercent: r.CohortPercent,
		  Generation: r.Generation,
		   PolicyAllowDrift: r.PolicyAllowDrift,
		  Creator:          r.Creator,
		  CreatedAt: r.CreatedAt,
		   UpdatedAt: r.UpdatedAt,
		  SealedAt:  r.SealedAt,
		   RolledBackAt: r.RolledBackAt,
	  }
   }

  func fromDBReleases(rows []sqlrepo.DualRelease) []*GrayRelease {
	   out := make([]*GrayRelease,
		  len(rows))
	   for i, r := range rows {
		  out[i] = fromDBRelease(r)
	   }
	   return out
   }

  func toDBDecision(d *DualDecision) sqlrepo.DualDecisionRow {
	  if d == nil {
		  return sqlrepo.DualDecisionRow{}
	  }
	  return sqlrepo.DualDecisionRow{
		  ID: d.ID,
		  GrayReleaseID: d.GrayReleaseID,
		   Generation:    d.Generation,
		   UserID:        d.UserID,
		  Asset:         d.Asset,
		   Amount:        d.Amount,
		  PackName: d.PackName,
		  BaselineAllow: d.BaselineAllow,
		   BaselineReason: d.BaselineReason,
		   CanaryAllow:  d.CanaryAllow,
		   CanaryReason: d.CanaryReason,
		  HasDiff:  d.HasDiff,
		  DiffKind: string(d.DiffKind),
		   SplitDiffID: d.SplitDiffID,
		  Applied:     d.Applied,
		   Sealed:      d.Sealed,
		   CreatedAt:   d.CreatedAt,
	   }
   }

   func fromDBDecision(r sqlrepo.DualDecisionRow) *DualDecision {
	  return &DualDecision{
		  ID: r.ID,
		   GrayReleaseID:  r.GrayReleaseID,
		   Generation:     r.Generation,
		   UserID:         r.UserID,
		  Asset:          r.Asset,
		   Amount:         r.Amount,
		   PackName:       r.PackName,
		  BaselineAllow:  r.BaselineAllow,
		   BaselineReason: r.BaselineReason,
		  CanaryAllow:  r.CanaryAllow,
		   CanaryReason: r.CanaryReason,
		  HasDiff: r.HasDiff,
		  DiffKind: DiffKind(r.DiffKind),
		   SplitDiffID: r.SplitDiffID,
		  Applied:   r.Applied,
		   Sealed:    r.Sealed,
		  CreatedAt: r.CreatedAt,
	   }
   }

   func toDBSplitDiff(d *SplitDiff) sqlrepo.SplitDiffRow {
	   if d == nil {
		  return sqlrepo.SplitDiffRow{}
	  }
	  return sqlrepo.SplitDiffRow{
		   ID:            d.ID,
		  GrayReleaseID: d.GrayReleaseID,
		  Generation:  d.Generation,
		   WindowStart: d.WindowStart,
		   WindowEnd: d.WindowEnd,
		  TotalDecisions: d.TotalDecisions,
		   DiffCount:      d.DiffCount,
		  DiffRatio:      d.DiffRatio,
		   AllowDenyCount:  d.AllowDenyCount,
		  DenyAllowCount:  d.DenyAllowCount,
		   LimitDriftCount: d.LimitDriftCount,
		   KillSwitchCount: d.KillSwitchCount,
		   ExceedsPolicy:   d.ExceedsPolicy,
		   CreatedAt:       d.CreatedAt,
	  }
  }

   func fromDBSplitDiff(r sqlrepo.SplitDiffRow) *SplitDiff {
	   return &SplitDiff{
		  ID: r.ID,
		  GrayReleaseID: r.GrayReleaseID,
		   Generation:    r.Generation,
		   WindowStart:    r.WindowStart,
		  WindowEnd:      r.WindowEnd,
		   TotalDecisions: r.TotalDecisions,
		   DiffCount: r.DiffCount,
		   DiffRatio:       r.DiffRatio,
		  AllowDenyCount:  r.AllowDenyCount,
		  DenyAllowCount:  r.DenyAllowCount,
		   LimitDriftCount: r.LimitDriftCount,
		   KillSwitchCount: r.KillSwitchCount,
		   ExceedsPolicy:   r.ExceedsPolicy,
		   CreatedAt:       r.CreatedAt,
	  }
   }

   func toDBSealAction(a *SealAction) sqlrepo.SealActionRow {
	  if a == nil {
		   return sqlrepo.SealActionRow{}
	   }
	  return sqlrepo.SealActionRow{
		  ID: a.ID,
		  GrayReleaseID: a.GrayReleaseID,
		   SplitDiffID:   a.SplitDiffID,
		  FromState:     string(a.FromState),
		  ToState: string(a.ToState),
		   Reason:    a.Reason,
		   Actor:     a.Actor,
		   CreatedAt: a.CreatedAt,
	  }
  }

  func fromDBSealAction(r sqlrepo.SealActionRow) *SealAction {
	  return &SealAction{
		  ID: r.ID,
		   GrayReleaseID: r.GrayReleaseID,
		   SplitDiffID: r.SplitDiffID,
		  FromState: SealState(r.FromState),
		  ToState:   SealState(r.ToState),
		  Reason:    r.Reason,
		  Actor:     r.Actor,
		   CreatedAt: r.CreatedAt,
	   }
   }

   func toDBProof(p *RollbackProof) sqlrepo.RollbackProofRow {
	   if p == nil {
		   return sqlrepo.RollbackProofRow{}
	  }
	  return sqlrepo.RollbackProofRow{
		   ID: p.ID,
		  GrayReleaseID:     p.GrayReleaseID,
		   PackName:          p.PackName,
		  RolledBackVersion: p.RolledBackVersion,
		  RestoredVersion:   p.RestoredVersion,
		  GenerationBefore:  p.GenerationBefore,
		  GenerationAfter: p.GenerationAfter,
		  TriggerSplitDiffID: p.TriggerSplitDiffID,
		   Actor:              p.Actor,
		  Reason: p.Reason,
		  CreatedAt: p.CreatedAt,
	   }
   }

   func fromDBProof(r sqlrepo.RollbackProofRow) *RollbackProof {
	   return &RollbackProof{
		   ID:                r.ID,
		   GrayReleaseID:     r.GrayReleaseID,
		  PackName:          r.PackName,
		  RolledBackVersion: r.RolledBackVersion,
		  RestoredVersion: r.RestoredVersion,
		   GenerationBefore: r.GenerationBefore,
		  GenerationAfter: r.GenerationAfter,
		  TriggerSplitDiffID: r.TriggerSplitDiffID,
		   Actor:              r.Actor,
		   Reason:             r.Reason,
		   CreatedAt:          r.CreatedAt,
	   }
  }

// ─────────────────────────────────────────────────────────────────────────────
// Manager 顶层协调器（进程入口初始化用）
// ─────────────────────────────────────────────────────────────────────────────
// Manager 整合 Engine / Cohort / Seal / Rollback，是 HTTP API 的依赖入口。
type Manager struct {
	Store Store
	Engine  *Engine
	Cohorts *CohortRegistry
	Seals   *SealRegistry
	Rollback *RollbackManager
	Policy   SealPolicy
	idgen func() string
	clock func() time.Time
}

// NewManager 构造 Manager。
// db 为 nil 时使用 MemStore（Mem 模式）。

   func NewManager(cfgStore *cfgcenter.Store, db *persist.SQLDB, idgen func() string) *Manager {
	  var st Store
	   if db != nil && db.DB != nil {
		   st = NewSQLStore(db)
	  } else {
		   st = NewMemStore()
	   }
	  cohorts := NewCohortRegistry()
	  seals := NewSealRegistry(idgen)
	   pol := DefaultSealPolicy()
	   engine := NewEngine(cfgStore, cohorts, seals, 4096, idgen)
	  rollback := NewRollbackManager(st, cohorts, seals, engine, idgen)
	   return &Manager{
		   Store:  st,
		  Engine: engine,
		   Cohorts: cohorts,
		  Seals: seals,
		  Rollback: rollback,
		  Policy:   pol,
		   idgen:    idgen,
		   clock:    func() time.Time { return time.Now().UTC() },
	}
}

// Bootstrap 从持久化层加载活跃发布并重建进程级注册表。
// 应在服务启动时调用一次。
   func (m *Manager) Bootstrap(ctx context.Context) error {
	  if m == nil || m.Store == nil {
		  return nil
	   }
	   releases,
		  err := m.Store.ListActiveReleases(ctx)
	  if err != nil {
		  return fmt.Errorf("cfgdual bootstrap: %w", err)
	   }
	  for _, r := range releases {
		   cohort := NewCohort(r.ID, r.PackName, r.CohortPercent, r.Generation)
		  m.Cohorts.Register(cohort)
		  m.Seals.GetOrCreate(r.ID)
		   m.Engine.RegisterRelease(r)
	   }
	  return nil
   }

// ActivateRelease 将草稿发布迁移到 canary 状态并注册 Cohort/SealGate。

  func (m *Manager) ActivateRelease(ctx context.Context, r *GrayRelease) error {
	  if r.State != StateDraft {
		   return fmt.Errorf("%w: release %s must be draft to activate", ErrInvalidState, r.ID)
	   }
	  if r.CanaryVersion <= r.BaselineVersion {
		   return fmt.Errorf("%w: canary version must be > baseline version", ErrVersionRequired)
	   }
	  now := m.clock()
	   r.State = StateCanary
	  r.UpdatedAt = now
	  if err := m.Store.SaveRelease(ctx, r); err != nil {
		  r.State = StateDraft
		  return err
	   }
	  cohort := NewCohort(r.ID, r.PackName, r.CohortPercent, r.Generation)
	  m.Cohorts.Register(cohort)
	   m.Seals.GetOrCreate(r.ID)
	  m.Engine.RegisterRelease(r)
	  return nil
   }

// CreateRelease 创建草稿发布。

  func (m *Manager) CreateRelease(ctx context.Context, req CreateReleaseRequest) (*GrayRelease, error) {
	  if strings.TrimSpace(req.PackName) == "" {
		   return nil, ErrPackRequired
	  }
	   if req.BaselineVersion <= 0 || req.CanaryVersion <= req.BaselineVersion {
		   return nil, ErrVersionRequired
	   }
	  now := m.clock()
	  r := &GrayRelease{
		   ID:       m.idgen(),
		  PackName: req.PackName,
		   BaselineVersion: req.BaselineVersion,
		  CanaryVersion:   req.CanaryVersion,
		  State: StateDraft,
		   CohortPercent:    req.CohortPercent,
		  Generation:       1,
		   PolicyAllowDrift: req.PolicyAllowDrift,
		  Creator: req.Creator,
		  CreatedAt: now,
		   UpdatedAt: now,
	   }
	  if r.PolicyAllowDrift <= 0 {
		   r.PolicyAllowDrift = 20
	  }
	  return r, m.Store.SaveRelease(ctx, r)
  }

// CreateReleaseRequest 创建发布的请求参数。
type CreateReleaseRequest struct {
	PackName string `json:"pack_name"`
	BaselineVersion int `json:"baseline_version"`
	CanaryVersion   int `json:"canary_version"`
	CohortPercent   int `json:"cohort_percent"`
	PolicyAllowDrift int `json:"policy_allow_drift"`
	Creator string `json:"creator"`
}
