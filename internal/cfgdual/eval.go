package cfgdual

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/cfgcenter"
)

// ─────────────────────────────────────────────────────────────────────────────
// Engine 双评引擎
// ─────────────────────────────────────────────────────────────────────────────
// Engine 双评引擎核心对象。
// 持有一个 cfgcenter.Store 引用（读取 baseline/canary 两侧配置）
// 与进程级 CohortRegistry（决定切流侧）。

type Engine struct {
	store    *cfgcenter.Store
	registry *CohortRegistry
	// releases 按 releaseID 缓存 GrayRelease（供代次查找）。
	releases map[string]*GrayRelease
	relMu    sync.RWMutex
	seals    *SealRegistry
	buf      chan pendingDecision
	wg       sync.WaitGroup
	clock func() time.Time
	idgen func() string
}

type pendingDecision struct {
	release *GrayRelease
	dec *DualDecision
}

// NewEngine 构造双评引擎。
// bufSize 为异步落仓缓冲大小；0 使用默认值 4096。

   func NewEngine(
	  store *cfgcenter.Store,
	   registry *CohortRegistry,
	   seals *SealRegistry,
	  bufSize int,
	  idgen func() string,
) *Engine {
	if bufSize <= 0 {
		bufSize = 4096
	}
	return &Engine{
		store:    store,
		registry: registry,
		releases: map[string]*GrayRelease{},
		seals:    seals,
		buf: make(chan pendingDecision,
			bufSize),
		clock: func() time.Time { return time.Now().UTC() },
		idgen: idgen,
	}
}

// WithClock 注入时钟（用）。

  func (e *Engine) WithClock(c func() time.Time) *Engine {
	  if c != nil {
		  e.clock = c
	  }
	  return e
   }

   // RegisterRelease 向 Engine 注册发布元数据（供双评时查找代次号）。
   func (e *Engine) RegisterRelease(r *GrayRelease) {
	  if r == nil {
		   return
	  }
	   e.relMu.Lock()
	   e.releases[r.ID] = r
	  e.relMu.Unlock()
  }

// UnregisterRelease 发布终结时移除。
  func (e *Engine) UnregisterRelease(releaseID string) {
	  e.relMu.Lock()
	  delete(e.releases, releaseID)
	  e.relMu.Unlock()
  }

  func (e *Engine) getRelease(releaseID string) (*GrayRelease, bool) {
	   e.relMu.RLock()
	  r, ok := e.releases[releaseID]
	  e.relMu.RUnlock()
	  return r, ok
  }

// Eval 执行双评并返回 DualResult。
// 逻辑：
// 1. 从 CohortRegistry 找到本 pack 对应的活跃 GrayRelease；
// 若无活跃发布，降级为单侧 baseline 评估（非灰度流量）。
// 2. 调用 cfgcenter 分别 Eval baseline 版与 canary 版。
// 3. 比较结论，分类差分；构造 DualDecision 放入异步缓冲。
// 4. kill switch 任一侧打开→整请求拒绝。
// 5. allow_deny 脑裂 + Seal enforced → fail-closed。

  func (e *Engine) Eval(ctx context.Context, releaseID string, req EvalRequest) (DualResult, error) {
	  if e == nil || e.store == nil {
		  return DualResult{},
			   ErrUnavailable
	  }
	  req.Pack = strings.TrimSpace(req.Pack)
	   if req.Pack == "" {
		   req.Pack = "kyc0"
	  }
	   // ── 1. 查找活跃发布与 Cohort ──────────────────────────────────────────
	   cohort, err := e.registry.Get(releaseID)
	   if err != nil {
		   // 无活跃发布：单侧 baseline 降级。
		  return e.evalBaselineOnly(ctx, req, "no active release")
	  }
	  rel, relOk := e.getRelease(releaseID)
	   if !relOk {
		   return e.evalBaselineOnly(ctx, req, "release not registered")
	   }
	   inCanary, gen := cohort.InCanary(req.UserID, req.Asset)
	  // ── 2. 评估 baseline 侧（直接用 EvalPack 指定代次）────────────────────
	  // 先检查 cfgcenter 全局/包级 kill switch（EvalPack 不走 store 的 Applied 路径）。
	  if e.store.PackKilled(rel.PackName) {
		   side := EvalSide{Allow: false, Reason: "kill switch", Killed: true}
		  return DualResult{
			   Baseline:   side,
			   FinalAllow: false,
			  FinalReason: "kill switch: " + rel.PackName,
			  Applied:     "baseline",
		  }, ErrKillSwitch
	   }
	   baselinePack, bErr := e.store.VersionPack(rel.PackName, rel.BaselineVersion)
	   if bErr != nil {
		  return e.evalBaselineOnly(ctx, req, "baseline pack not found")
	  }
	  // 检查包级 kill switch。
	  if baselinePack.Killed {
		   side := EvalSide{Allow: false, Reason: "pack kill switch", Killed: true, Version: baselinePack.Version}
		   return DualResult{
			   Baseline: side,
			  FinalAllow: false,
			   FinalReason: "kill switch: baseline pack",
			   Applied:     "baseline",
		  }, ErrKillSwitch
	   }
	  if rel.PackName == "" {
		   baselinePack.Name = req.Pack
	   }
	   cfgReq := cfgcenter.Request{
		   Pack: req.Pack,
		   Asset:  req.Asset,
		  Amount: req.Amount,
		  TodaySum: req.TodaySum,
		   Hourly: req.Hourly,
		  Daily: req.Daily,
	  }
	  baseDec, baseErr := cfgcenter.EvalPack(baselinePack, cfgReq)
	  baseSide := EvalSide{
		   Allow: baseDec.Allow,
		   Reason:  baseDec.Reason,
		  Version: baselinePack.Version,
		  Killed:  baseDec.Applied.Killed,
	   }
	  if baseErr != nil {
		   baseSide.Killed = isKillSwitch(baseErr)
		  baseSide.Reason = baseErr.Error()
		   if !baseSide.Killed {
			  return DualResult{
				   Baseline: baseSide,
				  FinalAllow:  false,
				  FinalReason: baseErr.Error(),
				  Applied: "baseline",
			  }, fmt.Errorf("%w: baseline eval: %w", ErrUnavailable, baseErr)
		   }
	  }
	  // ── 3. kill switch baseline 侧打开 → 整请求拒绝 ──────────────────────
	   if baseSide.Killed {
		   return DualResult{
			  Baseline: baseSide,
			  FinalAllow:  false,
			  FinalReason: "kill switch: baseline",
			  Applied:     "baseline",
		   }, ErrKillSwitch
	  }
	  // ── 4. 未命中 canary：仅 baseline 生效 ───────────────────────────────
	   if !inCanary {
		   return DualResult{
			  Baseline: baseSide,
			  InCohort:    false,
			   FinalAllow:  baseSide.Allow,
			  FinalReason: baseSide.Reason,
			  Applied:     "baseline",
		  }, nil
	  }
	   // ── 5. 评估 canary 侧（直接用 EvalPack 指定代次）────────────────────
	  canaryPack, cErr := e.store.VersionPack(rel.PackName, rel.CanaryVersion)
	  if cErr != nil {
		   // canary pack 找不到：以 baseline 为准（fail-safe）
		   return DualResult{
			   Baseline: baseSide,
			   FinalAllow:  baseSide.Allow,
			   FinalReason: "canary pack not found: " + cErr.Error(),
			   Applied:     "baseline",
		   }, nil
	   }
	   canaryDec, canaryErr := cfgcenter.EvalPack(canaryPack, cfgReq)
	  canarySide := EvalSide{
		  Allow: canaryDec.Allow,
		   Reason:  canaryDec.Reason,
		  Version: canaryPack.Version,
		  Killed: canaryDec.Applied.Killed,
	   }
	   if canaryErr != nil {
		   canarySide.Killed = isKillSwitch(canaryErr)
		  canarySide.Reason = canaryErr.Error()
	   }
	   // ── 6. kill switch canary 侧打开 → 整请求拒绝 ────────────────────────
	  if canarySide.Killed {
		  result := DualResult{
			  Baseline: baseSide,
			  Canary:   &canarySide,
			   InCohort:   true,
			  HasDiff:    true,
			   DiffKind:   DiffKillSwitch,
			   FinalAllow: false,
			   FinalReason: "kill switch: canary",
			  Applied:       "seal",
			  Sealed:        true,
			   GrayReleaseID: releaseID,
		  }
		   e.enqueueDecision(releaseID, gen, req, baseSide, canarySide, result)
		   return result, ErrKillSwitch
	   }
	   // ── 7. 差分分析 ───────────────────────────────────────────────────────
	  diffKind, hasDiff := classifyDiff(baseSide, canarySide)
	  // ── 8. 封堵检查：Seal enforced → 所有命中 canary 的请求 fail-closed ─────
	  // 一旦封堵闸 enforced，无论本次请求是否产生差分，均 fail-closed。
	  if e.seals != nil && e.seals.IsEnforced(releaseID) {
		  result := DualResult{
			  Baseline:    baseSide,
			   Canary:      &canarySide,
			  InCohort:    true,
			   HasDiff:     hasDiff,
			  DiffKind:    diffKind,
			  FinalAllow:  false,
			   FinalReason: "split-brain seal enforced: fail-closed",
			  Applied:     "seal",
			   Sealed:        true,
			  GrayReleaseID: releaseID,
		   }
		   e.enqueueDecision(releaseID, gen, req, baseSide, canarySide, result)
		   return result, ErrSealEnforced
	   }
	   // ── 9. 最终决策：有差分时以 baseline 为准（保守策略）─────────────────
	   finalAllow := baseSide.Allow
	  finalReason := baseSide.Reason
	   applied := "baseline"
	   if !hasDiff {
		   // 双侧一致：canary 生效（灰度目的）。
		   finalAllow = canarySide.Allow
		   finalReason = canarySide.Reason
		  applied = "canary"
	  }
	   result := DualResult{
		   Baseline: baseSide,
		   Canary:   &canarySide,
		   InCohort: true,
		  HasDiff:  hasDiff,
		  DiffKind: diffKind,
		   FinalAllow: finalAllow,
		   FinalReason:   finalReason,
		   Applied:       applied,
		   GrayReleaseID: releaseID,
	   }
	  e.enqueueDecision(releaseID, gen, req, baseSide, canarySide, result)
	  return result, nil
  }

// evalBaselineOnly 无活跃发布时的降级评估路径。
  func (e *Engine) evalBaselineOnly(ctx context.Context, req EvalRequest, reason string) (DualResult, error) {
	  baseReq := cfgcenter.Request{
		   Pack: req.Pack,
		   Asset:    req.Asset,
		  Amount:   req.Amount,
		  TodaySum: req.TodaySum,
		   Hourly: req.Hourly,
		   Daily:  req.Daily,
	  }
	  dec, err := e.store.Eval(baseReq)
	  side := EvalSide{
		  Allow: dec.Allow,
		   Reason:  dec.Reason,
		  Version: dec.Applied.Pack.Version,
		  Killed:  dec.Applied.Killed,
	  }
	   if err != nil {
		  if isKillSwitch(err) {
			   return DualResult{
				   Baseline:   side,
				   FinalAllow: false,
				  FinalReason: "kill switch: " + err.Error(),
				  Applied: "baseline",
			   }, ErrKillSwitch
		   }
		   return DualResult{
			  Baseline:   side,
			   FinalAllow: false,
			  FinalReason: err.Error(),
			  Applied: "baseline",
		  }, err
	  }
	   _ = reason // 内部诊断用
	  return DualResult{
		  Baseline: side,
		  FinalAllow:  side.Allow,
		   FinalReason: side.Reason,
		  Applied:     "baseline",
	  }, nil
   }

// enqueueDecision 将双评记录放入异步缓冲。缓冲满时丢弃（不阻塞请求链路）。

   func (e *Engine) enqueueDecision(
	   releaseID string, gen int,
	   req EvalRequest,
	  base, canary EvalSide,
	   result DualResult,
  ) {
	   if e.idgen == nil {
		  return
	   }
	  id := e.idgen()
	  now := e.clock()
	   dec := &DualDecision{
		   ID:            id,
		  GrayReleaseID: releaseID,
		  Generation:    gen,
		   UserID:        req.UserID,
		  Asset: req.Asset,
		   Amount: req.Amount,
		  PackName:       req.Pack,
		  BaselineAllow:  base.Allow,
		   BaselineReason: base.Reason,
		  CanaryAllow:  canary.Allow,
		   CanaryReason: canary.Reason,
		   HasDiff:   result.HasDiff,
		  DiffKind:  result.DiffKind,
		  Applied:   result.Applied,
		  Sealed:    result.Sealed,
		  CreatedAt: now,
	   }
	   pd := pendingDecision{dec: dec}
	   select {
	   case e.buf <- pd:
	  default:
		   // 缓冲满：丢弃，不阻塞。metrics 会记录溢出。
	  }
   }

// DrainBuf 消费缓冲区中的待落仓决策记录（由 worker 调用）。
// 最多消费 maxN 条；返回本次消费的切片。
  func (e *Engine) DrainBuf(maxN int) []*DualDecision {
	  if maxN <= 0 {
		  maxN = 256
	   }
	   out := make([]*DualDecision, 0, maxN)
	  for i := 0; i < maxN; i++ {
		  select {
		  case pd := <-e.buf:
			   out = append(out, pd.dec)
		   default:
			   return out
		   }
	   }
	  return out
   }

// BufLen 当前缓冲区积压数量（用于 metrics）。
   func (e *Engine) BufLen() int {
	  return len(e.buf)
   }

// ─────────────────────────────────────────────────────────────────────────────
// 辅助函数
// ─────────────────────────────────────────────────────────────────────────────
// classifyDiff 核对两侧结论并返回差分类型。
  func classifyDiff(base, canary EvalSide) (DiffKind, bool) {
	   if base.Killed || canary.Killed {
		   return DiffKillSwitch, true
	   }
	  if base.Allow && !canary.Allow {
		   return DiffAllow, true
	  }
	  if !base.Allow && canary.Allow {
		   return DiffDeny, true
	  }
	  // 两侧均允许但代次不同时，视为限额漂移（由调用方补充金额核对）。
	  if base.Allow && canary.Allow && base.Version != canary.Version {
		  return DiffLimit, true
	   }
	  return "", false
  }

// isKillSwitch 判断 cfgcenter 返回的错误是否因 kill switch 触发。
  func isKillSwitch(err error) bool {
	  return err != nil && strings.Contains(err.Error(), "kill switch")
   }
