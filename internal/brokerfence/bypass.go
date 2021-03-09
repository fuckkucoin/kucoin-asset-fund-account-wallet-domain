package brokerfence

import (
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
)

// BypassStore 旁路案件持久化接口，由 persist.go 实现。

type BypassStore interface {
	SaveBypassCase(bc *BypassCase) error
	GetBypassCase(id string) (*BypassCase, error)
	ListBypassCases(subID string,
		state BypassState) ([]*BypassCase, error)
	UpdateBypassCase(bc *BypassCase) error
}

// BypassDetector 围栏旁路检测器。
type BypassDetector struct {
	mu    sync.Mutex
	store BypassStore
	clock func() time.Time
}

// NewBypassDetector 构造旁路检测器。
   func NewBypassDetector(store BypassStore) *BypassDetector {
	  return &BypassDetector{
		  store: store,
		  clock: func() time.Time { return time.Now().UTC() },
	   }
  }

   func (d *BypassDetector) now() time.Time {
	  if d.clock != nil {
		   return d.clock()
	  }
	   return time.Now().UTC()
  }

// Detect 检测到一次旁路写，创建 BypassCase。
// leaseID：当前围栏租约 ID；kind：写方向类型；amount：绕过写金额。
// 返回新建的 BypassCase，调用方凭此 ID 追踪处置进度。
  func (d *BypassDetector) Detect(leaseID, subID, masterID, asset, amount string, kind BypassKind) (*BypassCase, error) {
	   leaseID = strings.TrimSpace(leaseID)
	   subID = strings.TrimSpace(subID)
	   masterID = strings.TrimSpace(masterID)
	   asset = strings.TrimSpace(asset)
	  amount = strings.TrimSpace(amount)
	   if leaseID == "" || subID == "" || asset == "" {
		   return nil, ErrInput
	   }
	  now := d.now()
	  bc := &BypassCase{
		  ID:       uuid.NewString(),
		   LeaseID:  leaseID,
		   SubID:    subID,
		  MasterID: masterID,
		   Asset: asset,
		   Amount: amount,
		  Kind:  kind,
		  State: BypassStateOpen,
		  HangingAmt: amount, // 检测到即挂账
		   DetectedAt: now,
		   UpdatedAt:  now,
	   }
	   d.mu.Lock()
	  err := d.store.SaveBypassCase(bc)
	  d.mu.Unlock()
	  if err != nil {
		   return nil, err
	  }
	   return bc, nil
   }

// Approve 对旁路案件执行双人审核的某一步。
// step=1：第一审；step=2：第二审（不得与第一审相同人）。
   func (d *BypassDetector) Approve(req BypassConfirmReq) (*BypassCase, error) {
	  if strings.TrimSpace(req.CaseID) == "" || strings.TrimSpace(req.Approver) == "" {
		  return nil, ErrInput
	  }
	  if req.Step != 1 && req.Step != 2 {
		   return nil, ErrInput
	   }
	  d.mu.Lock()
	  defer d.mu.Unlock()
	  bc, err := d.store.GetBypassCase(req.CaseID)
	  if err != nil {
		   return nil, err
	  }
	  if bc.State == BypassStateClosed {
		  return bc, nil // 幂等：已关闭
	  }
	   now := d.now()
	   approver := strings.TrimSpace(req.Approver)
	  switch req.Step {
	   case 1:
		  if bc.ApproverOne != "" {
			   return nil, ErrDualApprovalRequired // 第一审已完成，重复提交
		  }
		   bc.ApproverOne = approver
		   bc.UpdatedAt = now
		  if err := TransitBypass(bc, BypassStateProven, now); err != nil {
			   return nil, err
		   }
	   case 2:
		   if bc.State != BypassStateProven {
			   return nil, ErrDualApprovalRequired
		   }
		  if bc.ApproverOne == approver {
			  return nil, ErrSameReviewer
		  }
		  bc.ApproverTwo = approver
		  if err := TransitBypass(bc, BypassStateClosed, now); err != nil {
			  return nil, err
		  }
		  bc.HangingAmt = "0" // 双人确认后解挂
	  }
	   if err := d.store.UpdateBypassCase(bc); err != nil {
		   return nil, err
	   }
	   return bc, nil
  }

// Get 按 ID 查询旁路案件。
   func (d *BypassDetector) Get(caseID string) (*BypassCase, error) {
	  if strings.TrimSpace(caseID) == "" {
		  return nil, ErrInput
	   }
	   return d.store.GetBypassCase(caseID)
   }

// ListOpen 列出子账尚未关闭的旁路案件。

  func (d *BypassDetector) ListOpen(subID string) ([]*BypassCase, error) {
	   return d.store.ListBypassCases(strings.TrimSpace(subID), BypassStateOpen)
  }

// HasOpenCases 判断子账是否存在未关闭旁路案件（围栏世代提升前必须清零）。
   func (d *BypassDetector) HasOpenCases(subID string) (bool, error) {
	   cases, err := d.store.ListBypassCases(strings.TrimSpace(subID), BypassStateOpen)
	   if err != nil {
		  return false, err
	   }
	  proven, err := d.store.ListBypassCases(strings.TrimSpace(subID), BypassStateProven)
	   if err != nil {
		  return false, err
	   }
	  return len(cases)+len(proven) > 0, nil
  }
