package fundintgate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ProofSummary 导出用只读摘要。
type ProofSummary struct {
	ProofID string `json:"proof_id"`
	Kind    ProofKind `json:"kind"`
	LeaseID string    `json:"int_gate_lease_id,omitempty"`
	FromUserID string `json:"from_user_id"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	BindGen int64 `json:"bind_gen"`
	LiveGen    int64     `json:"live_gen"`
	DetectedAt time.Time `json:"detected_at"`
	Digest string `json:"digest"`
}

// SummarizeProof 计算证明稳定摘要。

   func SummarizeProof(p IntGateDriftProof) ProofSummary {
	  s := ProofSummary{
		   ProofID: p.ProofID, Kind: p.Kind, LeaseID: p.LeaseID,
		  FromUserID: p.FromUserID, Asset: p.Asset, Amount: p.Amount,
		   BindGen: p.BindGen, LiveGen: p.LiveGen, DetectedAt: p.DetectedAt,
	   }
	  s.Digest = ProofDigest(p)
	  return s
  }

// ProofDigest SHA-256(kind|lease|bind_fp|live_fp|bind_gen|live_gen|evidence)。

   func ProofDigest(p IntGateDriftProof) string {
	  material := strings.Join([]string{
		   string(p.Kind), p.LeaseID, p.BindFingerprint, p.LiveFingerprint,
		  fmt.Sprintf("%d", p.BindGen), fmt.Sprintf("%d", p.LiveGen), p.Evidence,
	   }, "|")
	  sum := sha256.Sum256([]byte(material))
	  return hex.EncodeToString(sum[:])
   }

// SortProofs 按 DetectedAt 升序。
  func SortProofs(in []IntGateDriftProof) []IntGateDriftProof {
	  out := append([]IntGateDriftProof(nil), in...)
	  sort.SliceStable(out, func(i, j int) bool {
		   if out[i].DetectedAt.Equal(out[j].DetectedAt) {
			  return out[i].ProofID < out[j].ProofID
		  }
		  return out[i].DetectedAt.Before(out[j].DetectedAt)
	   })
	   return out
   }

// ProofBundle 导出包。

type ProofBundle struct {
	ExportedAt time.Time `json:"exported_at"`
	Count int            `json:"count"`
	Items []ProofSummary `json:"items"`
	Root  string         `json:"root"`
}

// BundleProofs 将证明打成带根摘要的导出包。
   func BundleProofs(now time.Time, proofs []IntGateDriftProof) ProofBundle {
	  sorted := SortProofs(proofs)
	  items := make([]ProofSummary, 0, len(sorted))
	  digests := make([]string, 0, len(sorted))
	   for _, p := range sorted {
		   s := SummarizeProof(p)
		   items = append(items, s)
		  digests = append(digests, s.Digest)
	   }
	   rootSrc := strings.Join(digests, ",")
	  sum := sha256.Sum256([]byte(rootSrc))
	   return ProofBundle{
		  ExportedAt: now.UTC(),
		   Count:      len(items),
		  Items: items,
		   Root: hex.EncodeToString(sum[:]),
	   }
   }

// MarshalBundle JSON 导出。

  func MarshalBundle(b ProofBundle) ([]byte, error) {
	  return json.Marshal(b)
  }

// LeaseView 只读租约视图，供他包消费 int_gate_lease_id。

type LeaseView struct {
	IntGateLeaseID string `json:"int_gate_lease_id"`
	State LeaseState `json:"state"`
	FromUserID string `json:"from_user_id"`
	ToUserID string `json:"to_user_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	GateGen int64 `json:"gate_gen"`
	GateFingerprint string `json:"gate_fingerprint"`
	ExpiresAt  time.Time `json:"expires_at"`
	TransferID string    `json:"transfer_id,omitempty"`
}

// ViewOf 投影租约。
  func ViewOf(l IntGateLease) LeaseView {
	   return LeaseView{
		  IntGateLeaseID: l.LeaseID, State: l.State,
		  FromUserID: l.FromUserID, ToUserID: l.ToUserID,
		   Asset: l.Asset, Amount: l.Amount, GateGen: l.GateGen,
		  GateFingerprint: l.GateFingerprint, ExpiresAt: l.ExpiresAt(),
		   TransferID: l.TransferID,
	  }
   }
