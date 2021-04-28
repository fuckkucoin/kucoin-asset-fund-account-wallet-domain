package deppartial

import (
	"strings"
	"github.com/google/uuid"
)

// ──────────────────────────────────────────────────
// 贷记一致证明
// ──────────────────────────────────────────────────
// CheckAlign 检查单条 item 的贷记一致并生成证明。
   func (svc *Service) CheckAlign(req AlignCheckReq) (CreditAlignProof, error) {
	  if svc == nil {
		   return CreditAlignProof{}, ErrUnavailable
	  }
	   if err := requireActor(req.Checker); err != nil {
		   return CreditAlignProof{}, err
	   }
	  batchID := strings.TrimSpace(req.BatchID)
	  if batchID == "" {
		  return CreditAlignProof{},
			  ErrBatchNotFound
	   }
	  if _, ok := svc.store.GetBatchState(batchID); !ok {
		   return CreditAlignProof{}, ErrBatchNotFound
	  }
	   residues := svc.store.ListResiduesByBatch(batchID)
	  var target *ObserveResidue
	  for i := range residues {
		   if residues[i].Seq == req.Seq {
			   cp := residues[i]
			  target = &cp
			  break
		   }
	  }
	  now := svc.now()
	   proof := CreditAlignProof{
		   ID:      uuid.NewString(),
		   BatchID: batchID,
		  Seq:     req.Seq,
		  Checker: strings.TrimSpace(req.Checker),
	  }
	  if target != nil {
		  proof.Chain = target.Chain
		   proof.TxHash = target.TxHash
		  proof.LedgerID = target.LedgerID
		  proof.ItemState = string(target.State)
		  if target.State == ResCredited && target.LedgerID != "" {
			   proof.LedgerState = "credited"
			  proof.Aligned = true
			   proof.Status = ProofPassed
		   } else if target.State == ResAbandoned {
			   proof.LedgerState = "abandoned"
			  proof.Aligned = true
			   proof.Status = ProofPassed
		   } else {
			  proof.LedgerState = "mismatch"
			  proof.Aligned = false
			   proof.Status = ProofRejected
			  proof.Note = "item terminal state not aligned with ledger"
		   }
	   } else {
		   proof.Aligned = true
		   proof.Status = ProofPassed
		  proof.ItemState = "no_residue"
		   proof.LedgerState = "clean"
		  proof.Note = "no residue for this item"
	  }
	  proof.CheckedAt = now
	   svc.store.PutProof(&proof)
	  return proof, nil
   }

// CheckAlignBatch 检查批次所有 item 的贷记一致。
  func (svc *Service) CheckAlignBatch(batchID, checker string) ([]CreditAlignProof, error) {
	  if svc == nil {
		   return nil, ErrUnavailable
	   }
	   if err := requireActor(checker); err != nil {
		  return nil, err
	  }
	  batchID = strings.TrimSpace(batchID)
	   if batchID == "" {
		  return nil, ErrBatchNotFound
	   }
	  if _, ok := svc.store.GetBatchState(batchID); !ok {
		  return nil, ErrBatchNotFound
	   }
	   totalItems := svc.store.GetBatchItems(batchID)
	   var proofs []CreditAlignProof
	   for seq := 1; seq <= totalItems; seq++ {
		   p, err := svc.CheckAlign(AlignCheckReq{
			   BatchID: batchID,
			  Seq:     seq,
			   Checker: checker,
		  })
		  if err != nil {
			  return proofs, err
		   }
		   proofs = append(proofs, p)
	  }
	   return proofs, nil
  }

// ExportProofs 导出批次所有一致证明。
   func (svc *Service) ExportProofs(req ProofExportReq) (ProofExportResp, error) {
	  if svc == nil {
		   return ProofExportResp{}, ErrUnavailable
	  }
	  batchID := strings.TrimSpace(req.BatchID)
	   if batchID == "" {
		  return ProofExportResp{}, ErrBatchNotFound
	   }
	  if _, ok := svc.store.GetBatchState(batchID); !ok {
		   return ProofExportResp{}, ErrBatchNotFound
	  }
	  proofs := svc.store.ListProofsByBatch(batchID)
	   summary := ProofSummary{Total: len(proofs)}
	   for _, p := range proofs {
		  switch p.Status {
		   case ProofPassed:
			  summary.Passed++
		   case ProofRejected:
			   summary.Rejected++
		   default:
			   summary.Pending++
		  }
	   }
	  return ProofExportResp{
		  BatchID: batchID,
		   Proofs:  proofs,
		   Summary: summary,
	  }, nil
   }

// ListProofs 列出批次一致证明。
   func (svc *Service) ListProofs(batchID string) ([]CreditAlignProof, error) {
	  if svc == nil {
		  return nil, ErrUnavailable
	  }
	   batchID = strings.TrimSpace(batchID)
	   if batchID == "" {
		  return nil, ErrBatchNotFound
	   }
	  if _, ok := svc.store.GetBatchState(batchID); !ok {
		  return nil, ErrBatchNotFound
	  }
	   return svc.store.ListProofsByBatch(batchID), nil
   }
