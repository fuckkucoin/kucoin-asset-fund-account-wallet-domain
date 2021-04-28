package deppartial

import (
	"strings"
	"github.com/google/uuid"
)

// ──────────────────────────────────────────────────
// 残余扫描
// ──────────────────────────────────────────────────
// BatchItemSnapshot 外部提交的批次 item 快照，用于与实际 seen/ledger 核对。
type BatchItemSnapshot struct {
	Seq int `json:"seq"`
	Chain string `json:"chain"`
	TxHash string `json:"tx_hash"`
	Asset  string `json:"asset"`
	UserID string `json:"user_id"`
	Status string `json:"status"`
	LedgerID string `json:"ledger_id,omitempty"`
	Amount   string `json:"amount,omitempty"`
}

// SeenRecord 链上观察记录。
type SeenRecord struct {
	Chain string `json:"chain"`
	TxHash   string `json:"tx_hash"`
	Occupied bool   `json:"occupied"`
}

// LedgerRecord 账本贷记记录。
type LedgerRecord struct {
	Chain string `json:"chain"`
	TxHash   string `json:"tx_hash"`
	LedgerID string `json:"ledger_id"`
	Amount   string `json:"amount"`
	Credited bool `json:"credited"`
}

// ScanInput 残余扫描输入。
type ScanInput struct {
	BatchID string `json:"batch_id"`
	Items []BatchItemSnapshot `json:"items"`
	Seen   []SeenRecord   `json:"seen"`
	Ledger []LedgerRecord `json:"ledger"`
	Actor  string         `json:"actor"`
}

// ScanResult 残余扫描结果。

type ScanResult struct {
	BatchID string `json:"batch_id"`
	Residues []ObserveResidue `json:"residues"`
	NewFences    []SeenFence `json:"new_fences"`
	Generation   int64       `json:"generation"`
	ResidueCount int         `json:"residue_count"`
}

// Scan 执行残余扫描： items、seen 记录和 ledger 记录，找出缺口行。

   func (svc *Service) Scan(input ScanInput) (ScanResult, error) {
	   if svc == nil {
		  return ScanResult{}, ErrUnavailable
	  }
	  if err := requireActor(input.Actor); err != nil {
		   return ScanResult{}, err
	   }
	  batchID := strings.TrimSpace(input.BatchID)
	  if batchID == "" {
		  return ScanResult{},
			  ErrBatchNotFound
	   }
	  state, ok := svc.store.GetBatchState(batchID)
	  if !ok {
		  return ScanResult{}, ErrBatchNotFound
	  }
	   if state == BRClosed {
		  return ScanResult{}, ErrAlreadyClosed
	   }
	  seenMap := make(map[string]SeenRecord, len(input.Seen))
	   for _, s := range input.Seen {
		  k := fenceKey(s.Chain, s.TxHash)
		  seenMap[k] = s
	  }
	   ledgerMap := make(map[string]LedgerRecord, len(input.Ledger))
	  for _, l := range input.Ledger {
		  k := fenceKey(l.Chain, l.TxHash)
		  ledgerMap[k] = l
	  }
	  gen := svc.store.NextGen()
	   now := svc.now()
	  var residues []ObserveResidue
	   var newFences []SeenFence
	   for _, item := range input.Items {
		  k := fenceKey(item.Chain, item.TxHash)
		   if svc.store.IsFenceClosed(item.Chain, item.TxHash) {
			  continue
		  }
		  seen, hasSeen := seenMap[k]
		   ledger, hasLedger := ledgerMap[k]
		  var kind ResidueKind
		   detected := false
		   if hasSeen && seen.Occupied && (!hasLedger || !ledger.Credited) {
			  kind = KindSeenNoCredit
			   detected = true
		  } else if hasLedger && ledger.Credited && item.Status != "submitted" {
			   kind = KindCreditNoItem
			  detected = true
		   } else if hasSeen && seen.Occupied && hasLedger && ledger.Credited && item.LedgerID == "" {
			  kind = KindMismatchState
			  detected = true
		   }
		   if detected {
			  r := ObserveResidue{
				  ID:      uuid.NewString(),
				  BatchID: batchID,
				   Seq:     item.Seq,
				   Chain: strings.TrimSpace(item.Chain),
				   TxHash:     strings.TrimSpace(item.TxHash),
				   Asset:      strings.TrimSpace(item.Asset),
				  UserID:     strings.TrimSpace(item.UserID),
				   Kind:       kind,
				  State:      ResOpen,
				   Generation: gen,
				  DetectedAt: now,
			  }
			  if hasLedger {
				  r.LedgerID = ledger.LedgerID
				  r.Amount = ledger.Amount
			   }
			  residues = append(residues, r)
			   fence := SeenFence{
				  Chain:   item.Chain,
				   TxHash:  item.TxHash,
				   BatchID: batchID,
				   Generation: gen,
				  Closed:     false,
			  }
			  newFences = append(newFences, fence)
		   }
	   }
	  for i := range residues {
		  cp := residues[i]
		  svc.store.PutResidue(&cp)
	  }
	   for i := range newFences {
		  cp := newFences[i]
		   svc.store.PutFence(&cp)
	  }
	  if len(residues) > 0 {
		   svc.mu.Lock()
		   curState, _ := svc.store.GetBatchState(batchID)
		   if svc.batchFSM.Can(curState, brEvResidueFound) {
			   next, err := svc.batchFSM.Apply(curState, brEvResidueFound)
			  if err == nil {
				   svc.store.SetBatchState(batchID, next)
			  }
		  }
		  svc.mu.Unlock()
	   }
	  return ScanResult{
		   BatchID:  batchID,
		   Residues: residues,
		  NewFences: newFences,
		   Generation:   gen,
		  ResidueCount: len(residues),
	  }, nil
  }

// ScanAllBatches 批量扫描所有非 closed 批次，返回发现残余的批次 ID 列表。
  func (svc *Service) ScanAllBatches(inputs []ScanInput) ([]string, error) {
	  if svc == nil {
		  return nil, ErrUnavailable
	   }
	   var found []string
	   for _, inp := range inputs {
		  result, err := svc.Scan(inp)
		  if err != nil {
			  continue
		   }
		   if result.ResidueCount > 0 {
			  found = append(found, result.BatchID)
		   }
	  }
	   return found, nil
   }
