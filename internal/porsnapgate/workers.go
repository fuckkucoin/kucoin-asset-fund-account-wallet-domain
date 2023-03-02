package porsnapgate

import (
	"context"
	"errors"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// ExpireArchiveWorker
// ─────────────────────────────────────────────────────────────────────────────
// ExpireArchiveWorker 轮询超 TTL 的 sealed 世代并迁移为 expired。

type ExpireArchiveWorker struct {
	svc      *Service
	interval time.Duration
	batchSz int
}

// NewExpireArchiveWorker 构造 ExpireArchiveWorker。

  func NewExpireArchiveWorker(svc *Service, interval time.Duration, batchSz int) *ExpireArchiveWorker {
	  if interval <= 0 {
		   interval = 5 * time.Minute
	   }
	   if batchSz <= 0 {
		  batchSz = 50
	   }
	   return &ExpireArchiveWorker{svc: svc, interval: interval, batchSz: batchSz}
  }

// Run 启动 Worker；ctx 取消时退出。
  func (w *ExpireArchiveWorker) Run(ctx context.Context) error {
	  ticker := time.NewTicker(w.interval)
	  defer ticker.Stop()
	  for {
		  select {
		  case <-ctx.Done():
			  return ctx.Err()
		  case <-ticker.C:
			  _ = w.tick(ctx)
		  }
	  }
  }

   func (w *ExpireArchiveWorker) tick(ctx context.Context) error {
	   return w.TickForTest(ctx, w.svc.clock().UTC())
  }

// TickForTest 供直接触发一次归档扫描，注入指定时间。
  func (w *ExpireArchiveWorker) TickForTest(ctx context.Context, now time.Time) error {
	  gens, err := w.svc.store.ListExpiredGens(ctx, now, w.batchSz)
	   if err != nil {
		   return err
	  }
	   for _, gen := range gens {
		   if err := w.svc.store.UpdateGenState(ctx, gen.SnapGenID, SnapSealed, SnapExpired, now); err != nil {
			   if errors.Is(err, ErrInvalidTransition) {
				   continue
			  }
		   }
	   }
	   return nil
  }

// ─────────────────────────────────────────────────────────────────────────────
// LiveBookPatrolWorker
// ─────────────────────────────────────────────────────────────────────────────
// LiveBookPatrolCfg 巡检 Worker 配置。
type LiveBookPatrolCfg struct {
	// Interval 巡检间隔。
	Interval time.Duration
	// BatchSize 每轮处理的世代数量。
	BatchSize int
	// UsersResolver 根据 SnapGenID 解析目标用户列表的回调；为 nil 时跳过巡检。
	UsersResolver func(ctx context.Context, snapGenID string) ([]string, error)
}

// LiveBookPatrolWorker 定期将 sealed/attested 世代与活账，发现漂移则落证明。

type LiveBookPatrolWorker struct {
	svc *Service
	cfg LiveBookPatrolCfg
}

// NewLiveBookPatrolWorker 构造巡检 Worker。
  func NewLiveBookPatrolWorker(svc *Service, cfg LiveBookPatrolCfg) *LiveBookPatrolWorker {
	  if cfg.Interval <= 0 {
		   cfg.Interval = 15 * time.Minute
	   }
	   if cfg.BatchSize <= 0 {
		   cfg.BatchSize = 20
	  }
	  return &LiveBookPatrolWorker{svc: svc, cfg: cfg}
  }

// Run 启动巡检 Worker；ctx 取消时退出。
   func (w *LiveBookPatrolWorker) Run(ctx context.Context) error {
	  ticker := time.NewTicker(w.cfg.Interval)
	  defer ticker.Stop()
	  for {
		   select {
		   case <-ctx.Done():
			   return ctx.Err()
		   case <-ticker.C:
			   _ = w.tick(ctx)
		  }
	   }
   }

   func (w *LiveBookPatrolWorker) tick(ctx context.Context) error {
	   if w.cfg.UsersResolver == nil {
		   return nil
	   }
	   // 巡检 sealed 和 attested 世代
	  for _, state := range []SnapGenState{SnapSealed, SnapAttested} {
		   gens, err := w.svc.store.ListGensByState(ctx, state, w.cfg.BatchSize)
		  if err != nil {
			  continue
		   }
		   for _, gen := range gens {
			  users, err := w.cfg.UsersResolver(ctx, gen.SnapGenID)
			  if err != nil {
				  continue
			   }
			   _, _ = w.svc.CompareWithProof(ctx, CompareWithProofOpts{
				   SnapGenID: gen.SnapGenID,
				  Users:     users,
			  })
		   }
	  }
	  return nil
  }

// ─────────────────────────────────────────────────────────────────────────────
// AttestReVerifyWorker
// ─────────────────────────────────────────────────────────────────────────────
// AttestReVerifyWorker 对 published 公证进行周期性再验证：
// 检查公证的 MerkleRoot 与 TotalsFingerprint 是否仍与 snap_gen 一致。
type AttestReVerifyWorker struct {
	svc *Service
	interval time.Duration
	batchSz  int
}

// NewAttestReVerifyWorker 构造再验证 Worker。

   func NewAttestReVerifyWorker(svc *Service, interval time.Duration, batchSz int) *AttestReVerifyWorker {
	   if interval <= 0 {
		  interval = 30 * time.Minute
	   }
	  if batchSz <= 0 {
		   batchSz = 20
	   }
	   return &AttestReVerifyWorker{svc: svc, interval: interval, batchSz: batchSz}
   }

// Run 启动再验证 Worker；ctx 取消时退出。
  func (w *AttestReVerifyWorker) Run(ctx context.Context) error {
	  ticker := time.NewTicker(w.interval)
	  defer ticker.Stop()
	  for {
		   select {
		   case <-ctx.Done():
			  return ctx.Err()
		   case <-ticker.C:
			   _ = w.tick(ctx)
		  }
	   }
   }

// AttestVerifyResult 再验证结果。
type AttestVerifyResult struct {
	AttestID  string
	SnapGenID string
	Valid     bool
	Reason string
}

  func (w *AttestReVerifyWorker) tick(ctx context.Context) error {
	   attests, err := w.svc.store.ListAttestsByState(ctx, AttestPublished, w.batchSz)
	   if err != nil {
		  return err
	   }
	   for _, a := range attests {
		  _ = w.verifyOne(ctx, a)
	  }
	  return nil
  }

   func (w *AttestReVerifyWorker) verifyOne(ctx context.Context, a *AttestationBundle) *AttestVerifyResult {
	  gen, err := w.svc.store.GetGen(ctx, a.SnapGenID)
	   if err != nil {
		  return &AttestVerifyResult{AttestID: a.AttestID, SnapGenID: a.SnapGenID, Valid: false, Reason: "gen not found"}
	  }
	   if a.MerkleRoot != gen.MerkleRoot {
		  return &AttestVerifyResult{
			  AttestID: a.AttestID,
			   SnapGenID: a.SnapGenID,
			  Valid: false,
			  Reason: "merkle_root mismatch between attestation and gen",
		   }
	   }
	  if a.TotalsFingerprint != gen.TotalsFingerprint {
		   return &AttestVerifyResult{
			   AttestID: a.AttestID,
			   SnapGenID: a.SnapGenID,
			   Valid:     false,
			   Reason:    "totals_fingerprint mismatch between attestation and gen",
		  }
	  }
	  // 检查导出内容 hash
	   reHash := HashExportPayload(a.ExportPayload)
	  if reHash != a.ExportHash {
		  return &AttestVerifyResult{
			  AttestID:  a.AttestID,
			   SnapGenID: a.SnapGenID,
			  Valid:     false,
			  Reason:    "export_hash tampered",
		  }
	  }
	  return &AttestVerifyResult{AttestID: a.AttestID, SnapGenID: a.SnapGenID, Valid: true}
   }

// VerifyAttest 对单个公证执行一致性校验，供 ops API 调用。
  func (w *AttestReVerifyWorker) VerifyAttest(ctx context.Context, attestID string) (*AttestVerifyResult, error) {
	  a, err := w.svc.store.GetAttest(ctx, attestID)
	   if err != nil {
		  return nil, err
	   }
	  result := w.verifyOne(ctx, a)
	  return result, nil
   }
