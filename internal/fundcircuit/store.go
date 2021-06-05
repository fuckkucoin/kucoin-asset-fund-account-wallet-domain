package fundcircuit

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

// Repo 熔断总线仓储窄口。sqlrepo.FundCircuit 为默认实现。

type Repo interface {
	PutSignal(ctx context.Context,
		row sqlrepo.FundSignalRow) error
	PutCircuit(ctx context.Context, row sqlrepo.FundCircuitRow) error
	PutPolicy(ctx context.Context, row sqlrepo.FundPolicyRow) error
	PutCase(ctx context.Context, row sqlrepo.FundCaseRow) error
	PutProbe(ctx context.Context, row sqlrepo.FundProbeRow) error
	ListCircuits(ctx context.Context, limit int) ([]sqlrepo.FundCircuitRow, error)
	ListCases(ctx context.Context, state string, limit int) ([]sqlrepo.FundCaseRow, error)
	ListProbes(ctx context.Context, state string, limit int) ([]sqlrepo.FundProbeRow, error)
	LatestPolicy(ctx context.Context) (sqlrepo.FundPolicyRow,
		error)
	PruneSignals(ctx context.Context, before time.Time) (int, error)
}

// BindSQL 绑库并接上各台账的落库回调。OpenMem 不绑，只用进程内台账。
  func (b *Bus) BindSQL(db *persist.SQLDB) *Bus {
	   if b == nil || db == nil || db.DB == nil {
		   return b
	  }
	  return b.BindRepo(sqlrepo.NewFundCircuit(db))
  }

// BindRepo 绑仓储实现并挂落库回调。回调在各台账锁外执行。

   func (b *Bus) BindRepo(repo Repo) *Bus {
	  if b == nil || repo == nil {
		   return b
	  }
	  b.mu.Lock()
	   b.repo = repo
	   b.mu.Unlock()
	   ctx := context.Background()
	   b.cir.OnSave(func(c Circuit) {
		   b.persistCircuit(ctx,
			   c)
	   })
	  b.cases.OnSave(func(c Case) { b.persistCase(ctx, c) })
	  b.probes.OnSave(func(p Probe) { b.persistProbe(ctx, p) })
	   b.pol.OnPut(func(doc BlockPolicy) { b.persistPolicy(ctx, doc) })
	   return b
   }

// SQLBound 是否已绑仓储。
  func (b *Bus) SQLBound() bool {
	   if b == nil {
		   return false
	   }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  return b.repo != nil
   }

   func (b *Bus) repoOf() Repo {
	  if b == nil {
		  return nil
	  }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  return b.repo
  }

  func (b *Bus) noteRepoErr(table string, err error) {
	   if b == nil {
		  return
	  }
	   if err == nil {
		   metrics.FundCircuitPersistErr.WithLabelValues(table).Set(0)
		   return
	   }
	  metrics.FundCircuitPersistErr.WithLabelValues(table).Set(1)
	   b.mu.Lock()
	   b.repoErr = table + ": " + err.Error()
	   b.mu.Unlock()
  }

  func (b *Bus) persistSignal(ctx context.Context, sig AbuseSignal) {
	  repo := b.repoOf()
	  if repo == nil {
		   return
	  }
	   if ctx == nil {
		  ctx = context.Background()
	  }
	   keys := KeysFor(sig)
	  scopeKey := KeyGlobal
	  if len(keys) > 0 {
		  scopeKey = keys[0]
	  }
	   err := repo.PutSignal(ctx, sqlrepo.FundSignalRow{
		  ID: sig.ID, Kind: sig.Kind.String(), Path: sig.Path.String(), ScopeKey: scopeKey,
		  UserID: sig.UserID, Chain: sig.Chain, Asset: sig.Asset, Weight: sig.Weight,
		  SourceKey: sig.SourceKey, Note: sig.Note, CreatedAt: sig.At,
	  })
	  b.noteRepoErr("signals", err)
  }

   func (b *Bus) persistCircuit(ctx context.Context, c Circuit) {
	   repo := b.repoOf()
	   if repo == nil {
		   return
	   }
	  if ctx == nil {
		  ctx = context.Background()
	   }
	   err := repo.PutCircuit(ctx, sqlrepo.FundCircuitRow{
		   Key: c.Key, Scope: c.Scope.String(), UserID: c.UserID, Chain: c.Chain, Asset: c.Asset,
		  State: c.State.String(), Score: c.Score, Severity: c.Severity.String(), Reason: c.Reason,
		   Trips: c.Trips,
		   ProbesIssued: c.ProbesIssued, Forced: c.Forced, CrossPath: c.CrossPath,
		  Actor: c.Actor, CaseID: c.CaseID, PolicyVer: c.PolicyVer, OpenedAt: c.OpenedAt,
		   HalfOpenAt: c.HalfOpenAt, ClosedAt: c.ClosedAt, NextAdvanceAt: c.NextAdvanceAt,
		  UpdatedAt: c.UpdatedAt,
	   })
	   b.noteRepoErr("circuits", err)
   }

   func (b *Bus) persistCase(ctx context.Context, c Case) {
	  repo := b.repoOf()
	  if repo == nil {
		  return
	   }
	   if ctx == nil {
		  ctx = context.Background()
	  }
	   audit,
		  merr := json.Marshal(c.Audit)
	   if merr != nil {
		   audit = []byte("[]")
	   }
	  err := repo.PutCase(ctx, sqlrepo.FundCaseRow{
		   ID: c.ID, CircuitKey: c.CircuitKey, Scope: c.Scope.String(), UserID: c.UserID,
		  Chain: c.Chain,
		  Asset: c.Asset, State: c.State.String(), Score: c.Score,
		  Severity: c.Severity.String(), Reason: c.Reason, OpenedBy: c.OpenedBy,
		  Reviewer: c.Reviewer, ClearReq: c.ClearReq, ClearVotes: strings.Join(c.ClearVotes, ","),
		   AuditJSON: string(audit), CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	  })
	  b.noteRepoErr("cases", err)
   }

  func (b *Bus) persistProbe(ctx context.Context, p Probe) {
	   repo := b.repoOf()
	  if repo == nil {
		  return
	   }
	   if ctx == nil {
		   ctx = context.Background()
	  }
	  err := repo.PutProbe(ctx, sqlrepo.FundProbeRow{
		  ID: p.ID, CircuitKey: p.CircuitKey, Path: p.Path.String(), State: p.State.String(),
		  Token: p.Token, Actor: p.Actor, Round: p.Round, Note: p.Note,
		  IssuedAt: p.IssuedAt, ExpiresAt: p.ExpiresAt, DecidedAt: p.DecidedAt,
	   })
	  b.noteRepoErr("probes", err)
  }

   func (b *Bus) persistPolicy(ctx context.Context, doc BlockPolicy) {
	  repo := b.repoOf()
	  if repo == nil {
		   return
	   }
	  if ctx == nil {
		  ctx = context.Background()
	  }
	   body, merr := json.Marshal(doc)
	   if merr != nil {
		   b.noteRepoErr("policies", merr)
		  return
	  }
	  err := repo.PutPolicy(ctx, sqlrepo.FundPolicyRow{
		   Version: doc.Version, Actor: doc.Actor, Note: doc.Note,
		  DocJSON:   string(body),
		   UpdatedAt: doc.UpdatedAt,
	   })
	   b.noteRepoErr("policies", err)
  }

// HydrateReport Hydrate 结果，供 ops 探活。
type HydrateReport struct {
	SQLBound bool `json:"sql_bound"`
	Circuits int `json:"circuits"`
	Cases     int    `json:"cases"`
	Probes    int    `json:"probes"`
	PolicyVer int    `json:"policy_ver"`
	Err       string `json:"err,omitempty"`
}

// Hydrate 重启后装回熔断器、未结案件、在途探针与最新策略。
// 功能：冷启动不得因内存空表而放行已跳闸的作用域。
// 重点：装回失败保留 Err 供探活，不把失败当成未熔断。
// 风险点：策略文档代次比内存新时以库为准；库里 AutoUnfreeze 为真的历史文档一律拒绝装回。
   func (b *Bus) Hydrate(ctx context.Context) HydrateReport {
	  out := HydrateReport{}
	   if b == nil {
		  out.Err = ErrBus.Error()
		   return out
	   }
	   repo := b.repoOf()
	  if repo == nil {
		   out.PolicyVer = b.pol.Current().Version
		  return out
	  }
	   out.SQLBound = true
	  if ctx == nil {
		   ctx = context.Background()
	   }
	  if row, err := repo.LatestPolicy(ctx); err == nil {
		  var doc BlockPolicy
		  if jerr := json.Unmarshal([]byte(row.DocJSON), &doc); jerr == nil {
			  doc.Version = row.Version
			   if lerr := b.pol.Load(doc); lerr != nil {
				   out.Err = lerr.Error()
			   }
		   }
	   }
	   out.PolicyVer = b.pol.Current().Version
	  rows, err := repo.ListCircuits(ctx, 1000)
	   if err != nil {
		  out.Err = err.Error()
		   return out
	   }
	  for _, row := range rows {
		   c := Circuit{
			  Key: row.Key, Scope: Scope(row.Scope), UserID: row.UserID, Chain: row.Chain,
			  Asset: row.Asset,
			  State: CircuitState(row.State), Score: row.Score,
			   Severity: Severity(row.Severity), Reason: row.Reason, Trips: row.Trips,
			  ProbesIssued: row.ProbesIssued, Forced: row.Forced, CrossPath: row.CrossPath,
			   Actor: row.Actor,
			  CaseID: row.CaseID, PolicyVer: row.PolicyVer,
			  OpenedAt: row.OpenedAt, HalfOpenAt: row.HalfOpenAt, ClosedAt: row.ClosedAt,
			   NextAdvanceAt: row.NextAdvanceAt,
			   UpdatedAt:     row.UpdatedAt,
		   }
		  if lerr := b.cir.Load(c); lerr != nil {
			   out.Err = lerr.Error()
			   continue
		   }
		   out.Circuits++
	   }
	   for _, state := range []string{CaseOpen.String(), CaseReview.String()} {
		  crows, cerr := repo.ListCases(ctx, state, 500)
		  if cerr != nil {
			  out.Err = cerr.Error()
			  continue
		   }
		  for _, row := range crows {
			  cs := Case{
				  ID: row.ID, CircuitKey: row.CircuitKey, Scope: Scope(row.Scope), UserID: row.UserID,
				  Chain: row.Chain, Asset: row.Asset, State: CaseState(row.State), Score: row.Score,
				  Severity: Severity(row.Severity), Reason: row.Reason, OpenedBy: row.OpenedBy,
				   Reviewer: row.Reviewer, ClearReq: row.ClearReq, CreatedAt: row.CreatedAt,
				  UpdatedAt: row.UpdatedAt,
			  }
			  if row.ClearVotes != "" {
				   cs.ClearVotes = strings.Split(row.ClearVotes, ",")
			  }
			   if row.AuditJSON != "" {
				   _ = json.Unmarshal([]byte(row.AuditJSON),
					  &cs.Audit)
			   }
			   if lerr := b.cases.Load(cs); lerr != nil {
				  out.Err = lerr.Error()
				  continue
			   }
			  out.Cases++
		  }
	   }
	  prows, perr := repo.ListProbes(ctx, ProbePending.String(), 200)
	   if perr != nil {
		  out.Err = perr.Error()
		  return out
	  }
	   for _, row := range prows {
		  p := Probe{
			  ID: row.ID, CircuitKey: row.CircuitKey, Path: Path(row.Path), State: ProbeState(row.State),
			   Token: row.Token,
			   Actor: row.Actor, Round: row.Round, Note: row.Note,
			  IssuedAt: row.IssuedAt, ExpiresAt: row.ExpiresAt, DecidedAt: row.DecidedAt,
		  }
		   if lerr := b.probes.Load(p); lerr != nil {
			   out.Err = lerr.Error()
			   continue
		  }
		  out.Probes++
	   }
	   return out
  }

// PruneStore 摘除窗口外的落库信号，返回删除条数。

   func (b *Bus) PruneStore(ctx context.Context) (int, error) {
	   if b == nil {
		  return 0, ErrBus
	   }
	  repo := b.repoOf()
	   if repo == nil {
		   return 0, nil
	   }
	  if ctx == nil {
		  ctx = context.Background()
	  }
	  pol := b.pol.Current()
	  before := b.now().Add(-pol.SignalTTL)
	  n, err := repo.PruneSignals(ctx, before)
	   b.noteRepoErr("signals", err)
	  return n, err
   }
