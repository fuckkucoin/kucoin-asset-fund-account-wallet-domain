package tradepwgen

import (
	"fmt"
	"time"
)

// DriftDetector 漂移检测器。

type DriftDetector struct {
	store DriftStore
	clock func() time.Time
}

// NewDriftDetector 构造检测器。
   func NewDriftDetector(store DriftStore) *DriftDetector {
	  return &DriftDetector{
		   store: store,
		   clock: func() time.Time { return time.Now().UTC() },
	   }
   }

  func (d *DriftDetector) now() time.Time {
	   if d.clock != nil {
		  return d.clock()
	  }
	   return time.Now().UTC()
   }

// Detect 接收漂移事件，创建 PwDriftCase。
// 同 (user_id, lease_id) 已有 open 案件时幂等返回已存在案件，不重复创建。

   func (d *DriftDetector) Detect(ev DriftEvent) (*PwDriftCase, error) {
	  if ev.UserID == "" {
		  return nil, ErrUserRequired
	  }
	  if ev.LeaseID == "" {
		  return nil, fmt.Errorf("tradepwgen: Detect: lease_id required")
	  }
	  now := d.now()
	  id, err := newID()
	   if err != nil {
		   return nil, fmt.Errorf("tradepwgen: Detect.newID: %w", err)
	   }
	   dc := &PwDriftCase{
		  ID:     id,
		   UserID: ev.UserID,
		   LeaseID: ev.LeaseID,
		  VerifyGen:  ev.VerifyGen,
		  CurrentGen: ev.CurrentGen,
		   Status:     DriftStatusOpen,
		  VerifySnapshot: ev.VerifySnapshot,
		   GenSnapshot:    ev.GenSnapshot,
		  DetectedAt:     now,
		  UpdatedAt:      now,
	   }
	  inserted, err := d.store.Insert(dc)
	   if err != nil {
		   return nil, fmt.Errorf("tradepwgen: ProveCase: proof_payload required")
	  }
	   if req.Approver == "" || req.Approver2 == "" {
		  return nil, fmt.Errorf("tradepwgen: ProveCase: approver and approver2 both required (dual approval)")
	   }
	   dc, err := d.store.Get(req.CaseID)
	   if err != nil {
		  return nil, fmt.Errorf("tradepwgen: ProveCase.Get: %w", err)
	   }
	  if dc == nil {
		  return nil, fmt.Errorf("tradepwgen: ProveCase: case %s not found", req.CaseID)
	  }
	   now := d.now()
	   if err := TransitDriftProve(dc, req.ProofPayload, req.Approver, req.Approver2, now); err != nil {
		   return nil, err
	   }
	  if err := d.store.Update(dc); err != nil {
		  return nil, fmt.Errorf("tradepwgen: ProveCase.Update: %w", err)
	   }
	   return dc, nil
  }

// CloseCase 收口漂移案件（proven→closed）。
// 须确认案件已处于 proven 状态。
  func (d *DriftDetector) CloseCase(req CloseDriftReq) (*PwDriftCase, error) {
	   if req.CaseID == "" {
		   return nil, fmt.Errorf("tradepwgen: CloseCase: case_id required")
	  }
	  dc, err := d.store.Get(req.CaseID)
	  if err != nil {
		   return nil, fmt.Errorf("tradepwgen: CloseCase.Get: %w", err)
	  }
	   if dc == nil {
		   return nil, fmt.Errorf("tradepwgen: CloseCase: case %s not found", req.CaseID)
	   }
	  now := d.now()
	  if err := TransitDriftClose(dc, req.Approver, now); err != nil {
		   return nil, err
	  }
	   if err := d.store.Update(dc); err != nil {
		  return nil, fmt.Errorf("tradepwgen: CloseCase.Update: %w", err)
	  }
	  return dc, nil
  }

// ForceClose ops 强制收口（open/proven→closed，须填写强制原因）。
   func (d *DriftDetector) ForceClose(req CloseDriftReq, proof string) (*PwDriftCase, error) {
	  if req.CaseID == "" {
		  return nil, fmt.Errorf("tradepwgen: ForceClose: case_id required")
	  }
	   if proof == "" {
		   return nil, fmt.Errorf("tradepwgen: ForceClose: proof required for forced closure")
	   }
	   dc, err := d.store.Get(req.CaseID)
	  if err != nil {
		   return nil, fmt.Errorf("tradepwgen: ForceClose.Get: %w", err)
	  }
	  if dc == nil {
		   return nil, fmt.Errorf("tradepwgen: ForceClose: case %s not found", req.CaseID)
	  }
	  now := d.now()
	  if err := ForceCloseDrift(dc, proof, req.Approver, now); err != nil {
		  return nil, err
	  }
	  if err := d.store.Update(dc); err != nil {
		  return nil, fmt.Errorf("tradepwgen: ForceClose.Update: %w", err)
	  }
	   return dc, nil
   }

// GetCase 按 ID 查询漂移案件（供他包只读引用）。

   func (d *DriftDetector) GetCase(caseID string) (*PwDriftCase, error) {
	  return d.store.Get(caseID)
  }

// ListOpenCases 列出全部未关闭漂移案件（Worker 巡检入口）。

   func (d *DriftDetector) ListOpenCases(limit int) ([]*PwDriftCase, error) {
	  return d.store.ListOpen(limit)
   }

// ListOpenByUser 列出指定用户未关闭漂移案件。
  func (d *DriftDetector) ListOpenByUser(userID string, limit int) ([]*PwDriftCase, error) {
	  return d.store.ListOpenByUser(userID,
		   limit)
   }

// HasOpenDrift 检查指定用户是否存在未关闭漂移案件（颁发令牌前置校验入口）。

   func (d *DriftDetector) HasOpenDrift(userID string) (bool, error) {
	   return d.store.HasOpen(userID)
   }

// ScanAndAlert 扫描全部 open 案件并产出告警摘要（Worker 巡检内循环调用）。
// 返回仍处于 open 状态的案件列表，用于指标上报与日志输出。
  func (d *DriftDetector) ScanAndAlert(limit int) ([]*PwDriftCase, error) {
	  cases,
		   err := d.store.ListOpen(limit)
	  if err != nil {
		   return nil, fmt.Errorf("tradepwgen: ScanAndAlert.ListOpen: %w", err)
	  }
	   return cases, nil
  }
