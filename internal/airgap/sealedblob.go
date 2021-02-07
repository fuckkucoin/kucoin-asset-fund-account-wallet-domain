package airgap

import "time"

// SealedBlobDigest 密封 blob 摘要记录，用于重放检测。

type SealedBlobDigest struct {
	DigestID   string `json:"digest_id"`   // UUID
	DigestHex  string `json:"digest_hex"`  // sha256(blob) hex，全局唯一
	SessionID  string `json:"session_id"`  // 关联的 Session ID
	LeaseID    string `json:"lease_id"`    // 关联的 ImportLease ID
	ImportedBy string `json:"imported_by"` // actor
	ImportedAt time.Time `json:"imported_at"`
}

// Copy 深拷贝 SealedBlobDigest。

  func (d SealedBlobDigest) Copy() SealedBlobDigest {
	  return d
  }
