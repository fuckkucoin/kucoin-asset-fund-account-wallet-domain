package utamodegate

import (
	"context"
	"strings"
	"time"
	"github.com/google/uuid"
)

// DriftDetector 管理 ModeDriftCase 的完整生命周期。
type DriftDetector struct {
	store Store
	clock func() time.Time
}

  func newDriftDetector(store Store, clock func() time.Time) *DriftDetector {
	  return &DriftDetector{store: store, clock: clock}
   }

  // Open 开立一个新的 ModeDriftCase。
   // 幂等：若该用户+资产+Kind 已存在 open 状态的漂移案，直接返回已有记录（防并发重复开案）。

   func (d *DriftDetector) Open(ctx context.Context, req OpenDriftCaseReq) (*ModeDriftCase, error) {
	  userID := strings.TrimSpace(req.UserID)
	  if userID == "" {
		   return nil, ErrUserInvalid
	  }
	   asset := strings.ToUpper(strings.TrimSpace(req.Asset))
	  if asset == "" {
		   return nil, ErrAssetInvalid
	  }
	   // 幂等：同用户+资产+Kind 是否已有 open 案件。
	  existing,
		  err := d.store.FindOpenDriftCase(ctx, userID, asset, req.Kind)
	  if err == nil && existing != nil {
		   return existing, nil
	  }
	   now := d.clock()
	  dc := &ModeDriftCase{
		   ID:      uuid.NewString(),
		   UserID:  userID,
		  Asset:   asset,
		   Kind:    req.Kind,
		  OldMode: req.OldMode,
		  NewMode: req.NewMode,
		  OldGeneration: req.OldGeneration,
		   NewGeneration: req.NewGeneration,
		  LiabilityAmount: req.LiabilityAmount,
		  BindID:          req.BindID,
		   LeaseID:         req.LeaseID,
		  Status:     DriftOpen,
		  Note:       req.Note,
		  DetectedAt: now,
		  UpdatedAt: now,
	  }
	  if err := d.store.SaveDriftCase(ctx, dc); err != nil {
		  return nil, err
	  }
	   return dc, nil
   }

// AttachProof 为漂移案附加证明，推进状态至 proven。
// 约束：
// - DriftCase 必须处于 open 状态。
// - proofID 对应的 ModeDriftProof 必须存在且 DriftCaseID 一致。
  func (d *DriftDetector) AttachProof(ctx context.Context, caseID, proofID string) (*ModeDriftCase, error) {
	   dc, err := d.store.GetDriftCase(ctx, caseID)
	   if err != nil {
		   return nil, err
	   }
	   if dc.Status != DriftOpen {
		  return nil, ErrLeaseStale // 案件不处于 open，拒绝附证明
	  }
	  // 验证 proof 存在且 CaseID 一致。
	   proof, err := d.store.GetProof(ctx, proofID)
	  if err != nil {
		   return nil, ErrProofNotFound
	   }
	   if proof.DriftCaseID != caseID {
		  return nil, ErrProofDigestMismatch
	   }
	   dc.ProofID = proofID
	   dc.Status = DriftProven
	   dc.UpdatedAt = d.clock()
	   if err := d.store.SaveDriftCase(ctx, dc); err != nil {
		  return nil, err
	  }
	  return dc, nil
   }

// ConfirmClose 四眼收口确认。
// 第一次调用记录 Confirm1By；第二次调用由不同操作员记录 Confirm2By，推进至 closed。
// 同一操作员重复调用返回 ErrDuplicateConfirm。
// DriftCase 必须处于 proven 状态且 ProofID 非空，否则返回 ErrProofMissing。
  func (d *DriftDetector) ConfirmClose(ctx context.Context, req ConfirmCloseReq) (*ModeDriftCase, error) {
	   operator := strings.TrimSpace(req.Operator)
	   if operator == "" {
		  return nil, ErrOperatorInvalid
	  }
	  dc, err := d.store.GetDriftCase(ctx, req.DriftCaseID)
	   if err != nil {
		   return nil, err
	   }
	  if dc.Status == DriftClosed {
		  return nil, ErrDriftAlreadyClosed
	   }
	  if dc.Status != DriftProven {
		   return nil, ErrProofMissing
	  }
	   if strings.TrimSpace(dc.ProofID) == "" {
		  return nil, ErrProofMissing
	   }
	   now := d.clock()
	  // 第一确认
	  if dc.Confirm1By == "" {
		   dc.Confirm1By = operator
		   dc.Confirm1At = &now
		   dc.UpdatedAt = now
		  if err := d.store.SaveDriftCase(ctx, dc); err != nil {
			  return nil, err
		   }
		   return dc, nil
	  }
	  // 防止同一操作员重复确认
	   if dc.Confirm1By == operator {
		  return nil, ErrDuplicateConfirm
	  }
	   // 第二确认，推进至 closed
	  dc.Confirm2By = operator
	   dc.Confirm2At = &now
	   dc.Status = DriftClosed
	  dc.UpdatedAt = now
	  if err := d.store.SaveDriftCase(ctx, dc); err != nil {
		   return nil, err
	  }
	   return dc, nil
  }

// IsBlocked 查询用户是否有阻塞性漂移案（open 或 proven）。
// 用于 Service.SetMode 和高风险借出的前置检查。
// 返回 (blocked, firstCaseID, error)。

   func (d *DriftDetector) IsBlocked(ctx context.Context, userID string) (bool, string, error) {
	  cases,
		  err := d.store.ListBlockingDriftCases(ctx, userID)
	  if err != nil {
		   return false, "", err
	   }
	   if len(cases) > 0 {
		  return true, cases[0].ID, nil
	   }
	  return false, "", nil
  }

// GetCase 查询单条漂移案。
   func (d *DriftDetector) GetCase(ctx context.Context, caseID string) (*ModeDriftCase, error) {
	   return d.store.GetDriftCase(ctx, caseID)
  }

// ListBlockingCases 返回用户所有阻塞性漂移案。

   func (d *DriftDetector) ListBlockingCases(ctx context.Context, userID string) ([]*ModeDriftCase, error) {
	   return d.store.ListBlockingDriftCases(ctx, userID)
   }

// ScanOpenCases 批量扫描 open 状态漂移案（供 worker 调用）。

  func (d *DriftDetector) ScanOpenCases(ctx context.Context, limit int) ([]*ModeDriftCase, error) {
	  return d.store.ListOpenDriftCases(ctx, limit)
  }

// EscalationThreshold 超过此时长仍未收口的 open/proven 案件触发告警升级。

const EscalationThreshold = 10 * time.Minute
