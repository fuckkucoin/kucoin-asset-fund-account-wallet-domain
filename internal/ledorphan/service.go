// 孤儿封堵引擎服务编排层。
// Service 持有 Detector、GenerationManager、Sealer、Prover 的引用，
// 提供统一的 Scan / AcquireGen / Seal / ComputeProof / ExportProof 入口，
// 供 HTTP 层和 worker 层调用。
// 依赖注入：
// BindKV(kv) — 注入持久化存储；
// BindBatchObserver(bo) — 注入 ledbatch 只读观察器；
// BindLedgerObserver(lo) — 注入 ledger 只读 hop 查询器；
// BindReverseHop(fn) — 注入 ledger 冲正函数（仅 reverse 模式需要）。
// CreateDate: 2020.09.05

// Author: Cason
// Date: 2021.08.11

package ledorphan

import (
	"context"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// Service 孤儿发现与封堵引擎服务。
type Service struct {
	kv persist.KV
	gen    *GenerationManager
	detect *Detector
	seal   *Sealer
	proof  *Prover
	nowFn func() time.Time
}

// New 构造 Service（所有依赖通过 Bind* 注入）。
  func New() *Service {
	   return &Service{
		   nowFn: func() time.Time { return time.Now().UTC() },
	  }
  }

// BindKV 注入 KV 存储，并初始化所有子组件。
// 必须在 BindBatchObserver / BindLedgerObserver 之前调用。
  func (s *Service) BindKV(kv persist.KV) *Service {
	   if s == nil || kv == nil {
		   return s
	  }
	  s.kv = kv
	   s.gen = NewGenerationManager(kv)
	   s.seal = NewSealer(kv, s.gen, nil)
	   s.proof = NewProver(kv)
	   return s
   }

// BindBatchObserver 注入 ledbatch 只读观察适配器。
   func (s *Service) BindBatchObserver(bo BatchObserver) *Service {
	   if s == nil {
		  return s
	  }
	  if s.detect == nil {
		  s.detect = &Detector{nowFn: s.nowFn}
	   }
	   s.detect.batch = bo
	   return s
  }

// BindLedgerObserver 注入 ledger 只读 hop 查询适配器。
  func (s *Service) BindLedgerObserver(lo LedgerObserver) *Service {
	   if s == nil {
		  return s
	  }
	   if s.detect == nil {
		  s.detect = &Detector{nowFn: s.nowFn}
	  }
	  s.detect.ledger = lo
	  return s
   }

// BindReverseHop 注入 ledger 冲正函数（可选；不注入时 SealKindReverse 返回错误）。
   func (s *Service) BindReverseHop(fn ReverseFunc) *Service {
	   if s == nil {
		   return s
	  }
	   if s.seal != nil {
		   s.seal.reverseHop = fn
	  }
	  return s
  }

// ── 核心操作 ──────────────────────────────────────────────────────────────────
// Scan 对指定批次（或全量 submitted 批次）执行孤儿扫描，生成 OrphanReport。
// 若该批次已有未关闭报告，旧报告被标记为 superseded，产出新报告。
// 调用前不需要持有世代锁（扫描为只读操作）；世代号为 0 表示扫描阶段未锁。

  func (s *Service) Scan(ctx context.Context, req ScanReq) (*ScanResp, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	   if s.detect == nil {
		   return nil, ErrBatchObserveNil
	  }
	  if s.kv == nil {
		  return nil, ErrStorageNil
	   }
	   if _, err := validateActor(req.Actor); err != nil {
		  return nil, err
	   }
	   resp := &ScanResp{}
	   if batchID := strings.TrimSpace(req.BatchID); batchID != "" {
		   // 单批次扫描。
		  report,
			   err := s.detect.Detect(ctx, batchID, 0)
		   if err != nil {
			   return nil, err
		   }
		  if err := s.persistReport(ctx, report); err != nil {
			  return nil, fmt.Errorf("ledorphan scan: persist: %w", err)
		   }
		   resp.ScannedBatches = 1
		  resp.NewReports = 1
		  resp.TotalOrphans = report.TotalOrphans
		  resp.Reports = []OrphanReport{*report}
		  return resp, nil
	  }
	  // 全量扫描：submitted 状态批次。
	  reports, err := s.detect.ScanAll(ctx, 0, "", 100)
	   if err != nil {
		   return nil, fmt.Errorf("ledorphan scan all: %w", err)
	  }
	  for _, rep := range reports {
		   if rep == nil {
			  continue
		   }
		  if err := s.persistReport(ctx, rep); err != nil {
			   continue // 单条失败不中断
		   }
		  resp.NewReports++
		   resp.TotalOrphans += rep.TotalOrphans
		  resp.Reports = append(resp.Reports, *rep)
	   }
	  resp.ScannedBatches = len(reports)
	   return resp, nil
   }

// AcquireGen 获取批次续提世代锁（供 Resume worker 调用）。

  func (s *Service) AcquireGen(ctx context.Context, req AcquireGenReq) (*BatchGeneration, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	  if s.gen == nil {
		  return nil, ErrStorageNil
	   }
	  gen, err := s.gen.Acquire(ctx, req.BatchID, req.WorkerID)
	   if err != nil {
		   return nil, err
	  }
	  return &gen, nil
  }

// RefreshGen 续期批次世代锁心跳（供 Resume worker 循环调用）。
   func (s *Service) RefreshGen(ctx context.Context, batchID, workerID string, generation int64) error {
	  if s == nil {
		  return ErrUnavailable
	   }
	  if s.gen == nil {
		  return ErrStorageNil
	  }
	   return s.gen.Refresh(ctx, batchID, workerID, generation)
  }

// ReleaseGen 主动释放批次世代锁。
  func (s *Service) ReleaseGen(ctx context.Context, batchID, workerID string, generation int64) error {
	  if s == nil {
		  return ErrUnavailable
	  }
	   if s.gen == nil {
		  return ErrStorageNil
	  }
	   return s.gen.Release(ctx, batchID, workerID, generation)
  }

// GetGen 读取批次当前世代信息。
  func (s *Service) GetGen(ctx context.Context, batchID string) (*BatchGeneration, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	  }
	  if s.gen == nil {
		  return nil, ErrStorageNil
	   }
	   gen, err := s.gen.Get(ctx, batchID)
	   if err != nil {
		  return nil, err
	   }
	  return &gen, nil
  }

// Seal 对孤儿报告执行封堵/冲正/忽略动作（需双人授权）。
// 调用前需确保持有对应批次的世代锁。

   func (s *Service) Seal(ctx context.Context, req SealReq) (*SealAction, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	  if s.seal == nil {
		   return nil, ErrStorageNil
	  }
	  // 读取孤儿报告。
	   report,
		   err := LoadReport(ctx, s.kv, req.ReportID)
	   if err != nil {
		  return nil, err
	  }
	  action, err := s.seal.Seal(ctx, report, req)
	   if err != nil {
		   return nil, err
	   }
	  // 封堵动作完成后，若报告中所有孤儿均已处置，关闭报告。
	  if allDisposed(report.Items) {
		  _ = MarkReportClosed(ctx, s.kv, report.ReportID)
	  }
	  // 自动计算并持久化完整性证明。
	  if action.Status == SealApplied {
		  sealIdem := sealedIdemKeys(report)
		  ledgerIdem := ledgerIdemKeysFrom(report)
		   intentIdem := intentIdemKeys(report)
		  _, _ = s.proof.Compute(ctx,
			   report.BatchID, report.ReportID, action.SealID,
			   report.Generation,
			   intentIdem, ledgerIdem, sealIdem,
		   )
	   }
	  return action, nil
  }

// GetReport 按 reportID 读取孤儿报告。
  func (s *Service) GetReport(ctx context.Context, reportID string) (*OrphanReport, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	   if s.kv == nil {
		   return nil, ErrStorageNil
	   }
	  return LoadReport(ctx,
		   s.kv, reportID)
  }

// ListReports 列出孤儿报告（可按 batchID/status 过滤）。

  func (s *Service) ListReports(ctx context.Context, req ReportListReq) ([]OrphanReport, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	   if s.kv == nil {
		  return nil, ErrStorageNil
	  }
	  return ListReports(ctx, s.kv, req.BatchID, req.Status, req.Limit, req.After)
  }

// GetSeal 按 sealID 读取封堵动作。
  func (s *Service) GetSeal(ctx context.Context, sealID string) (*SealAction, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	   if s.seal == nil {
		   return nil, ErrStorageNil
	   }
	   return s.seal.GetSealAction(ctx, sealID)
   }

// GetProof 按 proofID 读取完整性证明。

   func (s *Service) GetProof(ctx context.Context, proofID string) (*IntegrityProof, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	   }
	  if s.proof == nil {
		   return nil, ErrStorageNil
	   }
	  return s.proof.Get(ctx, proofID)
  }

// ExportProof 导出批次最新完整性证明。
  func (s *Service) ExportProof(ctx context.Context, batchID string) (*IntegrityProof, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	  }
	  if s.proof == nil {
		   return nil, ErrStorageNil
	  }
	   return s.proof.Export(ctx,
		   batchID)
  }

// IsIdemBlocked 检查幂等键是否已被封堵。
   func (s *Service) IsIdemBlocked(ctx context.Context, idemKey string) (bool, error) {
	  if s == nil {
		   return false, ErrUnavailable
	  }
	   if s.seal == nil {
		   return false, ErrStorageNil
	   }
	   return s.seal.IsIdemBlocked(ctx, idemKey)
   }

// ── 内部工具 ──────────────────────────────────────────────────────────────────

  func (s *Service) persistReport(ctx context.Context, report *OrphanReport) error {
	   // 将同批次旧报告标记为 superseded。
	  _ = MarkReportSuperseded(ctx, s.kv, report.BatchID, report.ReportID)
	  return SaveReport(ctx, s.kv, report)
   }

   func validateActor(actor string) (string, error) {
	   actor = strings.TrimSpace(actor)
	   if actor == "" {
		  return "", ErrActorInvalid
	  }
	   return actor, nil
   }

   func allDisposed(items []OrphanItem) bool {
	   for _, o := range items {
		   if o.Status == OrphanDetected {
			   return false
		  }
	   }
	  return true
   }

// intentIdemKeys 从报告提取意图集合 idem keys（全部 items）。
  func intentIdemKeys(report *OrphanReport) []string {
	  keys := make([]string, 0, len(report.Items))
	   for _, o := range report.Items {
		   if o.IdempotencyKey != "" {
			  keys = append(keys, o.IdempotencyKey)
		  }
	  }
	  return keys
   }

// ledgerIdemKeysFrom 从报告中提取已落账的 idem keys（hanging 类型有 LedgerID）。
  func ledgerIdemKeysFrom(report *OrphanReport) []string {
	   keys := make([]string, 0)
	   for _, o := range report.Items {
		   if o.LedgerID != "" {
			   keys = append(keys, o.IdempotencyKey)
		  }
	  }
	   return keys
   }

// sealedIdemKeys 从报告中提取已封堵/冲正/忽略的孤儿 idem keys。
  func sealedIdemKeys(report *OrphanReport) []string {
	   keys := make([]string, 0)
	   for _, o := range report.Items {
		  switch o.Status {
		   case OrphanSealed, OrphanReversed, OrphanIgnored:
			   keys = append(keys, o.IdempotencyKey)
		  }
	   }
	   return keys
  }
