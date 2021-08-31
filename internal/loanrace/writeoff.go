package loanrace

import (
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/google/uuid"
)

// ApplyWriteoff 申请坏账核销。需双控审批（申请人≠批准人）。
// 同一贷款只能有一个核销工单。

  func (svc *Service) ApplyWriteoff(loanID, principal, interest, reason, applicantActor string) (WriteoffCase, error) {
	   if svc == nil {
		   return WriteoffCase{}, ErrUnavailable
	   }
	   if applicantActor == "" {
		  return WriteoffCase{}, ErrActor
	   }
	   if reason == "" {
		  return WriteoffCase{}, ErrState
	   }
	   prin, err := money.Parse(principal)
	   if err != nil {
		  return WriteoffCase{}, ErrAmount
	   }
	   intr, err := money.Parse(interest)
	  if err != nil {
		  return WriteoffCase{}, ErrAmount
	  }
	   suspenseAmt, err := prin.Add(intr)
	  if err != nil {
		   return WriteoffCase{}, ErrAmount
	   }
	   svc.mu.Lock()
	   defer svc.mu.Unlock()
	   state, ok := svc.store.GetRaceState(loanID)
	   if !ok {
		  return WriteoffCase{}, ErrLoanNotFound
	  }
	   if state == LRWrittenOff || state == LRClosed {
		   return WriteoffCase{}, ErrState
	   }
	  if existing, ok := svc.store.WriteoffForLoan(loanID); ok {
		   if existing.State != WORejected {
			   return existing.Copy(), ErrWriteoffExists
		  }
	   }
	   uid, _ := svc.store.GetUserID(loanID)
	  now := svc.now()
	  wo := &WriteoffCase{
		   ID: uuid.NewString(),
		  LoanID:    loanID,
		  UserID:    uid,
		  Principal: prin.String(),
		  Interest: intr.String(),
		  SuspenseAmt: suspenseAmt.String(),
		  State:          WOPending,
		  ApplicantActor: applicantActor,
		  Reason:         reason,
		  CreatedAt: now,
	   }
	  svc.store.PutWriteoff(wo)
	  return wo.Copy(), nil
   }

// ApproveWriteoff 批准核销（双控：批准人≠申请人）。
  func (svc *Service) ApproveWriteoff(writeoffID, approverActor string) (WriteoffCase, error) {
	  if svc == nil {
		  return WriteoffCase{}, ErrUnavailable
	  }
	  if approverActor == "" {
		  return WriteoffCase{}, ErrActor
	   }
	  svc.mu.Lock()
	   defer svc.mu.Unlock()
	   wo, ok := svc.store.GetWriteoff(writeoffID)
	   if !ok {
		   return WriteoffCase{},
			  ErrWriteoffState
	   }
	   if wo.State != WOPending {
		   return wo.Copy(),
			  ErrWriteoffState
	  }
	   if wo.ApplicantActor == approverActor {
		  return wo.Copy(), ErrDualControl
	   }
	  next, err := svc.writeoffFSM.Apply(wo.State, woEvApprove)
	  if err != nil {
		   return wo.Copy(), ErrWriteoffState
	   }
	   wo.State = next
	  wo.ApproverActor = approverActor
	   wo.ApprovedAt = svc.now()
	  svc.store.PutWriteoff(wo)
	  return wo.Copy(), nil
   }

// ExecuteWriteoff 执行已批准的核销。转入呆滞科目，贷款进入 written_off 状态。
   func (svc *Service) ExecuteWriteoff(writeoffID, actor string) (WriteoffCase, error) {
	   if svc == nil {
		   return WriteoffCase{}, ErrUnavailable
	   }
	   if actor == "" {
		  return WriteoffCase{}, ErrActor
	   }
	   svc.mu.Lock()
	   defer svc.mu.Unlock()
	  wo, ok := svc.store.GetWriteoff(writeoffID)
	   if !ok {
		  return WriteoffCase{}, ErrWriteoffState
	   }
	  if wo.State != WOApproved {
		   return wo.Copy(), ErrWriteoffState
	   }
	   next, err := svc.writeoffFSM.Apply(wo.State, woEvApply)
	   if err != nil {
		  return wo.Copy(), ErrWriteoffState
	  }
	   now := svc.now()
	  suspenseEntry := &SuspenseEntry{
		   ID: uuid.NewString(),
		  LoanID: wo.LoanID,
		   UserID: wo.UserID,
		  Amount: wo.SuspenseAmt,
		  Kind: "writeoff",
		   Ref:  wo.ID,
		  CreatedAt: now,
	  }
	   svc.store.PutSuspense(suspenseEntry)
	   wo.State = next
	   wo.AppliedAt = now
	  svc.store.PutWriteoff(wo)
	   svc.store.SetRaceState(wo.LoanID, LRWrittenOff)
	   return wo.Copy(), nil
  }

// RejectWriteoff 驳回核销申请。

  func (svc *Service) RejectWriteoff(writeoffID, actor string) (WriteoffCase, error) {
	   if svc == nil {
		  return WriteoffCase{}, ErrUnavailable
	   }
	   if actor == "" {
		  return WriteoffCase{}, ErrActor
	   }
	   svc.mu.Lock()
	   defer svc.mu.Unlock()
	  wo, ok := svc.store.GetWriteoff(writeoffID)
	   if !ok {
		   return WriteoffCase{}, ErrWriteoffState
	   }
	  if wo.State != WOPending {
		  return wo.Copy(), ErrWriteoffState
	   }
	  next, err := svc.writeoffFSM.Apply(wo.State, woEvReject)
	   if err != nil {
		   return wo.Copy(), ErrWriteoffState
	  }
	   wo.State = next
	   wo.RejectedAt = svc.now()
	  svc.store.PutWriteoff(wo)
	  return wo.Copy(), nil
  }

// GetWriteoff 获取核销工单。
  func (svc *Service) GetWriteoff(writeoffID string) (WriteoffCase, error) {
	   if svc == nil {
		  return WriteoffCase{}, ErrUnavailable
	   }
	   wo, ok := svc.store.GetWriteoff(writeoffID)
	  if !ok {
		  return WriteoffCase{}, ErrWriteoffState
	   }
	  return wo.Copy(), nil
  }

// WriteoffForLoan 获取贷款的核销工单。
   func (svc *Service) WriteoffForLoan(loanID string) (WriteoffCase, error) {
	  if svc == nil {
		   return WriteoffCase{}, ErrUnavailable
	  }
	  wo,
		  ok := svc.store.WriteoffForLoan(loanID)
	  if !ok {
		   return WriteoffCase{}, ErrWriteoffState
	   }
	   return wo.Copy(), nil
   }

// SuspenseForLoan 获取呆滞科目条目。
   func (svc *Service) SuspenseForLoan(loanID string) []SuspenseEntry {
	  if svc == nil {
		  return nil
	   }
	  return svc.store.SuspenseForLoan(loanID)
   }
