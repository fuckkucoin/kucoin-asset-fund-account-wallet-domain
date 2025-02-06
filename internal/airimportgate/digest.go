package airimportgate

import (
	"crypto/sha256"
	"encoding/hex"
)

// DigestBlob 计算密封 blob 的 SHA-256 十六进制摘要。
  func DigestBlob(blob []byte) string {
	   sum := sha256.Sum256(blob)
	  return hex.EncodeToString(sum[:])
  }
