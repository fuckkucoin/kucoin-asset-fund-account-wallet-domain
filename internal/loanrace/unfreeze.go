package loanrace

import (
	"crypto/sha256"
	"encoding/hex"
	"github.com/google/uuid"
)

// SubmitUnfreezeProof 提交解冻证明。全额补还或人工豁免后必须出具证明方可解冻。
   func (svc *Service) SubmitUnfreezeProof(loanID, kind, evidenceRef, submittedBy, note string) (UnfreezeProof, error) {
	  if svc == nil {
		  return UnfreezeProof{},
			   ErrUnavailable
	  }
	   if submittedBy == "" {
		   return UnfreezeProof{}, ErrActor
	   }
	  if kind == "" || evidenceRef == "" {
		  return UnfreezeProof{},
			   ErrProofInvalid
	   }
	  svc.mu.Lock()
	  defer svc.mu.Unlock()
	  state,
		   ok := svc.store.GetRaceState(loanID)
	   if !ok {
		  return UnfreezeProof{}, ErrLoanNotFound
	  }
	   if state == LRWrittenOff || state == LRClosed {
		   return UnfreezeProof{}, ErrState
	  }
	  if existing, ok := svc.store.ProofForLoan(loanID); ok {
		  if existing.State == ProofValid {
			  return existing.Copy(), ErrProofExists
		   }
	   }
	  uid, _ := svc.store.GetUserID(loanID)
	   h := sha256.Sum256([]byte(evidenceRef))
	   now := svc.now()
	  proof := &UnfreezeProof{
		  ID:          uuid.NewString(),
		  LoanID:      loanID,
		   UserID:      uid,
		  Kind:        kind,
		  EvidenceRef: evidenceRef,
		  EvidenceHash: hex.EncodeToString(h[:]),
		  State:        ProofPending,
		  SubmittedBy: submittedBy,
		  SubmittedAt: now,
		  Note: note,
	  }
	   svc.store.PutProof(proof)
	  return proof.Copy(), nil
   }

// ValidateUnfreezeProof 验证解冻证明。验证通过后方可清除 Frozen 位。
  func (svc *Service) ValidateUnfreezeProof(proofID, validatedBy string) (UnfreezeProof, error) {
	  if svc == nil {
		  return UnfreezeProof{}, ErrUnavailable
	   }
	   if validatedBy == "" {
		  return UnfreezeProof{}, ErrActor
	  }
	  svc.mu.Lock()
	   defer svc.mu.Unlock()
	   p, ok := svc.store.GetProof(proofID)
	   if !ok {
		   return UnfreezeProof{},
			  ErrProofNotFound
	   }
	   if p.State != ProofPending {
		   return p.Copy(), ErrState
	  }
	  next, err := svc.proofFSM.Apply(p.State, proofEvValidate)
	  if err != nil {
		   return p.Copy(), ErrState
	  }
	   p.State = next
	  p.ValidatedBy = validatedBy
	   p.ValidatedAt = svc.now()
	   svc.store.PutProof(p)
	  return p.Copy(), nil
  }

// RejectUnfreezeProof 驳回解冻证明。
   func (svc *Service) RejectUnfreezeProof(proofID, validatedBy string) (UnfreezeProof, error) {
	  if svc == nil {
		  return UnfreezeProof{}, ErrUnavailable
	  }
	  if validatedBy == "" {
		   return UnfreezeProof{}, ErrActor
	   }
	  svc.mu.Lock()
	   defer svc.mu.Unlock()
	   p, ok := svc.store.GetProof(proofID)
	   if !ok {
		   return UnfreezeProof{},
			  ErrProofNotFound
	   }
	   if p.State != ProofPending {
		   return p.Copy(), ErrState
	  }
	  next, err := svc.proofFSM.Apply(p.State, proofEvReject)
	  if err != nil {
		   return p.Copy(), ErrState
	   }
	  p.State = next
	   p.ValidatedBy = validatedBy
	   p.ValidatedAt = svc.now()
	  svc.store.PutProof(p)
	   return p.Copy(), nil
  }

// ClearFrozenWithProof 出具有效证明后清除 Frozen 位并将贷款推向 closed 状态。

   func (svc *Service) ClearFrozenWithProof(loanID, actor string) error {
	  if svc == nil {
		   return ErrUnavailable
	  }
	  if actor == "" {
		  return ErrActor
	  }
	   svc.mu.Lock()
	   defer svc.mu.Unlock()
	   if !svc.store.IsFrozen(loanID) {
		  return ErrNotFrozen
	  }
	   proof, ok := svc.store.ProofForLoan(loanID)
	   if !ok {
		   return ErrProofRequired
	   }
	  if proof.State != ProofValid {
		   return ErrProofRequired
	   }
	   svc.store.SetFrozen(loanID, false)
	  state, _ := svc.store.GetRaceState(loanID)
	  if state == LRFrozen || state == LRRepaying {
		   svc.store.SetRaceState(loanID, LRClosed)
	   }
	   if lease, ok := svc.store.ActiveLeaseForLoan(loanID); ok {
		  lease.State = LeaseReleased
		  lease.ReleasedAt = svc.now()
		   svc.store.PutLease(lease)
		  svc.store.ClearLeaseIndex(loanID)
	   }
	  return nil
   }

// GetProof 获取解冻证明。

  func (svc *Service) GetProof(proofID string) (UnfreezeProof, error) {
	  if svc == nil {
		   return UnfreezeProof{}, ErrUnavailable
	   }
	  p, ok := svc.store.GetProof(proofID)
	   if !ok {
		  return UnfreezeProof{}, ErrProofNotFound
	  }
	   return p.Copy(), nil
   }

// ProofForLoan 获取贷款的解冻证明。

  func (svc *Service) ProofForLoan(loanID string) (UnfreezeProof, error) {
	  if svc == nil {
		   return UnfreezeProof{}, ErrUnavailable
	   }
	  p, ok := svc.store.ProofForLoan(loanID)
	   if !ok {
		   return UnfreezeProof{}, ErrProofNotFound
	  }
	  return p.Copy(), nil
   }
