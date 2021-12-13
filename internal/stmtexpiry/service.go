package stmtexpiry

import (
	"context"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/stmt"
	"github.com/rs/zerolog"
)

// Service 对账单导出过期与漂移封堵服务。
type Service struct {
	engine *Engine
	store  Store
	led *ledger.Service
	ttlScan     *TTLScanWorker
	driftPatrol *DriftPatrolWorker
	proofSeal   *ProofSealWorker
	log         zerolog.Logger
}

// ServiceConfig 服务配置。

type ServiceConfig struct {
	TTLSec      int
	ScanBatchSz int
	DriftBatchSz int
	ProofBatchSz int
}

// NewService 创建服务。led 可为 nil（纯 Store 模式下不支持漂移巡逻自动重算）。
  func NewService(store Store, led *ledger.Service, cfg ServiceConfig, log zerolog.Logger) *Service {
	   if cfg.TTLSec <= 0 {
		   cfg.TTLSec = DefaultTTLSec
	   }
	   eng := NewEngine(store,
		  log, cfg.TTLSec)
	  svc := &Service{
		   engine: eng,
		  store: store,
		  led: led,
		   log: log,
	   }
	   svc.ttlScan = NewTTLScanWorker(store, log, cfg.ScanBatchSz)
	  svc.proofSeal = NewProofSealWorker(store, log, cfg.ProofBatchSz)
	   if led != nil {
		  svc.driftPatrol = NewDriftPatrolWorker(store,
			  &ledgerRecomputer{led: led}, log, cfg.DriftBatchSz)
	   }
	   return svc
   }

// ──────────────────────────── 导出签发 ────────────────────────────
// IssueExportReq 签发导出请求。

type IssueExportReq struct {
	UserID string `json:"user_id"`
	Format string `json:"format"`
	Asset  string `json:"asset,omitempty"`
	WindowStart time.Time `json:"window_start,omitempty"`
	WindowEnd time.Time `json:"window_end,omitempty"`
	TTLSec    int       `json:"ttl_sec,omitempty"`
}

// IssueExportResp 签发导出响应。
type IssueExportResp struct {
	LeaseID string `json:"lease_id"`
	ProofID string `json:"proof_id"`
	Root string `json:"root"`
	State LeaseState `json:"state"`
	ExpiresAt time.Time `json:"expires_at"`
	Proof IssueProof `json:"proof"`
}

// IssueExport 签发导出，创建租约+证明。
  func (s *Service) IssueExport(ctx context.Context, req IssueExportReq) (IssueExportResp, error) {
	   if strings.TrimSpace(req.UserID) == "" {
		   return IssueExportResp{}, fmt.Errorf("user_id required")
	   }
	   format, err := stmt.ParseFormat(req.Format)
	  if err != nil {
		  return IssueExportResp{}, err
	  }
	   bills, err := s.fetchBills(ctx, req.UserID, req.Asset)
	  if err != nil {
		   return IssueExportResp{}, fmt.Errorf("fetch bills: %w", err)
	  }
	   result, err := s.engine.BuildAndIssueExport(ctx, BuildExportReq{
		  UserID: req.UserID,
		  Format: format,
		   Bills:  bills,
		  WindowStart: req.WindowStart,
		  WindowEnd:   req.WindowEnd,
		  TTLSec: req.TTLSec,
	   })
	   if err != nil {
		   return IssueExportResp{}, err
	   }
	  return IssueExportResp{
		  LeaseID: result.Lease.ID,
		   ProofID: result.Proof.ID,
		  Root:  result.Lease.Root,
		   State: result.Lease.State,
		   ExpiresAt: result.Lease.ExpiresAt,
		   Proof: result.Proof,
	  }, nil
   }

// ──────────────────────────── 撤销 ────────────────────────────
// RevokeExportReq 撤销导出请求。

type RevokeExportReq struct {
	LeaseID string `json:"lease_id"`
	Reason string `json:"reason"`
	Actor string `json:"actor"`
}

// RevokeExport 手动撤销导出。

  func (s *Service) RevokeExport(ctx context.Context, req RevokeExportReq) error {
	  return s.engine.RevokeExport(ctx, req.LeaseID, req.Reason, req.Actor)
  }

// ──────────────────────────── 漂移确认 ────────────────────────────
// ConfirmDriftReq 确认漂移请求。
type ConfirmDriftReq struct {
	DriftID string `json:"drift_id"`
	Resolution string `json:"resolution"`
}

// ConfirmDrift 确认漂移案例。

   func (s *Service) ConfirmDrift(ctx context.Context, req ConfirmDriftReq) error {
	  return s.engine.ConfirmDrift(ctx, req.DriftID, req.Resolution)
  }

// ──────────────────────────── 验证 ────────────────────────────
// VerifyExport 验证导出是否仍有效。

  func (s *Service) VerifyExport(ctx context.Context, leaseID string) error {
	  return s.engine.VerifyExport(ctx, leaseID)
   }

// ──────────────────────────── 查询 ────────────────────────────
// GetLease 获取租约详情。

  func (s *Service) GetLease(ctx context.Context, leaseID string) (ExportLease, error) {
	   return s.store.GetLease(ctx,
		   leaseID)
  }

// GetProof 获取证明详情。

  func (s *Service) GetProof(ctx context.Context, leaseID string) (IssueProof, error) {
	   return s.store.GetProofByLease(ctx, leaseID)
   }

// ListActiveLeases 列出用户活跃租约。

  func (s *Service) ListActiveLeases(ctx context.Context, userID string) ([]ExportLease, error) {
	   return s.store.ListActiveLeasesByUser(ctx,
		  userID)
   }

// ListDrifts 列出租约相关的漂移案例。

  func (s *Service) ListDrifts(ctx context.Context, leaseID string) ([]RootDriftCase, error) {
	  return s.store.ListDriftsByLease(ctx,
		   leaseID)
  }

// ListRevocations 列出租约相关的撤销记录。
   func (s *Service) ListRevocations(ctx context.Context, leaseID string) ([]Revocation, error) {
	  return s.store.ListRevocationsByLease(ctx, leaseID)
  }

// ──────────────────────────── Worker Job 入口 ────────────────────────────
// TTLScanJob 返回 TTL 扫描 Job。

   func (s *Service) TTLScanJob(ctx context.Context) error {
	   return s.ttlScan.Job(ctx)
  }

// DriftPatrolJob 返回漂移巡逻 Job。仅在 Ledger 可用时有效。
  func (s *Service) DriftPatrolJob(ctx context.Context) error {
	   if s.driftPatrol == nil {
		  return nil
	  }
	   return s.driftPatrol.Job(ctx)
  }

// ProofSealJob 返回证明校验 Job。

   func (s *Service) ProofSealJob(ctx context.Context) error {
	   return s.proofSeal.Job(ctx)
  }

// ──────────────────────────── 内部 ────────────────────────────

   func (s *Service) fetchBills(ctx context.Context, userID, asset string) ([]stmt.Line, error) {
	  if s.led == nil {
		  return nil, fmt.Errorf("ledger service not available")
	   }
	  bills, err := s.led.ListBills(ctx, userID, 500)
	  if err != nil {
		   return nil, err
	   }
	   rep, err := stmt.Build(userID, bills, stmt.Filter{Asset: asset})
	   if err != nil {
		   return nil, err
	   }
	   return rep.Lines, nil
   }

// ledgerRecomputer 从 Ledger 服务重算 Root。
type ledgerRecomputer struct {
	led *ledger.Service
}

   func (r *ledgerRecomputer) RecomputeRoot(ctx context.Context, userID string, oldHW int) (string, int, []int, error) {
	   bills, err := r.led.ListBills(ctx, userID, 500)
	  if err != nil {
		  return "", 0, nil, err
	   }
	   rep, rErr := stmt.Build(userID, bills, stmt.Filter{})
	  if rErr != nil {
		   return "", 0, nil, rErr
	  }
	   exp,
		  eErr := stmt.ExportReport(rep, stmt.FormatJSON)
	  if eErr != nil {
		  return "", 0, nil, eErr
	  }
	   man, mErr := stmt.BuildManifest(userID, []stmt.Export{exp})
	  if mErr != nil {
		   return "", 0, nil, mErr
	  }
	  newHW := 0
	  var newSeqs []int
	   for _, ln := range rep.Lines {
		   if ln.Seq > newHW {
			  newHW = ln.Seq
		  }
		   if ln.Seq > oldHW {
			  newSeqs = append(newSeqs, ln.Seq)
		  }
	   }
	   return man.Root, newHW, newSeqs, nil
  }
