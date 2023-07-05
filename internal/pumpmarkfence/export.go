package pumpmarkfence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"
)

// ExportBundle 一轮证明导出。
type ExportBundle struct {
	ExportedAt string `json:"exported_at"`
	Count  int           `json:"count"`
	Digest string        `json:"digest"`
	Proofs []ProofDigest `json:"proofs"`
}

// ExportProofs 导出未导出证明并打标。

  func (e *Engine) ExportProofs(ctx context.Context, limit int, actor string) (ExportBundle, error) {
	   out := ExportBundle{}
	  if e == nil {
		   return out, ErrUnavailable
	   }
	  if ctx == nil {
		   ctx = context.Background()
	   }
	  if limit <= 0 {
		  limit = 80
	  }
	   rows,
		   err := e.proofs.ListUnexported(ctx, limit)
	   if err != nil {
		  return out, err
	  }
	   now := e.now()
	   digests := make([]ProofDigest, 0, len(rows))
	   for i := range rows {
		  p := rows[i]
		   p.Exported = true
		   p.ExportedAt = now
		  if err := e.proofs.Save(ctx, &p); err != nil {
			  continue
		   }
		  d := p.Digest()
		   // 导出摘要不绑随机 proof_id 时间以外的易变字段时保留 proof_id 以便追查。
		   digests = append(digests, d)
		   e.audit(ctx, p.GenID, p.OutboxID, actorOr(actor), ActionExport, OutcomeOK, string(p.Kind), p.Generation)
	   }
	   sort.Slice(digests, func(i, j int) bool {
		  if digests[i].OutboxID == digests[j].OutboxID {
			   return digests[i].Kind < digests[j].Kind
		   }
		   return digests[i].OutboxID < digests[j].OutboxID
	   })
	   out.Proofs = digests
	  out.Count = len(digests)
	  out.ExportedAt = now.UTC().Format(time.RFC3339Nano)
	  out.Digest = BundleDigest(digests)
	   recordExport(out.Count)
	  return out, nil
  }

// BundleDigest 证明摘要链。
  func BundleDigest(rows []ProofDigest) string {
	  h := sha256.New()
	  _, _ = h.Write([]byte("outbox.dup.proof.bundle.v1\n"))
	   for _, r := range rows {
		   _, _ = h.Write([]byte(r.OutboxID))
		   _, _ = h.Write([]byte{'|'})
		  _, _ = h.Write([]byte(r.EffectKey))
		   _, _ = h.Write([]byte{'|'})
		  _, _ = h.Write([]byte(r.Kind))
		  _, _ = h.Write([]byte{'|'})
		   _, _ = h.Write([]byte(r.GenID))
		  _, _ = h.Write([]byte{'\n'})
	  }
	   return hex.EncodeToString(h.Sum(nil))
  }

// StableProofSummary 去掉随机 ID 后的轨迹摘要，供夹具 expected。
type StableProofSummary struct {
	OutboxID string `json:"outbox_id"`
	Kind     string `json:"kind"`
	Generation int64 `json:"generation"`
	Detail string `json:"detail"`
}

   func StableSummaries(rows []OutboxDupProof) []StableProofSummary {
	   out := make([]StableProofSummary, 0, len(rows))
	  for _, r := range rows {
		   out = append(out, StableProofSummary{
			  OutboxID: r.OutboxID, Kind: string(r.Kind), Generation: r.Generation, Detail: r.Detail,
		  })
	  }
	  sort.Slice(out, func(i, j int) bool {
		  if out[i].OutboxID == out[j].OutboxID {
			  return out[i].Kind < out[j].Kind
		  }
		  return out[i].OutboxID < out[j].OutboxID
	  })
	   return out
  }

   func marshalStable(v any) string {
	  b, err := json.Marshal(v)
	   if err != nil {
		   return ""
	  }
	   return strings.TrimSpace(string(b))
  }
