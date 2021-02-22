package agreeleasegate

import (
	"context"
	"github.com/google/uuid"
)

// RevokeOnBalCross 余额跌破水位后按资产吊销活跃租约。

   func (e *Engine) RevokeOnBalCross(ctx context.Context, asset, reason string) (int, error) {
	  asset = upperTrim(asset)
	   live, _ := e.liveView("", asset)
	   gen := &AgreeGeneration{
		   ID: uuid.NewString(), Scope: "asset", Asset: asset,
		  Generation: live.RuleVersion,
		   RuleFingerprint: live.RuleFingerprint,
		   Reason: firstReason(reason, "bal_cross"), CreatedAt: e.now(),
	  }
	  _ = e.store.InsertGeneration(ctx, gen)
	  rows, err := e.store.ListActiveByAsset(ctx, asset, e.cfg.RevokeBatchSize)
	   if err != nil {
		  return 0, err
	   }
	  n := 0
	   for _, l := range rows {
		  lv, lerr := e.liveView(l.Chain, l.Asset)
		   if lerr != nil {
			  continue
		   }
		  if drift, dt, _ := e.detectDrift(l, lv); !drift {
			  continue
		   } else {
			   proof, _ := e.openProof(ctx, l, dt, lv)
			  if e.revokeLease(ctx, l, reasonOf(dt), proof) == nil {
				   n++
			   }
		   }
	  }
	   return n, nil
  }

// RevokeOnRuleFlip 规则代次或水位参数抬升后吊销旧租约。
   func (e *Engine) RevokeOnRuleFlip(ctx context.Context, asset, reason string) (int, error) {
	   asset = upperTrim(asset)
	  live, _ := e.liveView("", asset)
	  gen := &AgreeGeneration{
		  ID: uuid.NewString(), Scope: "rule", Asset: asset,
		   Generation: live.RuleVersion, RuleFingerprint: live.RuleFingerprint,
		  Reason: firstReason(reason, "rule_flip"), CreatedAt: e.now(),
	  }
	   _ = e.store.InsertGeneration(ctx,
		   gen)
	  var rows []*AgreeLease
	   var err error
	  if asset == "" {
		  rows, err = e.store.ListActiveLeases(ctx, e.cfg.RevokeBatchSize)
	   } else {
		   rows, err = e.store.ListActiveByAsset(ctx, asset, e.cfg.RevokeBatchSize)
	   }
	   if err != nil {
		  return 0, err
	   }
	   n := 0
	  for _, l := range rows {
		  lv, lerr := e.liveView(l.Chain, l.Asset)
		  if lerr != nil {
			  continue
		  }
		   if lv.RuleVersion <= l.RuleVersion && lv.RuleFingerprint == l.RuleFingerprint {
			  continue
		  }
		  proof, _ := e.openProof(ctx, l, DriftRule, lv)
		  if e.revokeLease(ctx, l, RevokeRuleFlip, proof) == nil {
			  n++
		   }
	   }
	   return n, nil
  }

// ScanBalRuleDrift 巡检全部活跃租约，复验 bal+rule。
  func (e *Engine) ScanBalRuleDrift(ctx context.Context) (int, error) {
	  rows, err := e.store.ListActiveLeases(ctx, e.cfg.RevokeBatchSize)
	  if err != nil {
		  return 0, err
	   }
	  n := 0
	  for _, l := range rows {
		   live, lerr := e.liveView(l.Chain, l.Asset)
		   if lerr != nil {
			  continue
		  }
		   if drift, dt, _ := e.detectDrift(l, live); drift {
			  proof, _ := e.openProof(ctx, l, dt, live)
			   if e.revokeLease(ctx, l, reasonOf(dt), proof) == nil {
				  n++
			  }
		   }
	  }
	  return n, nil
   }

// ExpireDue 扫描过期。

   func (e *Engine) ExpireDue(ctx context.Context) (int, error) {
	   rows, err := e.store.ListExpiredLeases(ctx, e.now(), e.cfg.RevokeBatchSize)
	  if err != nil {
		  return 0, err
	  }
	  n := 0
	  for _, l := range rows {
		   if e.ExpireLease(ctx, l.ID) == nil {
			  n++
		   }
	   }
	   return n, nil
   }

// ScanHalfPlans 半截计划挡写巡检：planned 过久未 Exec。
  func (e *Engine) ScanHalfPlans(ctx context.Context) (int, error) {
	   cutoff := e.now().Add(-e.cfg.StalePlanAfter)
	  rows, err := e.store.ListPlannedStale(ctx, cutoff, e.cfg.RevokeBatchSize)
	   if err != nil {
		   return 0, err
	   }
	   n := 0
	  for _, l := range rows {
		  if e.blockHalfPlan(ctx, l, "stale_plan") == nil {
			  proof, _ := e.openProof(ctx, l, DriftHalfPlan, LiveView{Reason: "stale_plan"})
			   _ = e.revokeLease(ctx, l, RevokeHalfPlan, proof)
			  n++
		  }
	   }
	  return n, nil
  }

// ProveProof open→proven。
  func (e *Engine) ProveProof(ctx context.Context, proofID string) error {
	   p, err := e.store.GetProofByID(ctx, proofID)
	   if err != nil {
		  return err
	   }
	  if p.State != ProofStateOpen {
		  return nil
	  }
	  t := e.now()
	   p.State = ProofStateProven
	  p.ProvenAt = &t
	  p.UpdatedAt = t
	  return e.store.UpdateProof(ctx, p)
   }

// ProveOpen 批量固化。

  func (e *Engine) ProveOpen(ctx context.Context) (int, error) {
	   rows, err := e.store.ListProofsByState(ctx, ProofStateOpen, e.cfg.ProofBatchSize)
	   if err != nil {
		   return 0, err
	  }
	   n := 0
	   for _, p := range rows {
		  if e.ProveProof(ctx, p.ID) == nil {
			  n++
		   }
	  }
	   return n, nil
   }

// ConfirmProof / DismissProof 关闭证明。

   func (e *Engine) ConfirmProof(ctx context.Context, proofID, closedBy, notes string) error {
	   return e.closeProof(ctx, proofID, closedBy, notes, true)
  }

// DismissProof 驳回证明。

  func (e *Engine) DismissProof(ctx context.Context, proofID, closedBy, notes string) error {
	   return e.closeProof(ctx, proofID, closedBy, notes, false)
  }

   func (e *Engine) closeProof(ctx context.Context, proofID, closedBy, notes string, requireProven bool) error {
	   p, err := e.store.GetProofByID(ctx, proofID)
	  if err != nil {
		  return err
	   }
	   if p.State.Closed() {
		  return nil
	  }
	  if requireProven && p.State != ProofStateProven {
		  return ErrProofBlocked
	   }
	  t := e.now()
	   p.State = ProofStateClosed
	  p.ClosedAt = &t
	  p.ClosedBy = closedBy
	   p.Notes = notes
	   p.UpdatedAt = t
	  return e.store.UpdateProof(ctx, p)
  }

// ExportProof 导出证明。

   func (e *Engine) ExportProof(ctx context.Context, proofID, actor string) (*ProofExport, error) {
	   p, err := e.store.GetProofByID(ctx, proofID)
	  if err != nil {
		   return nil, err
	   }
	   out := &ProofExport{Proof: *p,
		   ExportedAt: e.now(), ExportedBy: actor}
	   if p.LeaseID != "" {
		  if l, lerr := e.store.GetLeaseByID(ctx, p.LeaseID); lerr == nil {
			   out.Lease = l
		   }
	   }
	   return out, nil
   }

// GetLease 查询。
  func (e *Engine) GetLease(ctx context.Context, id string) (*AgreeLease, error) {
	  return e.store.GetLeaseByID(ctx, id)
   }

// GetProof 查询证明。

  func (e *Engine) GetProof(ctx context.Context, id string) (*AgreeDriftProof, error) {
	  return e.store.GetProofByID(ctx,
		  id)
   }

// ListActive 列举活跃租约。

   func (e *Engine) ListActive(ctx context.Context, asset string, limit int) ([]*AgreeLease, error) {
	  if asset != "" {
		  return e.store.ListActiveByAsset(ctx, upperTrim(asset), limit)
	   }
	  return e.store.ListActiveLeases(ctx, limit)
   }

// ListProofs 按状态列举。

   func (e *Engine) ListProofs(ctx context.Context, state ProofState, limit int) ([]*AgreeDriftProof, error) {
	  return e.store.ListProofsByState(ctx, state, limit)
   }

// ListGenerations 世代。
   func (e *Engine) ListGenerations(ctx context.Context, limit int) ([]*AgreeGeneration, error) {
	  return e.store.ListGenerations(ctx, limit)
   }

// ListHalfPlans 半截计划。
  func (e *Engine) ListHalfPlans(ctx context.Context, limit int) ([]*HalfPlanBlock, error) {
	  return e.store.ListHalfPlans(ctx, limit)
  }

// ManualRevoke 运维吊销。
  func (e *Engine) ManualRevoke(ctx context.Context, leaseID, reason string) (*AgreeLease, error) {
	   lease, err := e.store.GetLeaseByID(ctx, leaseID)
	  if err != nil {
		   return nil, err
	   }
	   if reason == "" {
		  reason = string(RevokeManual)
	   }
	   if err := e.revokeLease(ctx, lease, RevokeReason(reason), nil); err != nil {
		  return nil, err
	  }
	   return e.store.GetLeaseByID(ctx,
		   leaseID)
  }

  func firstReason(a, b string) string {
	  if a != "" {
		  return a
	  }
	   return b
   }
