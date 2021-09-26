package ledorphan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
)

const kindIntegrityProof = "lo_integrity_proof"

// Prover 完整性证明计算与存储。
type Prover struct {
	kv persist.KV
	nowFn func() time.Time
}

// NewProver 构造 Prover。
   func NewProver(kv persist.KV) *Prover {
	  return &Prover{
		   kv:    kv,
		   nowFn: func() time.Time { return time.Now().UTC() },
	  }
   }

  func (p *Prover) now() time.Time {
	  if p.nowFn != nil {
		  return p.nowFn()
	   }
	   return time.Now().UTC()
   }

// Compute 基于三组 idem key 集合计算完整性哈希，并将证明持久化。
// 参数：
// batchID — 所属批次；
// reportID — 关联孤儿报告（可为空）；
// sealID — 关联封堵动作（可为空）；
// generation — 当前世代号；
// intentIdem — 意图集合 idem keys（原始 Items）；
// ledgerIdem — 落账集合 idem keys（ledger 已落账 hop）；
// sealIdem — 封堵集合 idem keys（已 sealed/reversed 的孤儿）。
  func (p *Prover) Compute(ctx context.Context,
	   batchID, reportID, sealID string,
	   generation int64,
	   intentIdem, ledgerIdem, sealIdem []string,
   ) (*IntegrityProof, error) {
	  if p.kv == nil {
		  return nil, ErrStorageNil
	   }
	  // 防御性拷贝并排序，确保调用方传入未排序列表时哈希仍正确。
	   intent := sortedCopy(intentIdem)
	  ledger := sortedCopy(ledgerIdem)
	  seal := sortedCopy(sealIdem)
	   hash := ComputeHash(intent, ledger, seal)
	   now := p.now()
	   proof := &IntegrityProof{
		   ProofID: uuid.NewString(),
		  BatchID:  strings.TrimSpace(batchID),
		   ReportID: strings.TrimSpace(reportID),
		   SealID:     strings.TrimSpace(sealID),
		   Generation: generation,
		   Status: ProofPending,
		   Hash:        hash,
		   IntentCount: len(intent),
		  LedgerCount: len(ledger),
		   SealCount:      len(seal),
		   IntentIdemKeys: intent,
		   LedgerIdemKeys: ledger,
		   SealIdemKeys: seal,
		   CreatedAt: now,
	  }
	  if err := p.save(ctx, proof); err != nil {
		   return nil, fmt.Errorf("ledorphan proof compute: save: %w", err)
	  }
	  return proof, nil
  }

// Verify 对已存储的 IntegrityProof 执行哈希校验。
// 用途：周期性完整性巡检或导出前的验证。
// 若哈希不一致，状态更新为 ProofInvalid，同时返回 ErrProofMismatch。

  func (p *Prover) Verify(ctx context.Context, proofID string) (*IntegrityProof, error) {
	  if p.kv == nil {
		  return nil, ErrStorageNil
	  }
	   proof, err := p.load(ctx, strings.TrimSpace(proofID))
	   if err != nil {
		  return nil, err
	   }
	   // 重算哈希；输入集合使用存储的已排序列表。
	  recomputed := ComputeHash(proof.IntentIdemKeys, proof.LedgerIdemKeys, proof.SealIdemKeys)
	  now := p.now()
	  proof.VerifiedAt = &now
	   if recomputed != proof.Hash {
		   proof.Status = ProofInvalid
		  _ = p.save(ctx, proof)
		   return proof, fmt.Errorf("%w: stored=%s recomputed=%s batch=%s",
			   ErrProofMismatch, proof.Hash, recomputed, proof.BatchID)
	  }
	  proof.Status = ProofVerified
	  if err := p.save(ctx, proof); err != nil {
		   return proof, fmt.Errorf("ledorphan proof verify: save: %w", err)
	   }
	   return proof, nil
   }

// Export 导出批次最新完整性证明（游标为空则取全部中最新一条）。
   func (p *Prover) Export(ctx context.Context, batchID string) (*IntegrityProof, error) {
	   if p.kv == nil {
		  return nil, ErrStorageNil
	   }
	  rows, err := p.kv.List(ctx, kindIntegrityProof, 200)
	   if err != nil {
		   return nil, fmt.Errorf("ledorphan proof export: list: %w", err)
	  }
	   var latest *IntegrityProof
	  for _, row := range rows {
		   var pr IntegrityProof
		  if err := json.Unmarshal(row.Body, &pr); err != nil {
			  continue
		  }
		   if pr.BatchID != strings.TrimSpace(batchID) {
			  continue
		  }
		   if latest == nil || pr.CreatedAt.After(latest.CreatedAt) {
			  cp := pr
			   latest = &cp
		   }
	  }
	   if latest == nil {
		  return nil, ErrProofNotFound
	   }
	  return latest, nil
   }

// Get 按 proofID 读取完整性证明。
  func (p *Prover) Get(ctx context.Context, proofID string) (*IntegrityProof, error) {
	   if p.kv == nil {
		  return nil, ErrStorageNil
	  }
	  return p.load(ctx, strings.TrimSpace(proofID))
   }

// ── 哈希算法（可独立单测）────────────────────────────────────────────────────
// ComputeHash 对三组已排序的 idem key 集合计算 SHA-256 完整性哈希。
// 输入要求：三组列表须已排序（调用 sortedCopy 保证）。
// 返回 64 字符小写 hex 字符串。
  func ComputeHash(intentIdem, ledgerIdem, sealIdem []string) string {
	  h := sha256.New()
	  writeSet(h, intentIdem)
	   h.Write([]byte{0x00}) // 集合间分隔符，防止跨组拼接歧义
	   writeSet(h, ledgerIdem)
	  h.Write([]byte{0x00})
	   writeSet(h, sealIdem)
	   return hex.EncodeToString(h.Sum(nil))
   }

// VerifyHash 外部校验哈希（不需要访问 KV）。
// 传入三组 idem keys 和预期哈希，若不一致返回 ErrProofMismatch。
   func VerifyHash(intentIdem, ledgerIdem, sealIdem []string, expectedHash string) error {
	   intent := sortedCopy(intentIdem)
	   ledger := sortedCopy(ledgerIdem)
	  seal := sortedCopy(sealIdem)
	  got := ComputeHash(intent, ledger, seal)
	  if got != strings.ToLower(strings.TrimSpace(expectedHash)) {
		  return fmt.Errorf("%w: expected=%s got=%s", ErrProofMismatch, expectedHash, got)
	  }
	  return nil
   }

// writeSet 将 idem key 列表以换行符为分隔符写入 sha256.Hash。

   func writeSet(h interface{ Write([]byte) (int, error) }, keys []string) {
	  for _, k := range keys {
		   _, _ = h.Write([]byte(k))
		  _, _ = h.Write([]byte{'\n'})
	  }
   }

// sortedCopy 对输入做防御性拷贝并排序，不修改原切片。

   func sortedCopy(in []string) []string {
	  if len(in) == 0 {
		  return []string{}
	  }
	   out := make([]string, len(in))
	  copy(out, in)
	   sort.Strings(out)
	  return out
  }

// ── 内部持久化 ────────────────────────────────────────────────────────────────

  func (p *Prover) save(ctx context.Context, proof *IntegrityProof) error {
	   body, err := json.Marshal(proof)
	   if err != nil {
		   return err
	   }
	   row := persist.Row{Kind: kindIntegrityProof, Key: proof.ProofID, Body: body}
	   cur, getErr := p.kv.Get(ctx, kindIntegrityProof, proof.ProofID)
	  if getErr == persist.ErrNotFound {
		   return persist.PutNew(ctx, p.kv, row)
	   }
	  if getErr != nil {
		  return getErr
	   }
	  return p.kv.Put(ctx, row, cur.Fence)
  }

   func (p *Prover) load(ctx context.Context, proofID string) (*IntegrityProof, error) {
	  row, err := p.kv.Get(ctx, kindIntegrityProof, proofID)
	   if err == persist.ErrNotFound {
		   return nil, ErrProofNotFound
	  }
	  if err != nil {
		   return nil, err
	   }
	   var proof IntegrityProof
	   if err := json.Unmarshal(row.Body, &proof); err != nil {
		  return nil, fmt.Errorf("ledorphan proof load: %w", err)
	   }
	  return &proof, nil
   }
