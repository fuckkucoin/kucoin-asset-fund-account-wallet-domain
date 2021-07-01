package fiatbalgate

import (
	"context"
	"fmt"
)

// ClearanceService 四眼清障服务（对 HTTP 层暴露）。

type ClearanceService struct {
	engine *Engine
	store  Store
}

// NewClearanceService 构造清障服务。

   func NewClearanceService(engine *Engine, store Store) *ClearanceService {
	  return &ClearanceService{engine: engine, store: store}
   }

// ClearStatus 清障进度。
type ClearStatus struct {
	// CaseID 案件 ID。
	CaseID string `json:"case_id"`
	// CaseState 案件当前状态。
	CaseState CaseState `json:"case_state"`
	// Approvals 已提交的确认记录（按时间升序）。
	Approvals []*ClearApproval `json:"approvals"`
	// StepRequired 下一步操作说明。
	StepRequired string `json:"step_required"`
}

// GetStatus 返回指定案件的清障进度。

  func (cs *ClearanceService) GetStatus(ctx context.Context, caseID string) (*ClearStatus, error) {
	  if cs == nil {
		   return nil, ErrUnavailable
	   }
	   c, err := cs.store.GetCase(ctx, caseID)
	  if err != nil {
		  return nil, err
	  }
	  approvals,
		   err := cs.store.ListApprovalsByCase(ctx, caseID)
	   if err != nil {
		  return nil, err
	  }
	  var stepRequired string
	   switch {
	  case c.State == CaseCleared:
		  stepRequired = "already cleared"
	  case len(approvals) == 0:
		   stepRequired = "submit first approval (step1)"
	   case len(approvals) == 1:
		   stepRequired = fmt.Sprintf("submit second approval by a different actor (not %s)", approvals[0].Actor)
	   default:
		  stepRequired = "completed"
	   }
	   return &ClearStatus{
		   CaseID: caseID,
		   CaseState: c.State,
		  Approvals: approvals,
		  StepRequired: stepRequired,
	   }, nil
   }

// SubmitStep1 第一眼确认。

  func (cs *ClearanceService) SubmitStep1(ctx context.Context, req ClearReq) (*ClearStatus, error) {
	  if cs == nil {
		   return nil, ErrUnavailable
	  }
	   _, err := cs.engine.ApproveFirst(ctx, req)
	  if err != nil {
		   return nil, err
	  }
	  return cs.GetStatus(ctx,
		   req.CaseID)
   }

// SubmitStep2 第二眼确认；成功后返回清障后的状态（含 ProofID）。
  func (cs *ClearanceService) SubmitStep2(ctx context.Context, req ClearReq) (*ClearStatus, *FiatImbalanceProof, error) {
	  if cs == nil {
		  return nil, nil, ErrUnavailable
	  }
	  proof, err := cs.engine.ApproveSecond(ctx, req)
	  if err != nil {
		  return nil, nil, err
	   }
	   status, err := cs.GetStatus(ctx, req.CaseID)
	   if err != nil {
		   return nil, proof, err
	  }
	  return status, proof, nil
  }
