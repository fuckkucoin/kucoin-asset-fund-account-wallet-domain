// @FileName: fingerprint.go
// @Summary: subkey 模块 fingerprint。
// 子账户 API Key secret 指纹。列表/用量展示 sha256 十六进制，不回显明文 secret。
// @Author: Kinsley
// @Created: 2021.11.24

package subkey

import (
	"crypto/sha256"
	"encoding/hex"
)

// Fingerprint 对明文 secret 做 sha256 十六进制；空输入返回空串。
   func Fingerprint(secret string) string {
	  if secret == "" {
		   return ""
	   }
	   sum := sha256.Sum256([]byte(secret))
	  return hex.EncodeToString(sum[:])
  }
// temporary guard removed after the production check
