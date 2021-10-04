package prefroute

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// DriftDetector 检测路由快照与实际贷记科目的偏差。

type DriftDetector struct {
	store Store
	lm *LeaseManager
	clock func() time.Time
}

// NewDriftDetector 构造漂移检测器。
   func NewDriftDetector(store Store, lm *LeaseManager) *DriftDetector {
	   return &DriftDetector{
		   store: store,
		   lm:    lm,
		   clock: func() time.Time { return time.Now().UTC() },
	  }
  }

// WithClock 注入时钟。
  func (d *DriftDetector) WithClock(c func() time.Time) *DriftDetector {
	  if d != nil && c != nil {
		  d.clock = c
	   }
	  return d
  }

   // ConfirmCredit 贷记完成后调用：填充 CreditedBook 并检测漂移。
   // 若 CreditedBook ≠ BoundBook，创建 PrefDriftCase 并阻断对应租约。

   func (d *DriftDetector) ConfirmCredit(ctx context.Context, req ConfirmCreditReq) (RouteSnap, *PrefDriftCase, error) {
	  if d == nil || d.store == nil {
		   return RouteSnap{}, nil, fmt.Errorf("%w: drift detector unavailable", ErrInvalidInput)
	  }
	  if req.RouteSnapID == "" {
		  return RouteSnap{}, nil, fmt.Errorf("%w: routeSnapID required", ErrInvalidInput)
	   }
	   if req.CreditedBook == "" {
		  return RouteSnap{}, nil, fmt.Errorf("%w: creditedBook required", ErrInvalidInput)
	   }
	   snap,
		   err := d.store.GetRouteSnap(ctx, req.RouteSnapID)
	  if err != nil {
		   return RouteSnap{}, nil, fmt.Errorf("get route snap: %w", err)
	  }
	  if snap.CreditedBook != "" {
		   return snap, nil, ErrSnapAlreadyCredited
	  }
	  now := d.clock()
	   snap.CreditedBook = req.CreditedBook
	  snap.CreditedAt = &now
	   if req.CreditTxnID != "" {
		   snap.CreditTxnID = req.CreditTxnID
	  }
	   if snap.HasDrift() {
		  // 产生漂移案例
		  dc := PrefDriftCase{
			   ID: uuid.NewString(),
			   UserID:      snap.UserID,
			   LeaseID:     snap.LeaseID,
			  RouteSnapID: snap.ID,
			   BoundBook: snap.BoundBook,
			  CreditedBook: snap.CreditedBook,
			   Generation:   snap.Generation,
			  CreditTxnID:  snap.CreditTxnID,
			  Asset:        snap.Asset,
			   Amount:       snap.Amount,
			  State:      DriftStateOpen,
			  DetectedAt: now,
		   }
		   if err := d.store.SaveDriftCase(ctx, dc); err != nil {
			  return RouteSnap{}, nil, fmt.Errorf("save drift case: %w", err)
		   }
		  snap.DriftCaseID = dc.ID
		   // 阻断对应租约
		  if d.lm != nil && snap.LeaseID != "" {
			   reason := fmt.Sprintf("drift: bound=%s credited=%s drift_case=%s",
				  snap.BoundBook, snap.CreditedBook, dc.ID)
			   if _, err2 := d.lm.BlockLease(ctx, snap.LeaseID, reason); err2 != nil {
				  _ = err2 // 不因阻断失败而回滚漂移案例
			  }
		  }
		   if err := d.store.UpdateRouteSnap(ctx, snap); err != nil {
			  return RouteSnap{}, nil, fmt.Errorf("update route snap: %w", err)
		   }
		   return snap, &dc, nil
	   }
	   if err := d.store.UpdateRouteSnap(ctx, snap); err != nil {
		   return RouteSnap{}, nil, fmt.Errorf("update route snap: %w", err)
	  }
	  return snap, nil, nil
   }

// PatrolRouteSnaps 巡检：扫描已贷记但未核对的快照（worker 周期调用）。
   func (d *DriftDetector) PatrolRouteSnaps(ctx context.Context, userIDs []string) ([]PrefDriftCase, error) {
	  if d == nil || d.store == nil {
		  return nil, fmt.Errorf("%w: drift detector unavailable", ErrInvalidInput)
	   }
	   var cases []PrefDriftCase
	   for _, userID := range userIDs {
		   userID = strings.TrimSpace(userID)
		  if userID == "" {
			  continue
		   }
		   leases, err := d.store.ListActiveLeases(ctx, userID)
		   if err != nil {
			   continue
		   }
		  for _, l := range leases {
			   snaps, err := d.store.ListRouteSnapsByLease(ctx, l.ID)
			  if err != nil {
				  continue
			  }
			   for _, snap := range snaps {
				   if snap.CreditedBook == "" || snap.DriftCaseID != "" {
					  continue
				  }
				  if snap.HasDrift() {
					   req := ConfirmCreditReq{
						   RouteSnapID:  snap.ID,
						   CreditedBook: snap.CreditedBook,
						  CreditTxnID:  snap.CreditTxnID,
					  }
					  _, dc, err := d.ConfirmCredit(ctx, req)
					   if err == nil && dc != nil {
						   cases = append(cases, *dc)
					  }
				   }
			  }
		   }
	   }
	   return cases, nil
  }

// IsDriftBlocking 检查用户是否有阻断中的漂移案例。

   func IsDriftBlocking(ctx context.Context, store Store, userID string) (bool, string, error) {
	  has, caseID, err := store.HasOpenDriftCase(ctx, userID)
	  if err != nil {
		   return false, "", err
	  }
	   return has, caseID, nil
   }

// ─────────────────────────────────────────────
// DriftProofManager 证明管理器
// ─────────────────────────────────────────────
// DriftProofManager 管理 PrefDriftCase 证明的提交与关闭。
type DriftProofManager struct {
	store Store
	clock func() time.Time
}

// NewDriftProofManager 构造证明管理器。

   func NewDriftProofManager(store Store) *DriftProofManager {
	   return &DriftProofManager{
		   store: store,
		   clock: func() time.Time { return time.Now().UTC() },
	   }
  }

// WithClock 注入时钟。

   func (m *DriftProofManager) WithClock(c func() time.Time) *DriftProofManager {
	  if m != nil && c != nil {
		   m.clock = c
	  }
	   return m
  }

   // SubmitProof 提交漂移证明。DriftCase → proven。

   func (m *DriftProofManager) SubmitProof(ctx context.Context, req SubmitDriftProofReq) (DriftProof, error) {
	  if m == nil || m.store == nil {
		  return DriftProof{}, fmt.Errorf("%w: proof manager unavailable", ErrInvalidInput)
	   }
	   if req.DriftCaseID == "" || req.SubmittedBy == "" {
		  return DriftProof{}, fmt.Errorf("%w: driftCaseID and submittedBy required", ErrInvalidInput)
	  }
	   dc, err := m.store.GetDriftCase(ctx, req.DriftCaseID)
	  if err != nil {
		   return DriftProof{}, fmt.Errorf("get drift case: %w", err)
	  }
	  if dc.State != DriftStateOpen {
		  return DriftProof{}, fmt.Errorf("%w: drift case %s state=%s, only open cases accept proofs",
			   ErrLeaseInvalid, dc.ID, dc.State)
	  }
	  now := m.clock()
	   proof := DriftProof{
		   ID:          uuid.NewString(),
		  DriftCaseID: dc.ID,
		  SubmittedBy: req.SubmittedBy,
		   Summary:     req.Summary,
		  Evidence:    req.Evidence,
		  SubmittedAt: now,
	   }
	   if err := m.store.SaveDriftProof(ctx, proof); err != nil {
		  return DriftProof{}, fmt.Errorf("save drift proof: %w", err)
	   }
	  dc.State = DriftStateProven
	  dc.ProvenAt = &now
	  dc.ProofID = proof.ID
	   dc.ProofSummary = req.Summary
	   if err := m.store.UpdateDriftCase(ctx, dc); err != nil {
		  return DriftProof{}, fmt.Errorf("update drift case: %w", err)
	  }
	   return proof, nil
   }

// ConfirmClose 四眼确认关闭漂移案例。DriftCase → closed。
   func (m *DriftProofManager) ConfirmClose(ctx context.Context, driftCaseID, confirmedBy string) (PrefDriftCase, error) {
	  if m == nil || m.store == nil {
		  return PrefDriftCase{}, fmt.Errorf("%w: proof manager unavailable", ErrInvalidInput)
	  }
	  if driftCaseID == "" || confirmedBy == "" {
		   return PrefDriftCase{}, fmt.Errorf("%w: driftCaseID and confirmedBy required", ErrInvalidInput)
	  }
	  dc, err := m.store.GetDriftCase(ctx, driftCaseID)
	  if err != nil {
		   return PrefDriftCase{}, fmt.Errorf("get drift case: %w", err)
	  }
	   if dc.State != DriftStateProven {
		  return PrefDriftCase{}, fmt.Errorf("%w: drift case %s must be proven before close, state=%s",
			   ErrDriftUnproven, dc.ID, dc.State)
	   }
	   now := m.clock()
	  dc.State = DriftStateClosed
	  dc.ClosedAt = &now
	  dc.ClosedBy = confirmedBy
	  // 更新关联证明的 confirmed 字段
	  if dc.ProofID != "" {
		   proof, err := m.store.GetDriftProof(ctx, dc.ProofID)
		  if err == nil {
			  proof.ConfirmedBy = confirmedBy
			   proof.ConfirmedAt = &now
			  _ = m.store.SaveDriftProof(ctx,
				   proof)
		   }
	   }
	  if err := m.store.UpdateDriftCase(ctx, dc); err != nil {
		  return PrefDriftCase{}, fmt.Errorf("update drift case: %w", err)
	  }
	   return dc, nil
  }

// ExportProof 导出漂移证明（只读）。

  func (m *DriftProofManager) ExportProof(ctx context.Context, proofID string) (DriftProof, error) {
	  if m == nil || m.store == nil {
		  return DriftProof{}, fmt.Errorf("%w: proof manager unavailable", ErrInvalidInput)
	   }
	  return m.store.GetDriftProof(ctx, proofID)
   }

// GetDriftCase 查询漂移案例。
  func (d *DriftDetector) GetDriftCase(ctx context.Context, caseID string) (PrefDriftCase, error) {
	   return d.store.GetDriftCase(ctx, caseID)
   }

// ListOpenDriftCases 列出 open 漂移案例。

  func (d *DriftDetector) ListOpenDriftCases(ctx context.Context, userID string) ([]PrefDriftCase, error) {
	   return d.store.ListOpenDriftCases(ctx,
		  userID)
   }

// ─────────────────────────────────────────────
// inflight credit 检查辅助
// ─────────────────────────────────────────────
// StoreInflightChecker 基于 Store 的在途贷记检查器（实现 fundpref.InflightChecker）。
type StoreInflightChecker struct {
	store Store
}

// NewStoreInflightChecker 构造在途贷记检查器。
  func NewStoreInflightChecker(store Store) *StoreInflightChecker {
	  return &StoreInflightChecker{store: store}
  }

// HasInflightCredit 检查用户是否有 pending 在途贷记。

   func (c *StoreInflightChecker) HasInflightCredit(ctx context.Context, userID string) (bool, error) {
	   credits,
		   err := c.store.ListInflightCredits(ctx, userID, InflightStatePending)
	   if err != nil {
		   return false, err
	   }
	   return len(credits) > 0, nil
  }
