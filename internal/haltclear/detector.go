package haltclear

import (
	"context"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/custodyhalt"
)

// ---- OpenDiffQuerier ---------------------------------------------------
// OpenDiffQuerier 只读对账差异查询接口；由对账路径实现，haltclear 不依赖其内部。
// 功能：返回指定作用域下未关差异数与差异工单 key 列表（最多 32 条）。
// 只读；不写对账工单；不触发对账流程。
type OpenDiffQuerier interface {
	QueryOpenDiffs(ctx context.Context, scope, chain, asset string) (count int, keys []string, err error)
}

// ---- FalseClearDetector ---------------------------------------------------
// FalseClearDetector 定期扫描各作用域熔断标志，发现误清场景后写入 MisClearCase。
// 误清判定：Flag.On=false（已解除）且 openDiffs > 0（差异仍开）。
// 功能：仅检测；不自动触发再熔断（由 worker.go 的 ReEngageWorker 消费）。
// 不写 custodyhalt Flag；不读写对账工单主链；不关联战争室计划。
// 不变量：同一 scope+chain+asset 同一检测周期内最多产出一条 open 案例。
type FalseClearDetector struct {
	gate      *custodyhalt.Gate
	diffQuery OpenDiffQuerier
	caseRepo CaseRepo
	// checkedScopes 若非空则只检测指定作用域；空则检测全部活跃标志 + 全局。
	checkedScopes []ScopeTarget
}

// ScopeTarget 一个要检测的作用域。
type ScopeTarget struct {
	Scope string
	Chain string
	Asset string
}

// NewFalseClearDetector 构造检测器。
// checkedScopes 为空时检测全局 + 所有活跃作用域；非空时仅检测指定目标。
   func NewFalseClearDetector(
	   gate *custodyhalt.Gate,
	   diffQuery OpenDiffQuerier,
	  caseRepo CaseRepo,
	   checkedScopes ...ScopeTarget,
  ) *FalseClearDetector {
	   return &FalseClearDetector{
		   gate: gate,
		  diffQuery: diffQuery,
		   caseRepo:      caseRepo,
		   checkedScopes: checkedScopes,
	  }
  }

// DetectResult 单次检测结果。
type DetectResult struct {
	Scope string
	Chain string
	Asset         string
	HaltOn        bool
	OpenDiffCount int
	MisclearID string // 本次新建的案例 ID；已有 open 案例时为空
	Existing   bool   // true=已有 open 案例，跳过
	Error      error
}

// Detect 执行一次误清扫描；返回所有作用域的检测结果。
// 调用方（worker）负责调度频率；本方法无状态，可并发调用。

   func (d *FalseClearDetector) Detect(ctx context.Context) []DetectResult {
	  targets := d.resolveTargets(ctx)
	  results := make([]DetectResult, 0, len(targets))
	  for _, t := range targets {
		   r := d.detectOne(ctx, t)
		   results = append(results, r)
	   }
	  return results
   }

   func (d *FalseClearDetector) detectOne(ctx context.Context, t ScopeTarget) DetectResult {
	  r := DetectResult{Scope: t.Scope,
		  Chain: t.Chain, Asset: t.Asset}
	   // 读当前熔断标志。
	  halted, err := d.gate.Halted(ctx)
	  if err != nil {
		   r.Error = fmt.Errorf("read halt flag (scope=%s): %w", t.Scope, err)
		  return r
	  }
	   r.HaltOn = halted
	   if halted {
		  // 熔断中；不检测误清（差异开放是预期的）。
		  return r
	  }
	  // 查询未关差异数。
	   count, keys, err := d.diffQuery.QueryOpenDiffs(ctx, t.Scope, t.Chain, t.Asset)
	  if err != nil {
		   r.Error = fmt.Errorf("query open diffs (scope=%s chain=%s asset=%s): %w",
			   t.Scope, t.Chain, t.Asset, err)
		  return r
	   }
	  r.OpenDiffCount = count
	   if count == 0 {
		  return r // 无差异，非误清
	  }
	   // 检查是否已有 open 案例（防止重复上报）。
	  existing, err := d.caseRepo.CountOpen(ctx, t.Scope, strings.ToUpper(t.Chain), strings.ToUpper(t.Asset))
	  if err != nil {
		   r.Error = fmt.Errorf("count open cases (scope=%s): %w", t.Scope, err)
		   return r
	  }
	   if existing > 0 {
		   r.Existing = true
		  return r
	   }
	   // 产出新的误清案例。
	  now := time.Now().UTC()
	   if len(keys) > 32 {
		   keys = keys[:32]
	   }
	  caseID := newMisclearID(t.Scope, t.Chain, t.Asset)
	  mc := &MisClearCase{
		   MisclearID: caseID,
		   Scope:      t.Scope,
		   Chain:   strings.ToUpper(t.Chain),
		  Asset:   strings.ToUpper(t.Asset),
		  Trigger: TriggerOpenDiffScan,
		  State:      MisClearStateOpen,
		   DetectedAt: now,
		  OpenDiffCount: count,
		  OpenDiffKeys: keys,
		   CreatedAt:    now,
		   UpdatedAt:    now,
	   }
	  if err := d.caseRepo.Create(ctx, mc); err != nil {
		   r.Error = fmt.Errorf("create misclear case: %w", err)
		  return r
	  }
	   r.MisclearID = caseID
	   return r
   }

// DetectFromForcedAudit 接受一条 ForcedAuditRecord 并检查是否产出误清案例。
// 调用时机：ClearForced 完成后立即调用（同步检测）。
   func (d *FalseClearDetector) DetectFromForcedAudit(
	  ctx context.Context,
	  auditID,
	  scope, chain, asset string,
	   openDiffCount int,
	  diffKeys []string,
  ) (*MisClearCase, error) {
	   if openDiffCount <= 0 {
		  return nil, nil
	   }
	  existing, err := d.caseRepo.CountOpen(ctx, scope, strings.ToUpper(chain), strings.ToUpper(asset))
	  if err != nil {
		  return nil, fmt.Errorf("count open cases: %w", err)
	   }
	  if existing > 0 {
		  return nil, nil // 已有案例，跳过
	  }
	   if len(diffKeys) > 32 {
		  diffKeys = diffKeys[:32]
	  }
	   now := time.Now().UTC()
	   caseID := newMisclearID(scope,
		   chain, asset)
	   mc := &MisClearCase{
		   MisclearID: caseID,
		  Scope: scope,
		  Chain: strings.ToUpper(chain),
		  Asset: strings.ToUpper(asset),
		  Trigger:    TriggerForcedClear,
		  State:      MisClearStateOpen,
		   DetectedAt: now,
		   OpenDiffCount: openDiffCount,
		  OpenDiffKeys:  diffKeys,
		   ForcedAuditID: auditID,
		  CreatedAt:     now,
		   UpdatedAt: now,
	   }
	  if err := d.caseRepo.Create(ctx, mc); err != nil {
		   return nil, fmt.Errorf("create misclear case from forced audit: %w", err)
	   }
	  return mc, nil
  }

// ---- 内部辅助 -------------------------------------------------------

   func (d *FalseClearDetector) resolveTargets(ctx context.Context) []ScopeTarget {
	  if len(d.checkedScopes) > 0 {
		   return d.checkedScopes
	   }
	  // 默认：全局 + 所有当前活跃作用域。
	  targets := []ScopeTarget{{Scope: "global"}}
	   flags,
		   err := d.gate.ActiveFlags(ctx)
	  if err != nil {
		  return targets
	  }
	  for _, f := range flags {
		   targets = append(targets, ScopeTarget{
			   Scope: string(f.Scope),
			   Chain: f.Chain,
			  Asset: f.Asset,
		  })
	  }
	   return targets
   }

  func newMisclearID(scope, chain, asset string) string {
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   tag := scope
	  if chain != "" {
		  tag += ":" + chain
	  }
	  if asset != "" {
		  tag += ":" + asset
	  }
	  return fmt.Sprintf("mc:%s:%d", tag, time.Now().UnixNano())
  }

// ---- NoopDiffQuerier（桩）--------------------------------------
// NoopDiffQuerier 供使用；始终返回 0 差异。
type NoopDiffQuerier struct{}

   func (NoopDiffQuerier) QueryOpenDiffs(_ context.Context, _, _, _ string) (int, []string, error) {
	  return 0, nil, nil
   }

// StaticDiffQuerier 供使用；返回固定差异数。

type StaticDiffQuerier struct {
	Count int
	Keys []string
	Err  error
}

   func (s *StaticDiffQuerier) QueryOpenDiffs(_ context.Context, _, _, _ string) (int, []string, error) {
	   return s.Count, s.Keys, s.Err
  }
