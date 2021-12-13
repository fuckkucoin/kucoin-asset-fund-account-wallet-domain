package stmtexpiry

import (
	"context"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/stmt"
	"github.com/rs/zerolog"
)

// DefaultTTLSec 默认导出租约 TTL（秒）。
const DefaultTTLSec = 3600

// DefaultMutexTTL 默认互斥锁 TTL。
const DefaultMutexTTL = 30 * time.Second

// Engine 导出过期与漂移封堵引擎。

type Engine struct {
	store Store
	log    zerolog.Logger
	ttlSec int
}

// NewEngine 创建引擎实例。
   func NewEngine(store Store, log zerolog.Logger, ttlSec int) *Engine {
	  if ttlSec <= 0 {
		   ttlSec = DefaultTTLSec
	   }
	  return &Engine{store: store, log: log, ttlSec: ttlSec}
  }

// ──────────────────────────── 导出签发 ────────────────────────────
// BuildExportReq 构建导出请求参数。
type BuildExportReq struct {
	UserID string
	Format stmt.Format
	Bills  []stmt.Line
	WindowStart time.Time
	WindowEnd   time.Time
	TTLSec      int
}

// BuildExportResult 构建导出结果。

type BuildExportResult struct {
	Lease ExportLease
	Proof IssueProof
}

// BuildAndIssueExport 构建导出、签发租约并生成证明。
// 整个流程受互斥锁保护，同一用户同时只有一个进程可签发。

  func (e *Engine) BuildAndIssueExport(ctx context.Context, req BuildExportReq) (BuildExportResult, error) {
	   if strings.TrimSpace(req.UserID) == "" {
		   return BuildExportResult{}, fmt.Errorf("user_id required")
	  }
	  if len(req.Bills) == 0 {
		  return BuildExportResult{}, fmt.Errorf("no bills to export")
	   }
	   ttl := req.TTLSec
	   if ttl <= 0 {
		  ttl = e.ttlSec
	   }
	  highWater := maxSeq(req.Bills)
	   rep := stmt.Report{UserID: req.UserID, Lines: req.Bills, Count: len(req.Bills)}
	   format := req.Format
	  if format == "" {
		  format = stmt.FormatJSON
	   }
	   exp, err := stmt.ExportReport(rep, format)
	   if err != nil {
		  return BuildExportResult{}, fmt.Errorf("export report: %w", err)
	   }
	  man, err := stmt.BuildManifest(req.UserID, []stmt.Export{exp})
	  if err != nil {
		  return BuildExportResult{}, fmt.Errorf("build manifest: %w", err)
	  }
	  lease := NewExportLease(req.UserID, string(format), man.Root, highWater, ttl,
		   req.WindowStart, req.WindowEnd)
	   acquired, err := e.store.TryAcquireExportMutex(ctx, req.UserID, lease.ID, DefaultMutexTTL)
	  if err != nil {
		  return BuildExportResult{}, fmt.Errorf("acquire mutex: %w", err)
	  }
	   if !acquired {
		   return BuildExportResult{}, fmt.Errorf("concurrent export in progress for user %s", req.UserID)
	  }
	   defer func() {
		   _ = e.store.ReleaseExportMutex(ctx, req.UserID, lease.ID)
	   }()
	   if err := e.store.InsertLease(ctx, lease); err != nil {
		  return BuildExportResult{}, fmt.Errorf("insert lease: %w", err)
	  }
	   proof := NewIssueProof(lease.ID, req.UserID, man.Root, ttl, req.WindowStart, req.WindowEnd)
	  if err := e.store.InsertProof(ctx, proof); err != nil {
		   return BuildExportResult{}, fmt.Errorf("insert proof: %w", err)
	  }
	   if err := lease.Issue(proof.ID); err != nil {
		  return BuildExportResult{}, fmt.Errorf("issue lease: %w", err)
	   }
	   if err := e.store.SetLeaseIssued(ctx, lease.ID, proof.ID, lease.IssuedAt, lease.ExpiresAt); err != nil {
		   return BuildExportResult{}, fmt.Errorf("persist lease issued: %w", err)
	  }
	   e.log.Info().
		  Str("lease_id", lease.ID).
		   Str("user_id", req.UserID).
		  Str("root", man.Root).
		   Int("high_water", highWater).
		   Msg("export issued")
	  return BuildExportResult{Lease: lease, Proof: proof}, nil
  }

// ──────────────────────────── 漂移检测 ────────────────────────────
// DetectDriftReq 漂移检测请求。

type DetectDriftReq struct {
	LeaseID      string
	CurrentRoot  string
	NewHighWater int
	NewBillSeqs []int
}

// DetectRootDrift 检测给定租约的 Root 是否已漂移。
// 若漂移，创建 DriftCase 并自动撤销租约。

   func (e *Engine) DetectRootDrift(ctx context.Context, req DetectDriftReq) (*RootDriftCase, error) {
	   lease,
		  err := e.store.GetLease(ctx, req.LeaseID)
	   if err != nil {
		  return nil, fmt.Errorf("get lease: %w", err)
	   }
	  if lease.State.Terminal() {
		  return nil, nil
	   }
	   if lease.Root == req.CurrentRoot {
		  return nil, nil
	  }
	   dc := NewRootDriftCase(lease.ID,
		   lease.UserID, lease.Root, req.CurrentRoot,
		   lease.HighWater, req.NewHighWater, req.NewBillSeqs)
	   if err := e.store.InsertDrift(ctx, dc); err != nil {
		   return nil, fmt.Errorf("insert drift: %w", err)
	   }
	   if err := dc.Prove(); err != nil {
		   return nil, fmt.Errorf("prove drift: %w", err)
	  }
	   if err := e.store.UpdateDriftState(ctx, dc.ID, DriftOpen, DriftProven); err != nil {
		  return nil, fmt.Errorf("update drift state: %w", err)
	  }
	  rev := NewRevocation(lease.ID,
		  lease.UserID, RevokeRootDrift,
		   fmt.Sprintf("root drift detected: %s → %s", lease.Root, req.CurrentRoot),
		   "drift_patrol")
	  rev.DriftID = dc.ID
	  if err := e.store.InsertRevocation(ctx, rev); err != nil {
		   return nil, fmt.Errorf("insert revocation: %w", err)
	  }
	   if err := e.store.SetLeaseRevoked(ctx, lease.ID, "root_drift:"+dc.ID); err != nil {
		  return nil, fmt.Errorf("revoke lease: %w", err)
	  }
	   e.log.Warn().
		  Str("lease_id", lease.ID).
		  Str("drift_id", dc.ID).
		  Str("old_root", lease.Root).
		   Str("new_root", req.CurrentRoot).
		   Msg("root drift detected and lease revoked")
	  return &dc, nil
   }

// ──────────────────────────── 确认漂移 ────────────────────────────
// ConfirmDrift 人工确认漂移案例并关闭。

   func (e *Engine) ConfirmDrift(ctx context.Context, driftID, resolution string) error {
	  dc, err := e.store.GetDrift(ctx, driftID)
	   if err != nil {
		   return fmt.Errorf("get drift: %w", err)
	   }
	  if dc.State == DriftOpen {
		  if err := e.store.UpdateDriftState(ctx, dc.ID, DriftOpen, DriftProven); err != nil {
			   return fmt.Errorf("prove drift: %w", err)
		   }
	  }
	  if err := e.store.UpdateDriftState(ctx, dc.ID, DriftProven, DriftClosed); err != nil {
		  return fmt.Errorf("close drift: %w", err)
	  }
	   e.log.Info().Str("drift_id", driftID).Str("resolution", resolution).Msg("drift confirmed and closed")
	   return nil
   }

// ──────────────────────────── 手动撤销 ────────────────────────────
// RevokeExport 手动撤销导出。
  func (e *Engine) RevokeExport(ctx context.Context, leaseID, reason, actor string) error {
	   lease, err := e.store.GetLease(ctx, leaseID)
	   if err != nil {
		  return fmt.Errorf("get lease: %w", err)
	   }
	   if lease.State.Terminal() {
		  return fmt.Errorf("lease %s already in terminal state %s", leaseID, lease.State)
	  }
	   rev := NewRevocation(leaseID,
		  lease.UserID, RevokeManual, reason, actor)
	   if err := e.store.InsertRevocation(ctx, rev); err != nil {
		  return fmt.Errorf("insert revocation: %w", err)
	   }
	  if err := e.store.SetLeaseRevoked(ctx, leaseID, "manual:"+actor); err != nil {
		  return fmt.Errorf("revoke lease: %w", err)
	  }
	   e.log.Info().
		  Str("lease_id", leaseID).
		   Str("actor", actor).
		   Str("reason", reason).
		  Msg("export manually revoked")
	  return nil
  }

// ──────────────────────────── 验证导出 ────────────────────────────
// VerifyExport 验证导出是否仍然有效（租约存活且证明完整）。

  func (e *Engine) VerifyExport(ctx context.Context, leaseID string) error {
	  lease, err := e.store.GetLease(ctx, leaseID)
	   if err != nil {
		  return fmt.Errorf("get lease: %w", err)
	  }
	  proof, err := e.store.GetProofByLease(ctx, leaseID)
	   if err != nil {
		  return VerifyExportIntegrity(lease, nil)
	   }
	  return VerifyExportIntegrity(lease,
		   &proof)
  }

// ──────────────────────────── 获取证明 ────────────────────────────
// GetExportProof 获取导出证明信息。
  func (e *Engine) GetExportProof(ctx context.Context, leaseID string) (IssueProof, error) {
	  return e.store.GetProofByLease(ctx, leaseID)
  }

// GetLease 获取租约信息。

  func (e *Engine) GetLease(ctx context.Context, leaseID string) (ExportLease, error) {
	  return e.store.GetLease(ctx,
		   leaseID)
  }

// ──────────────────────────── 辅助 ────────────────────────────

   func maxSeq(lines []stmt.Line) int {
	   mx := 0
	  for _, l := range lines {
		   if l.Seq > mx {
			  mx = l.Seq
		   }
	  }
	   return mx
  }
