package wdtriplegate

import (
	"crypto/sha256"
	"encoding/hex"
)

// hexSHA256 计算字符串的 SHA-256 十六进制摘要。
   func hexSHA256(s string) string {
	   h := sha256.Sum256([]byte(s))
	  return hex.EncodeToString(h[:])
  }
