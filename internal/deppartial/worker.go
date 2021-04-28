package deppartial

import (
	"context"
	"time"
	"github.com/rs/zerolog"
)

// ──────────────────────────────────────────────────
// 工人：残余扫描、修复贷记/放弃、租约回收
// ──────────────────────────────────────────────────
// ResidueScanWorker 定期扫描所有非 closed 批次。
type ResidueScanWorker struct {
	svc *Service
	interval time.Duration
	inputFn func(batchID string) (ScanInput, bool)
	log     zerolog.Logger
}

  func NewResidueScanWorker(svc *Service, interval time.Duration, inputFn func(string) (ScanInput, bool)) *ResidueScanWorker {
	  return &ResidueScanWorker{
		  svc: svc,
		   interval: interval,
		   inputFn: inputFn,
		   log:     zerolog.Nop(),
	   }
  }

   func (w *ResidueScanWorker) WithLogger(l zerolog.Logger) *ResidueScanWorker {
	  if w != nil {
		  w.log = l
	  }
	   return w
  }

// Run 启动残余扫描工人。

   func (w *ResidueScanWorker) Run(ctx context.Context) {
	   if w == nil || w.svc == nil {
		   return
	  }
	  ticker := time.NewTicker(w.interval)
	   defer ticker.Stop()
	  for {
		   select {
		   case <-ctx.Done():
			  return
		   case <-ticker.C:
			   w.scan()
		  }
	  }
  }

   func (w *ResidueScanWorker) scan() {
	  batches := w.svc.AllBatchIDs()
	  for _, batchID := range batches {
		   state, err := w.svc.GetBatchState(batchID)
		  if err != nil || state == BRClosed {
			  continue
		   }
		   if w.inputFn == nil {
			  continue
		  }
		   input, ok := w.inputFn(batchID)
		   if !ok {
			  continue
		   }
		  result, err := w.svc.Scan(input)
		  if err != nil {
			   w.log.Warn().Err(err).Str("batch_id", batchID).Msg("残余扫描失败")
			   continue
		   }
		   if result.ResidueCount > 0 {
			  w.log.Info().Str("batch_id", batchID).Int("residue_count", result.ResidueCount).Msg("发现残余行")
		  }
	  }
  }

// RunOnce 单次扫描（用）。

   func (w *ResidueScanWorker) RunOnce() {
	  if w == nil {
		   return
	  }
	   w.scan()
   }

// LeaseReclaimWorker 定期回收过期租约。
type LeaseReclaimWorker struct {
	svc      *Service
	interval time.Duration
	log      zerolog.Logger
}

  func NewLeaseReclaimWorker(svc *Service, interval time.Duration) *LeaseReclaimWorker {
	   return &LeaseReclaimWorker{
		   svc: svc,
		  interval: interval,
		  log:      zerolog.Nop(),
	   }
   }

   func (w *LeaseReclaimWorker) WithLogger(l zerolog.Logger) *LeaseReclaimWorker {
	  if w != nil {
		  w.log = l
	  }
	  return w
   }

// Run 启动租约回收工人。
   func (w *LeaseReclaimWorker) Run(ctx context.Context) {
	   if w == nil || w.svc == nil {
		   return
	  }
	   ticker := time.NewTicker(w.interval)
	  defer ticker.Stop()
	  for {
		  select {
		   case <-ctx.Done():
			  return
		  case <-ticker.C:
			   w.reclaim()
		  }
	   }
   }

   func (w *LeaseReclaimWorker) reclaim() {
	   expired := w.svc.ExpireLeases()
	   for _, l := range expired {
		  w.log.Info().Str("lease_id", l.ID).Str("batch_id", l.BatchID).Msg("过期租约已回收")
	   }
  }

// RunOnce 单次回收（用）。

   func (w *LeaseReclaimWorker) RunOnce() {
	  if w == nil {
		  return
	   }
	  w.reclaim()
   }

// RepairWorker 自动修复/放弃工人。
type RepairWorker struct {
	svc *Service
	interval time.Duration
	decisionFn func(residue ObserveResidue) (action string, ledgerID string, amount string, reason string)
	log        zerolog.Logger
}

   func NewRepairWorker(svc *Service, interval time.Duration, decisionFn func(ObserveResidue) (string, string, string, string)) *RepairWorker {
	  return &RepairWorker{
		  svc:      svc,
		   interval: interval,
		   decisionFn: decisionFn,
		  log:        zerolog.Nop(),
	  }
  }

  func (w *RepairWorker) WithLogger(l zerolog.Logger) *RepairWorker {
	  if w != nil {
		   w.log = l
	   }
	  return w
  }

// Run 启动修复工人。
   func (w *RepairWorker) Run(ctx context.Context) {
	   if w == nil || w.svc == nil {
		  return
	  }
	  ticker := time.NewTicker(w.interval)
	  defer ticker.Stop()
	  for {
		  select {
		  case <-ctx.Done():
			   return
		  case <-ticker.C:
			  w.repair()
		  }
	   }
  }

  func (w *RepairWorker) repair() {
	  batches := w.svc.BatchesInState(BRReopening)
	  for _, batchID := range batches {
		  residues, err := w.svc.ListResidues(batchID)
		  if err != nil {
			  continue
		   }
		   for _, r := range residues {
			  if r.State != ResOpen && r.State != ResEscalated {
				  continue
			   }
			  if w.decisionFn == nil {
				   continue
			  }
			  action, ledgerID, amount, reason := w.decisionFn(r)
			  switch action {
			   case "credit":
				   lease, err := w.svc.GetActiveLease(batchID)
				  if err != nil {
					  w.log.Warn().Err(err).Str("batch_id", batchID).Msg("修复：无活跃租约")
					   continue
				  }
				   _, err = w.svc.RepairCredit(RepairCreditReq{
					   BatchID:   batchID,
					  ResidueID: r.ID,
					  Actor:    "repair_worker",
					  LedgerID: ledgerID,
					  Amount:   amount,
					   Generation: lease.Generation,
				  })
				   if err != nil {
					   w.log.Warn().Err(err).Str("residue_id", r.ID).Msg("修复贷记失败")
				  }
			  case "abandon":
				  _, err := w.svc.Abandon(AbandonReq{
					  BatchID:   batchID,
					   ResidueID: r.ID,
					  Actor:  "repair_worker",
					   Reason: reason,
				   })
				   if err != nil {
					  w.log.Warn().Err(err).Str("residue_id", r.ID).Msg("放弃残余失败")
				   }
			   case "escalate":
				   _, err := w.svc.Escalate(EscalateReq{
					  BatchID: batchID,
					  ResidueID: r.ID,
					   Actor:  "repair_worker",
					   Reason: reason,
				   })
				  if err != nil {
					  w.log.Warn().Err(err).Str("residue_id", r.ID).Msg("升级残余失败")
				  }
			  }
		   }
	   }
   }

// RunOnce 单次修复（用）。

   func (w *RepairWorker) RunOnce() {
	  if w == nil {
		  return
	  }
	  w.repair()
   }

// AlignCheckWorker 定期检查一致证明。
type AlignCheckWorker struct {
	svc      *Service
	interval time.Duration
	log zerolog.Logger
}

  func NewAlignCheckWorker(svc *Service, interval time.Duration) *AlignCheckWorker {
	  return &AlignCheckWorker{
		  svc:      svc,
		   interval: interval,
		  log: zerolog.Nop(),
	   }
   }

  func (w *AlignCheckWorker) WithLogger(l zerolog.Logger) *AlignCheckWorker {
	  if w != nil {
		  w.log = l
	   }
	   return w
  }

// Run 启动一致检查工人。

  func (w *AlignCheckWorker) Run(ctx context.Context) {
	  if w == nil || w.svc == nil {
		  return
	  }
	  ticker := time.NewTicker(w.interval)
	   defer ticker.Stop()
	   for {
		   select {
		   case <-ctx.Done():
			  return
		  case <-ticker.C:
			   w.check()
		  }
	  }
   }

   func (w *AlignCheckWorker) check() {
	   for _, state := range []BatchResidueState{BRReopening, BRResidueOpen} {
		  batches := w.svc.BatchesInState(state)
		   for _, batchID := range batches {
			   proofs, err := w.svc.CheckAlignBatch(batchID, "align_worker")
			   if err != nil {
				   w.log.Warn().Err(err).Str("batch_id", batchID).Msg("对齐检查失败")
				   continue
			  }
			  passed := 0
			   for _, p := range proofs {
				   if p.Status == ProofPassed {
					   passed++
				   }
			   }
			  w.log.Info().Str("batch_id", batchID).Int("total", len(proofs)).Int("passed", passed).Msg("对齐检查完成")
		  }
	   }
  }

// RunOnce 单次检查（用）。

   func (w *AlignCheckWorker) RunOnce() {
	   if w == nil {
		  return
	  }
	   w.check()
  }
