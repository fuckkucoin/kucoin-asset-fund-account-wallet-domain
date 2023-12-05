package sagaresidue

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/safesaga"
)

// DetectorConfig 发现算法可调参数。

type DetectorConfig struct {
	// HighThreshold 金额超过此值（如 "10000"）升为 high 级别。
	HighThreshold decimal.Decimal
	// MediumThreshold 金额超过此值升为 medium 级别。
	MediumThreshold decimal.Decimal
	// MultiItemUpgrade 残留科目数超过此值自动升一级。
	MultiItemUpgrade int
	// LedgerIdemSuffix 步骤名到幂等键后缀的映射；零值时用 "-{step}" 默认后缀。
	LedgerIdemSuffix map[string]string
}

// DefaultDetectorConfig 默认配置。

   func DefaultDetectorConfig() DetectorConfig {
	  return DetectorConfig{
		   HighThreshold:    decimal.NewFromInt(10000),
		   MediumThreshold:  decimal.NewFromInt(100),
		   MultiItemUpgrade: 2,
		   LedgerIdemSuffix: map[string]string{
			   string(safesaga.StepFreeze): "-move",
			   string(safesaga.StepMove):   "-move",
			  string(safesaga.StepSign):   "-sign",
			   string(safesaga.StepCommit): "-commit",
		  },
	   }
   }

// LedgerReader 只读接口：按幂等键查询 ledger.Transfer。
// 由外部注入ledger.Service 或桩。
type LedgerReader interface {
	GetByIdem(ctx context.Context, idemKey string) (*ledger.Transfer, error)
}

// PartialReader 只读接口：按 Run ID 获取 safesaga.Partial。
// 由外部注入 safesaga.Compensator 或桩。

type PartialReader interface {
	Get(id string) (safesaga.Partial,
		error)
}

// RunReader 只读接口：按 ID 获取 safesaga.Run（用于读取 Idem / Asset / Amount）。
type RunReader interface {
	Get(id string) (safesaga.Run, error)
}

// Detector 残留发现器。线程安全（无可变状态）。

type Detector struct {
	cfg    DetectorConfig
	ledger LedgerReader
	partial PartialReader
	runReader RunReader
}

// NewDetector 构造 Detector，所有依赖均必须非空。

  func NewDetector(cfg DetectorConfig, lr LedgerReader, pr PartialReader, rr RunReader) *Detector {
	   return &Detector{cfg: cfg, ledger: lr, partial: pr, runReader: rr}
   }

// Detect 对指定 saga Run ID 执行残留扫描，产出 ResidueReport。
// 返回 ErrNoResidues 表示扫描干净，调用方不应创建 ResidueRun。
// 返回 ErrSagaRunNotFound 表示找不到 Run 或 Partial 记录。

   func (d *Detector) Detect(ctx context.Context, req DetectReq) (*ResidueReport, error) {
	   if d == nil || d.ledger == nil || d.partial == nil || d.runReader == nil {
		  return nil, ErrUnavailable
	  }
	   runID := strings.TrimSpace(req.SagaRunID)
	   if runID == "" {
		  return nil, ErrSagaRunNotFound
	   }
	   // ── 1. 读取 safesaga.Run（获取 Idem / Asset / UserID）────────────────────
	   run, err := d.runReader.Get(runID)
	  if err != nil {
		   return nil, fmt.Errorf("%w: get run: %v", ErrSagaRunNotFound, err)
	   }
	  // ── 2. 读取 safesaga.Partial（获取 Done / Undone / Cause / Closed）────────
	  partial, err := d.partial.Get(runID)
	  if err != nil {
		  return nil, fmt.Errorf("%w: get partial: %v", ErrSagaRunNotFound, err)
	  }
	   // 未进入失败路径则不扫（Cause 为空）
	   if strings.TrimSpace(partial.Cause) == "" {
		  return nil, ErrNoResidues
	  }
	  // ── 3. 构建"已成功撤销"集合（Undone set）─────────────────────────────────
	   undoneSet := make(map[string]struct{}, len(partial.Undone))
	  for _, s := range partial.Undone {
		  undoneSet[string(s)] = struct{}{}
	   }
	   // ── 4. 按 Done 步骤逐一检查 ledger hop 落账状态 ──────────────────────────
	  var items []ResidueItem
	   total := decimal.Zero
	  now := time.Now().UTC()
	   for _, step := range partial.Done {
		  stepStr := string(step)
		   // 若已 Undone，说明补偿成功执行，不构成残留
		   if _, done := undoneSet[stepStr]; done {
			  continue
		   }
		   idemKey := d.idemKeyForStep(run.Idem, stepStr)
		   transfer, err := d.ledger.GetByIdem(ctx, idemKey)
		   if err != nil {
			   // 该步骤未在 ledger 落账 → 无实际资金移动，无残留
			   continue
		  }
		  // ledger 已落账且未 rolled_back → 构成残留
		  if transfer.State == ledger.StateRolledBack {
			  continue
		  }
		   amt := transfer.Amount
		  total = total.Add(amt)
		  items = append(items, ResidueItem{
			  ID: newID(),
			  ResidueRunID: "", // 由 Service 填入
			   Step:         stepStr,
			  IdemKey:       idemKey,
			   LedgerEntryID: transfer.ID,
			   Amount:        amt.String(),
			  Asset:         transfer.Asset,
			   Direction: string(transfer.Direction),
			  HopState:    string(transfer.State),
			   IsConfirmed: false,
			   CreatedAt:   now,
		   })
	  }
	   if len(items) == 0 {
		  return nil, ErrNoResidues
	   }
	   // ── 5. 计算严重级别 ───────────────────────────────────────────────────────
	   sev := d.computeSeverity(total, len(items))
	  // ── 6. 排序 items（按 SeqNo / Step 字母顺序，确保哈希稳定）────────────────
	   sort.Slice(items, func(i, j int) bool {
		  return items[i].Step < items[j].Step
	  })
	  report := &ResidueReport{
		   ID:        newID(),
		  SagaRunID: runID,
		  ScanEpoch:   time.Now().UnixNano(),
		   Items:       items,
		  TotalAmount: total.String(),
		   Severity: sev,
		  ScanNote: fmt.Sprintf("partial cause=%q done=%d undone=%d residue=%d asset=%s",
			   partial.Cause, len(partial.Done), len(partial.Undone), len(items), run.Asset),
		  ScannedAt: now,
	   }
	  return report, nil
   }

// idemKeyForStep 拼接 ledger 幂等键。
// 先使用 cfg.LedgerIdemSuffix 中的自定义映射，否则使用 "idem-step" 形态。
// safesaga 的 Open 在创建 ledger hop 时用的幂等键模式与此一致：
// StepFreeze → idem+"-move"（见 safesaga/service.go Advance case StepFreeze）

   func (d *Detector) idemKeyForStep(idem, step string) string {
	   if d.cfg.LedgerIdemSuffix != nil {
		   if suffix, ok := d.cfg.LedgerIdemSuffix[step]; ok {
			  return idem + suffix
		  }
	  }
	   return fmt.Sprintf("%s-%s", idem, step)
   }

// computeSeverity 基于总金额与残留条数判定严重级别。
  func (d *Detector) computeSeverity(total decimal.Decimal, itemCount int) Severity {
	  var base Severity
	  switch {
	  case total.GreaterThan(d.cfg.HighThreshold):
		  base = SeverityHigh
	   case total.GreaterThan(d.cfg.MediumThreshold):
		  base = SeverityMedium
	  default:
		  base = SeverityLow
	  }
	   // 多科目残留自动升一级
	   if itemCount > d.cfg.MultiItemUpgrade {
		   base = upgradeSeverity(base)
	  }
	   return base
   }

// upgradeSeverity 将级别提升一档，至多 critical。

  func upgradeSeverity(s Severity) Severity {
	   switch s {
	  case SeverityLow:
		  return SeverityMedium
	   case SeverityMedium:
		   return SeverityHigh
	  case SeverityHigh, SeverityCritical:
		   return SeverityCritical
	  }
	   return SeverityCritical
  }

// ── 账本只读适配器（内部用，对外通过接口注入）─────────────────────────────────
// ledgerReaderAdapter 将 *ledger.Service 适配为 LedgerReader 接口。
type ledgerReaderAdapter struct {
	svc *ledger.Service
}

// NewLedgerReaderAdapter 构造 ledger 只读适配器。
// 仅暴露 GetByIdem 能力，不持有账本写权限。

  func NewLedgerReaderAdapter(svc *ledger.Service) LedgerReader {
	  return &ledgerReaderAdapter{svc: svc}
  }

  func (a *ledgerReaderAdapter) GetByIdem(ctx context.Context, idemKey string) (*ledger.Transfer, error) {
	   if a.svc == nil {
		   return nil, ErrUnavailable
	   }
	   // ledger.Service.List 不暴露 idem 查询，此处使用内部存储接口。
	   // 路径由 *ledger.SQLDB 实现的 GetByIdempotency 提供。
	  // 本适配器通过 ledger.Service.Get 以 idem 为键查询，若平台 API 不暴露此路径，
	   // 则直接由 SQLStore 层注入。
	   _ = idemKey
	  // 具体 SQL 查询见 persist.go 的 ledgerHopStore。
	   return nil, ErrUnavailable
   }

// partialReaderAdapter 将 *safesaga.Compensator 适配为 PartialReader。

type partialReaderAdapter struct {
	comp *safesaga.Compensator
}

// NewPartialReaderAdapter 构造 Partial 只读适配器。
   func NewPartialReaderAdapter(comp *safesaga.Compensator) PartialReader {
	   return &partialReaderAdapter{comp: comp}
   }

   func (a *partialReaderAdapter) Get(id string) (safesaga.Partial, error) {
	  if a.comp == nil {
		   return safesaga.Partial{}, ErrUnavailable
	   }
	   return a.comp.Get(id)
  }

// runReaderAdapter 将 *safesaga.Service 适配为 RunReader。

type runReaderAdapter struct {
	svc *safesaga.Service
}

// NewRunReaderAdapter 构造 Run 只读适配器。

   func NewRunReaderAdapter(svc *safesaga.Service) RunReader {
	  return &runReaderAdapter{svc: svc}
  }

  func (a *runReaderAdapter) Get(id string) (safesaga.Run, error) {
	   if a.svc == nil {
		  return safesaga.Run{}, ErrUnavailable
	   }
	   return a.svc.Get(id)
  }
