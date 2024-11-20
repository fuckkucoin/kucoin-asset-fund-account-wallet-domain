package stmtexpiry

import (
	"context"
	"fmt"
	"time"
	"github.com/rs/zerolog"
)

// RootRecomputer 外部传入的 Root 重算接口。
// 引擎不直接操作 stmt.Build / stmt.BuildManifest，由调用方注入具体实现。

type RootRecomputer interface {
	RecomputeRoot(ctx context.Context,
		userID string, highWater int) (root string, newHW int, newSeqs []int, err error)
}

// TTLScanWorker 扫描已过期的 issued 租约并自动撤销。

type TTLScanWorker struct {
	store   Store
	log     zerolog.Logger
	batchSz int
}

   func NewTTLScanWorker(store Store, log zerolog.Logger, batchSz int) *TTLScanWorker {
	   if batchSz <= 0 {
		   batchSz = 100
	  }
	  return &TTLScanWorker{store: store, log: log, batchSz: batchSz}
  }

// Tick 执行一次扫描。返回本轮处理的过期租约数量。
   func (w *TTLScanWorker) Tick(ctx context.Context) (int, error) {
	  now := time.Now().UTC()
	   expired, err := w.store.ListExpiredLeases(ctx, now, w.batchSz)
	  if err != nil {
		  return 0, fmt.Errorf("list expired: %w", err)
	   }
	   revoked := 0
	   for _, lease := range expired {
		   rev := NewRevocation(lease.ID,
			  lease.UserID, RevokeTTLExpiry,
			  fmt.Sprintf("TTL expired at %s", lease.ExpiresAt.Format(time.RFC3339)),
			  "ttl_scan_worker")
		  if err := w.store.InsertRevocation(ctx, rev); err != nil {
			  w.log.Error().Err(err).Str("lease_id", lease.ID).Msg("insert revocation failed")
			  continue
		  }
		   if err := w.store.SetLeaseRevoked(ctx, lease.ID, "ttl_expiry"); err != nil {
			  w.log.Error().Err(err).Str("lease_id", lease.ID).Msg("revoke lease failed")
			   continue
		  }
		   revoked++
		  w.log.Info().Str("lease_id", lease.ID).Str("user_id", lease.UserID).Msg("lease TTL expired and revoked")
	   }
	   return revoked, nil
  }

// Job 返回 workerloop 兼容的 Job 函数。

  func (w *TTLScanWorker) Job(ctx context.Context) error {
	  _, err := w.Tick(ctx)
	   return err
  }

// DriftPatrolWorker 巡逻已签发租约，重算 Root 并检测漂移。

type DriftPatrolWorker struct {
	store      Store
	recomputer RootRecomputer
	log        zerolog.Logger
	batchSz int
}

  func NewDriftPatrolWorker(store Store, rc RootRecomputer, log zerolog.Logger, batchSz int) *DriftPatrolWorker {
	  if batchSz <= 0 {
		   batchSz = 50
	  }
	   return &DriftPatrolWorker{store: store, recomputer: rc, log: log, batchSz: batchSz}
   }

// Tick 执行一轮漂移巡逻。

   func (w *DriftPatrolWorker) Tick(ctx context.Context) (int, error) {
	  leases, err := w.store.ListIssuedLeases(ctx, w.batchSz)
	   if err != nil {
		  return 0, fmt.Errorf("list issued: %w", err)
	  }
	  drifts := 0
	   for _, lease := range leases {
		  if lease.IsExpired() {
			  continue
		   }
		   newRoot, newHW, newSeqs, err := w.recomputer.RecomputeRoot(ctx, lease.UserID, lease.HighWater)
		   if err != nil {
			  w.log.Error().Err(err).Str("lease_id", lease.ID).Msg("recompute root failed")
			   continue
		  }
		   if newRoot == lease.Root {
			  continue
		  }
		  dc := NewRootDriftCase(lease.ID, lease.UserID, lease.Root, newRoot,
			  lease.HighWater, newHW, newSeqs)
		  if err := w.store.InsertDrift(ctx, dc); err != nil {
			  w.log.Error().Err(err).Str("lease_id", lease.ID).Msg("insert drift failed")
			   continue
		   }
		   if err := w.store.UpdateDriftState(ctx, dc.ID, DriftOpen, DriftProven); err != nil {
			  w.log.Error().Err(err).Str("drift_id", dc.ID).Msg("prove drift failed")
			  continue
		   }
		  rev := NewRevocation(lease.ID, lease.UserID, RevokeRootDrift,
			  fmt.Sprintf("root drift patrol: %s → %s", lease.Root, newRoot),
			  "drift_patrol_worker")
		   rev.DriftID = dc.ID
		   if err := w.store.InsertRevocation(ctx, rev); err != nil {
			  w.log.Error().Err(err).Str("lease_id", lease.ID).Msg("insert revocation failed")
			  continue
		  }
		   if err := w.store.SetLeaseRevoked(ctx, lease.ID, "root_drift:"+dc.ID); err != nil {
			   w.log.Error().Err(err).Str("lease_id", lease.ID).Msg("revoke failed")
			   continue
		   }
		  drifts++
		   w.log.Warn().
			   Str("lease_id", lease.ID).
			   Str("drift_id", dc.ID).
			   Str("old_root", lease.Root).
			   Str("new_root", newRoot).
			   Msg("drift detected by patrol")
	  }
	   return drifts, nil
  }

   func (w *DriftPatrolWorker) Job(ctx context.Context) error {
	   _, err := w.Tick(ctx)
	  return err
   }

// ProofSealWorker 校验所有已签发租约的证明完整性。

type ProofSealWorker struct {
	store   Store
	log     zerolog.Logger
	batchSz int
}

   func NewProofSealWorker(store Store, log zerolog.Logger, batchSz int) *ProofSealWorker {
	   if batchSz <= 0 {
		  batchSz = 100
	  }
	   return &ProofSealWorker{store: store, log: log, batchSz: batchSz}
  }

// Tick 执行一轮证明校验。
   func (w *ProofSealWorker) Tick(ctx context.Context) (int, error) {
	  leases, err := w.store.ListIssuedLeases(ctx, w.batchSz)
	  if err != nil {
		   return 0, fmt.Errorf("list issued: %w", err)
	   }
	  sealed := 0
	   for _, lease := range leases {
		   proof, err := w.store.GetProofByLease(ctx, lease.ID)
		  if err != nil {
			  w.log.Error().Err(err).Str("lease_id", lease.ID).Msg("proof missing for issued lease")
			  rev := NewRevocation(lease.ID, lease.UserID, RevokeManual,
				   "proof missing for issued lease", "proof_seal_worker")
			   if insErr := w.store.InsertRevocation(ctx, rev); insErr != nil {
				  w.log.Error().Err(insErr).Str("lease_id", lease.ID).Msg("insert revocation failed")
			  }
			  if revErr := w.store.SetLeaseRevoked(ctx, lease.ID, "proof_missing"); revErr != nil {
				  w.log.Error().Err(revErr).Str("lease_id", lease.ID).Msg("revoke failed")
			   }
			  continue
		   }
		  if err := proof.Verify(); err != nil {
			  w.log.Error().Err(err).Str("lease_id", lease.ID).Str("proof_id", proof.ID).Msg("proof seal verification failed")
			   rev := NewRevocation(lease.ID, lease.UserID, RevokeManual,
				   "proof seal verification failed: "+err.Error(), "proof_seal_worker")
			   if insErr := w.store.InsertRevocation(ctx, rev); insErr != nil {
				  w.log.Error().Err(insErr).Str("lease_id", lease.ID).Msg("insert revocation failed")
			  }
			   if revErr := w.store.SetLeaseRevoked(ctx, lease.ID, "proof_invalid"); revErr != nil {
				  w.log.Error().Err(revErr).Str("lease_id", lease.ID).Msg("revoke failed")
			   }
			   continue
		  }
		  sealed++
	   }
	  return sealed, nil
   }

   func (w *ProofSealWorker) Job(ctx context.Context) error {
	   _, err := w.Tick(ctx)
	  return err
  }
