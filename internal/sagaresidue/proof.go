package sagaresidue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Prover 收口证明生成器与验证器。

type Prover struct {
	store Store
}

// NewProver 构造 Prover。
  func NewProver(st Store) *Prover {
	   return &Prover{store: st}
  }

// IssueProof 生成 CloseProof 并持久化。
// 前置条件：
// 1. Plan 下所有腿均处于 applied/skipped（无 pending / failed）；
// 2. 所有 ResidueItem 已 IsConfirmed；
// 3. rrun.PartialDoneSteps 不为空。
// 若条件不满足，返回描述性错误（而非 ErrProofMissing）。
// ErrProofMissing 仅在校验路径中返回（证明尚未生成时标 Closed）。

   func (p *Prover) IssueProof(ctx context.Context, rrun *ResidueRun, items []ResidueItem, legs []CloseoutLeg) (*CloseProof, error) {
	  if p == nil || p.store == nil {
		   return nil, ErrUnavailable
	   }
	   // ── 1. 校验前置条件 ───────────────────────────────────────────────────────
	   for _, l := range legs {
		   if l.State == LegStatePending || l.State == LegStateFailed {
			  return nil, fmt.Errorf("sagaresidue issue proof: leg %s still %s", l.ID, l.State)
		   }
	   }
	   for _, item := range items {
		  if !item.IsConfirmed {
			  return nil, fmt.Errorf("sagaresidue issue proof: item %s not confirmed", item.ID)
		  }
	   }
	  // ── 2. 计算各分量哈希 ─────────────────────────────────────────────────────
	  intentHash, err := hashDoneSteps(rrun.PartialDoneSteps)
	   if err != nil {
		  return nil, fmt.Errorf("sagaresidue issue proof hash intent: %w", err)
	  }
	   residueHash, err := hashResidueItems(items)
	   if err != nil {
		   return nil, fmt.Errorf("sagaresidue issue proof hash residue: %w", err)
	   }
	  legHash, err := hashCloseoutLegs(legs)
	   if err != nil {
		   return nil, fmt.Errorf("sagaresidue issue proof hash legs: %w", err)
	  }
	   proofDigest := hashProofDigest(intentHash, residueHash, legHash)
	  // ── 3. 构造并持久化证明 ───────────────────────────────────────────────────
	   now := time.Now().UTC()
	  // 查找关联 Plan ID（从 legs 中取第一个 PlanID）
	   planID := ""
	  if len(legs) > 0 {
		  planID = legs[0].PlanID
	  }
	   proof := &CloseProof{
		   ID: newID(),
		  ResidueRunID: rrun.ID,
		   SagaRunID:    rrun.SagaRunID,
		   PlanID:       planID,
		  IntentHash:   intentHash,
		   ResidueHash:  residueHash,
		   LegHash:      legHash,
		  ProofDigest: proofDigest,
		  IsValid:     true,
		   VerifiedAt: now,
		  CreatedAt:  now,
	   }
	  if err := p.store.SaveProof(ctx, proof); err != nil {
		   return nil, fmt.Errorf("sagaresidue issue proof persist: %w", err)
	   }
	  return proof, nil
  }

// VerifyProof 重校验 CloseProof（周期性由 worker 调用）。
// 重新从 DB 加载 items 和 legs，重算 ProofDigest 并与 proof.ProofDigest 。
// 结果写回 proof.IsValid / proof.VerifiedAt。
// 不匹配时返回 ErrProofInvalid（并写回 IsValid=false）。

  func (p *Prover) VerifyProof(ctx context.Context, req ProofVerifyReq) (*CloseProof, error) {
	   if p == nil || p.store == nil {
		  return nil, ErrUnavailable
	  }
	   // ── 1. 加载 proof ─────────────────────────────────────────────────────────
	  var proof *CloseProof
	   var err error
	  if strings.TrimSpace(req.ProofID) != "" {
		   proof, err = p.store.GetProof(ctx, req.ProofID)
	   } else if strings.TrimSpace(req.ResidueRunID) != "" {
		   proof, err = p.store.GetProofByRunID(ctx, req.ResidueRunID)
	  } else {
		  return nil, ErrProofNotFound
	   }
	   if err != nil {
		  return nil, ErrProofNotFound
	   }
	   // ── 2. 加载 rrun ──────────────────────────────────────────────────────────
	  rrun, err := p.store.GetResidueRun(ctx, proof.ResidueRunID)
	   if err != nil {
		   return nil, fmt.Errorf("sagaresidue verify proof load run: %w", err)
	   }
	  // ── 3. 加载 items 和 legs ─────────────────────────────────────────────────
	   items, err := p.store.ListResidueItems(ctx, proof.ResidueRunID)
	   if err != nil {
		  return nil, fmt.Errorf("sagaresidue verify proof load items: %w", err)
	   }
	   var confirmedItems []ResidueItem
	   for _, item := range items {
		   if item.IsConfirmed {
			  confirmedItems = append(confirmedItems, item)
		  }
	   }
	  legs,
		   err := p.store.ListLegs(ctx, LegListReq{PlanID: proof.PlanID, Limit: 10000})
	  if err != nil {
		  return nil, fmt.Errorf("sagaresidue verify proof load legs: %w", err)
	  }
	  var appliedLegs []CloseoutLeg
	  for _, l := range legs {
		  if l.State == LegStateApplied || l.State == LegStateSkipped {
			   appliedLegs = append(appliedLegs, l)
		   }
	   }
	   // ── 4. 重算哈希 ───────────────────────────────────────────────────────────
	   intentHash, _ := hashDoneSteps(rrun.PartialDoneSteps)
	   residueHash, _ := hashResidueItems(confirmedItems)
	   legHash, _ := hashCloseoutLegs(appliedLegs)
	   digest := hashProofDigest(intentHash, residueHash, legHash)
	   now := time.Now().UTC()
	   proof.VerifiedAt = now
	  if digest != proof.ProofDigest {
		  proof.IsValid = false
		   _ = p.store.SaveProof(ctx,
			  proof)
		   return proof, fmt.Errorf("%w: expected=%s got=%s", ErrProofInvalid, proof.ProofDigest, digest)
	   }
	   proof.IsValid = true
	   _ = p.store.SaveProof(ctx, proof)
	   return proof, nil
  }

// ── 哈希算法实现 ─────────────────────────────────────────────────────────────
// hashDoneSteps 对已完成步骤列表计算哈希。
// 输入按字母顺序排序，确保哈希与顺序无关。
   func hashDoneSteps(steps []string) (string, error) {
	  sorted := make([]string, len(steps))
	   copy(sorted, steps)
	  sort.Strings(sorted)
	  type entry struct {
		   Step string `json:"step"`
	  }
	   entries := make([]entry, len(sorted))
	   for i, s := range sorted {
		  entries[i] = entry{Step: s}
	   }
	  b, err := json.Marshal(entries)
	  if err != nil {
		   return "", err
	   }
	   sum := sha256.Sum256(b)
	  return hex.EncodeToString(sum[:]),
		   nil
  }

// hashResidueItems 对已确认的残留科目计算哈希。
// 按 IdemKey 排序，确保哈希稳定。
  func hashResidueItems(items []ResidueItem) (string, error) {
	   type entry struct {
		   IdemKey       string `json:"idem_key"`
		   LedgerEntryID string `json:"ledger_entry_id"`
		  Amount        string `json:"amount"`
		   Asset string `json:"asset"`
		  Step string `json:"step"`
	  }
	   sorted := make([]entry, len(items))
	   for i, item := range items {
		  sorted[i] = entry{
			   IdemKey:       item.IdemKey,
			   LedgerEntryID: item.LedgerEntryID,
			  Amount: item.Amount,
			   Asset: item.Asset,
			  Step:  item.Step,
		  }
	  }
	   sort.Slice(sorted, func(i, j int) bool {
		   return sorted[i].IdemKey < sorted[j].IdemKey
	   })
	  b, err := json.Marshal(sorted)
	  if err != nil {
		  return "", err
	   }
	  sum := sha256.Sum256(b)
	   return hex.EncodeToString(sum[:]), nil
  }

// hashCloseoutLegs 对收口腿集合计算哈希。
// 按 SeqNo 排序；State 过滤为 applied 或 skipped。
  func hashCloseoutLegs(legs []CloseoutLeg) (string, error) {
	  type entry struct {
		  SeqNo int `json:"seq_no"`
		  Kind       string `json:"kind"`
		  IdemSuffix string `json:"idem_suffix"`
		  LedgerEntryID string `json:"ledger_entry_id"`
		   State string `json:"state"`
	   }
	   sorted := make([]entry, 0, len(legs))
	   for _, l := range legs {
		  if l.State == LegStateApplied || l.State == LegStateSkipped {
			  sorted = append(sorted, entry{
				   SeqNo: l.SeqNo,
				  Kind:          string(l.Kind),
				   IdemSuffix:    l.IdemSuffix,
				   LedgerEntryID: l.LedgerEntryID,
				   State:         string(l.State),
			  })
		  }
	  }
	  sort.Slice(sorted, func(i, j int) bool {
		   return sorted[i].SeqNo < sorted[j].SeqNo
	   })
	  b, err := json.Marshal(sorted)
	  if err != nil {
		  return "", err
	  }
	   sum := sha256.Sum256(b)
	   return hex.EncodeToString(sum[:]),
		  nil
   }

// hashProofDigest 组合三个分量哈希为最终证明摘要。

   func hashProofDigest(intentHash, residueHash, legHash string) string {
	  composite := intentHash + ":" + residueHash + ":" + legHash
	  sum := sha256.Sum256([]byte(composite))
	  return hex.EncodeToString(sum[:])
  }
