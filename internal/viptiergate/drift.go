package viptiergate

import (
	"context"
	"strings"
	"time"
	"github.com/google/uuid"
)

// DriftDetector 管理 DriftCase 的完整生命周期。

type DriftDetector struct {
	store Store
	clock func() time.Time
}

   func newDriftDetector(store Store, clock func() time.Time) *DriftDetector {
	  return &DriftDetector{store: store, clock: clock}
   }

  // Open 开立一个新的 DriftCase。
  // 幂等：若该用户+资产+Kind 已存在 open 状态的 DriftCase，
  // 直接返回已有记录（防并发重复开案）。
   func (d *DriftDetector) Open(ctx context.Context, req OpenDriftCaseReq) (*DriftCase, error) {
	   if strings.TrimSpace(req.UserID) == "" {
		   return nil, ErrUserInvalid
	  }
	  if strings.TrimSpace(req.Asset) == "" {
		  return nil, ErrAssetInvalid
	   }
	  // 幂等检查：同用户+资产+Kind 是否已有 open 案件。
	   existing, err := d.store.FindOpenDriftCase(ctx, req.UserID, req.Asset, req.Kind)
	  if err == nil && existing != nil {
		   return existing, nil
	  }
	   now := d.clock()
	  dc := &DriftCase{
		   ID: uuid.NewString(),
		  UserID:       req.UserID,
		  Asset:        strings.ToUpper(req.Asset),
		  Kind:         req.Kind,
		   ObservedTier: req.ObservedTier,
		  ExpectedTier: req.ExpectedTier,
		   ObservedBPS: req.ObservedBPS,
		   ExpectedBPS: req.ExpectedBPS,
		  Amount: req.Amount,
		  QuoteBindID: req.QuoteBindID,
		  LeaseID:     req.LeaseID,
		   Generation:  req.Generation,
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
// 验证：证明 CaseID 与 DriftCase.ID 一致，且案件处于 open 状态。
  func (d *DriftDetector) AttachProof(ctx context.Context, caseID, proofID string) (*DriftCase, error) {
	  dc, err := d.store.GetDriftCase(ctx, caseID)
	  if err != nil {
		   return nil, ErrDriftCaseNotFound
	   }
	   if dc.Status != DriftOpen {
		  return nil, ErrLeaseStale
	   }
	   now := d.clock()
	  dc.ProofID = proofID
	   dc.Status = DriftProven
	  dc.UpdatedAt = now
	   if err := d.store.SaveDriftCase(ctx, dc); err != nil {
		  return nil, err
	  }
	   return dc, nil
   }

// ConfirmClose 四眼收口：提交一次操作员确认。
// 第一次确认：FirstConfirmBy = operator, status 保持 proven。
// 第二次确认（不同操作员）：SecondConfirmBy = operator, status → closed。
// 同一操作员重复确认返回 ErrDuplicateConfirm。
// 未证明（status = open）时不允许收口，返回 ErrProofMissing。
  func (d *DriftDetector) ConfirmClose(ctx context.Context, req ConfirmDriftReq) (*DriftCase, error) {
	   if strings.TrimSpace(req.CaseID) == "" {
		   return nil, ErrDriftCaseNotFound
	  }
	   if strings.TrimSpace(req.Operator) == "" {
		  return nil, ErrUserInvalid
	   }
	   dc, err := d.store.GetDriftCase(ctx, req.CaseID)
	  if err != nil {
		  return nil, ErrDriftCaseNotFound
	   }
	  if dc.Status == DriftClosed {
		  // 已关闭，幂等返回。
		   return dc, nil
	   }
	  if dc.Status == DriftOpen {
		   // 尚无证明，不允许收口。
		   return nil, ErrProofMissing
	   }
	  now := d.clock()
	   if dc.FirstConfirmBy == "" {
		  // 第一次确认。
		   dc.FirstConfirmBy = req.Operator
		   dc.FirstConfirmAt = &now
		   dc.UpdatedAt = now
		   if req.Note != "" {
			   dc.Note = req.Note
		   }
		  if err := d.store.SaveDriftCase(ctx, dc); err != nil {
			  return nil, err
		   }
		  return dc, nil
	   }
	   // 第二次确认。
	  if dc.FirstConfirmBy == req.Operator {
		   return nil, ErrDuplicateConfirm
	   }
	  if dc.SecondConfirmBy != "" {
		   // 已有两次确认，幂等。
		  return dc, nil
	   }
	   dc.SecondConfirmBy = req.Operator
	   dc.SecondConfirmAt = &now
	  dc.Status = DriftClosed
	  dc.ClosedAt = &now
	   dc.UpdatedAt = now
	   if req.Note != "" {
		  dc.Note = req.Note
	   }
	  if err := d.store.SaveDriftCase(ctx, dc); err != nil {
		  return nil, err
	  }
	  return dc, nil
   }

// IsBlocked 返回用户是否存在阻塞性 DriftCase（open 或 proven）。
// 若存在，返回第一个找到的案件 ID，供错误信息使用。

  func (d *DriftDetector) IsBlocked(ctx context.Context, userID string) (bool, string, error) {
	   cases, err := d.store.ListOpenDriftCasesByUser(ctx, userID)
	  if err != nil {
		  return false, "", err
	   }
	   for _, dc := range cases {
		   if dc.IsBlocking() {
			   return true, dc.ID, nil
		  }
	   }
	   return false, "", nil
   }

// GetCase 按 case_id 查询。
   func (d *DriftDetector) GetCase(ctx context.Context, caseID string) (*DriftCase, error) {
	   return d.store.GetDriftCase(ctx, caseID)
  }

// ListOpenByUser 返回用户所有 open/proven 的漂移案。

  func (d *DriftDetector) ListOpenByUser(ctx context.Context, userID string) ([]*DriftCase, error) {
	   return d.store.ListOpenDriftCasesByUser(ctx, userID)
   }

// ScanFreezeForDrift 扫描在途冻额列表，与当前世代，
// 对每一个世代不符的冻额开立 DriftFreezeStale 类型的 DriftCase。
// 由 worker 定期调用。
  func (d *DriftDetector) ScanFreezeForDrift(ctx context.Context,
	  userID string,
	  freezes []*InflightFreeze,
	  currentGen int64, currentTier, expectedTier Tier,
	   currentBPS int) ([]*DriftCase, error) {
	  var opened []*DriftCase
	   for _, f := range freezes {
		   if f.Generation == currentGen {
			  continue // 世代一致，无漂移
		   }
		  dc, err := d.Open(ctx, OpenDriftCaseReq{
			   UserID: userID,
			  Asset: f.Asset,
			   Kind: DriftFreezeStale,
			   ObservedTier: f.Tier,
			   ExpectedTier: expectedTier,
			  ObservedBPS: f.BPS,
			  ExpectedBPS: currentBPS,
			   Amount:      f.Amount,
			  LeaseID:    "",
			   Generation: currentGen,
			  Note: "inflight freeze with stale tier generation detected by drift scanner; " +
				   "freeze_id=" + f.ID,
		  })
		   if err != nil {
			   continue
		   }
		  opened = append(opened, dc)
	  }
	  return opened, nil
   }

// EstimatedDrift 计算 ObservedBPS 与 ExpectedBPS 之间的手续费差额（basis points 差 × amount）。
// 仅用于报告/统计，不影响 FSM 推进。
// 返回值 driftBPS = |observed - expected|。

  func EstimatedDrift(observedBPS, expectedBPS int, amount string) int {
	  d := observedBPS - expectedBPS
	  if d < 0 {
		   d = -d
	  }
	   return d
  }

// ProofIDForCase 计算 DriftCase 的规范 proof_id 前缀，方便关联查找。

  func ProofIDForCase(caseID string) string {
	   return "proof-" + caseID
   }

// canonicalDriftCaseKey 构造幂等查找 key（内部用）。
  func canonicalDriftCaseKey(userID, asset string, kind DriftKind) string {
	  return userID + "|" + strings.ToUpper(asset) + "|" + string(kind)
  }

// NewDriftCaseID 生成 DriftCase ID。
   func NewDriftCaseID() string {
	   return "dc-" + uuid.NewString()
  }
