package haltepochgate

import (
	"context"
	"log"
	"time"
)

// WorkerConfig 巡检间隔。

type WorkerConfig struct {
	WindowScanInterval time.Duration
	ProofPumpInterval  time.Duration
	ExportInterval     time.Duration
	BatchSize          int
	AutoProve bool
}

  func DefaultWorkerConfig() WorkerConfig {
	   return WorkerConfig{
		   WindowScanInterval: 15 * time.Second,
		  ProofPumpInterval:  20 * time.Second,
		   ExportInterval:     60 * time.Second,
		  BatchSize:          100,
		  AutoProve: true,
	   }
   }

// Worker 滑窗巡检泵。

type Worker struct {
	svc *Service
	cfg WorkerConfig
	log *log.Logger
}

   func NewWorker(svc *Service, cfg WorkerConfig, lg *log.Logger) *Worker {
	  if cfg.BatchSize <= 0 {
		   cfg.BatchSize = 100
	   }
	  if cfg.WindowScanInterval <= 0 {
		  cfg.WindowScanInterval = 15 * time.Second
	  }
	   if cfg.ProofPumpInterval <= 0 {
		  cfg.ProofPumpInterval = 20 * time.Second
	  }
	   if cfg.ExportInterval <= 0 {
		  cfg.ExportInterval = 60 * time.Second
	  }
	   return &Worker{svc: svc, cfg: cfg, log: lg}
  }

  func (w *Worker) Run(ctx context.Context) {
	   if w == nil || w.svc == nil {
		  return
	   }
	  scan := time.NewTicker(w.cfg.WindowScanInterval)
	   proof := time.NewTicker(w.cfg.ProofPumpInterval)
	   exp := time.NewTicker(w.cfg.ExportInterval)
	  defer scan.Stop()
	  defer proof.Stop()
	   defer exp.Stop()
	  w.tickScan(ctx)
	   for {
		   select {
		   case <-ctx.Done():
			  return
		  case <-scan.C:
			  w.tickScan(ctx)
		   case <-proof.C:
			   w.tickProof(ctx)
		  case <-exp.C:
			  w.tickExport(ctx)
		   }
	   }
   }

   func (w *Worker) tickScan(ctx context.Context) {
	  rev,
		   proofs, err := w.svc.ScanSlipWindow(ctx, w.cfg.BatchSize)
	   if err != nil {
		  w.printf("window scan: %v", err)
		  return
	   }
	  ObserveScan(rev, proofs)
	   if rev > 0 {
		   w.printf("window scan revoked=%d open_proofs=%d", rev, proofs)
	   }
   }

  func (w *Worker) tickProof(ctx context.Context) {
	   if !w.cfg.AutoProve {
		  return
	   }
	  list, err := w.svc.DrainOpenProofs(ctx, w.cfg.BatchSize)
	  if err != nil {
		   w.printf("proof pump: %v", err)
		  return
	   }
	   ObserveProofs(len(list))
  }

  func (w *Worker) tickExport(ctx context.Context) {
	  ex, err := w.svc.ExportProofs(ctx, w.cfg.BatchSize)
	  if err != nil {
		   w.printf("export: %v", err)
		  return
	   }
	  for _, p := range ex.Proofs {
		   if p.ExportedAt.IsZero() && p.State != SlipOpen {
			   _ = w.svc.MarkExported(ctx, p.ProofID)
		   }
	  }
	  ObserveExport(ex.Count)
   }

   func (w *Worker) printf(f string, args ...any) {
	   if w.log != nil {
		  w.log.Printf("[halt-epoch-gate] "+f, args...)
	   }
   }

// RunOnce 单次巡检，供。
   func (w *Worker) RunOnce(ctx context.Context) error {
	  w.tickScan(ctx)
	  w.tickProof(ctx)
	   w.tickExport(ctx)
	   return nil
   }
