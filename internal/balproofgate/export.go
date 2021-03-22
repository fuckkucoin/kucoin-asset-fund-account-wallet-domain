package balproofgate

import (
	"encoding/json"
	"strings"
	"time"
)

// ExportDigest 将导出包压成稳定 JSON 再哈希前的规范对象。

type ExportDigest struct {
	ProofLeaseID string `json:"proof_lease_id"`
	Status     string `json:"status"`
	Generation int64  `json:"generation"`
	Avail      string `json:"available"`
	Frozen string `json:"frozen"`
	ObserveGen int64 `json:"observe_gen"`
	RootDigest string `json:"root_digest"`
	Signature  string `json:"signature"`
	DriftIDs   string `json:"drift_ids"`
	Revoked       bool   `json:"revoked"`
	IsolationMark string `json:"isolation_mark"`
	ExportedAt string `json:"exported_at"`
}

// SummarizeExport 从包生成摘要对象。
   func SummarizeExport(b *ExportBundle) ExportDigest {
	   if b == nil {
		  return ExportDigest{IsolationMark: IsolationMark}
	  }
	  ids := make([]string, 0, len(b.Drifts))
	   for _, d := range b.Drifts {
		   ids = append(ids, d.ID)
	  }
	   at := b.ExportedAt
	   if at.IsZero() {
		  at = time.Now().UTC()
	   }
	  return ExportDigest{
		   ProofLeaseID: b.ProofLeaseID,
		  Status:       string(b.Lease.Status),
		  Generation: b.Lease.Generation,
		   Avail:  b.Lease.Snapshot.Available,
		  Frozen: b.Lease.Snapshot.Frozen,
		  ObserveGen: b.Lease.Snapshot.ObserveGen,
		   RootDigest: b.Lease.Snapshot.RootDigest,
		  Signature:  b.Lease.Signature,
		  DriftIDs:   strings.Join(ids, ","),
		  Revoked:       b.Revocation != nil,
		  IsolationMark: IsolationMark,
		   ExportedAt: at.UTC().Format(time.RFC3339Nano),
	  }
  }

// MarshalExport 稳定序列化。

   func MarshalExport(b *ExportBundle) ([]byte, error) {
	  d := SummarizeExport(b)
	   return json.Marshal(d)
   }
