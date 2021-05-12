package fiataddgate

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// BoundSig 计算绑定到特定世代的签名。
// 输入形态：key + (id + ":" + amount + ":" + gen)
// 与 fiatadd.Sign 的区别：追加 ":gen" 后缀；旧签名在新 gen 下不能通过校验。

   func BoundSig(key, sessionID, amount string, gen int64) string {
	  mac := hmac.New(sha256.New, []byte(key))
	  _, _ = mac.Write([]byte(fmt.Sprintf("%s:%s:%d", sessionID, amount, gen)))
	  return hex.EncodeToString(mac.Sum(nil))
  }

// SigDigest 计算签名的 SHA-256 摘要，用于 ConfirmLease/CreditEffect 的 sig_digest 字段。
// 存摘要而非原始签名，避免 key 泄漏。
  func SigDigest(sig string) string {
	  h := sha256.Sum256([]byte(sig))
	   return hex.EncodeToString(h[:])
  }

// ValidateBoundSig 校验绑定世代签名。
// key、sessionID、amount、gen 必须与签发租约时完全一致。
// sig 区分大小写；调用方须先 strings.ToLower(strings.TrimSpace(sig))。

   func ValidateBoundSig(key, sessionID, amount, sig string, gen int64) bool {
	  key = strings.TrimSpace(key)
	   sig = strings.ToLower(strings.TrimSpace(sig))
	   if key == "" || sig == "" {
		   return false
	   }
	   want := BoundSig(key, sessionID, amount, gen)
	   return hmac.Equal([]byte(want), []byte(sig))
   }

// ValidateLegacySig 校验旧版签名（不含 gen）。仅用于向后兼容路径（fiatadd.Book.Confirm）；
// 确认闸主路径必须使用 ValidateBoundSig。
  func ValidateLegacySig(key, sessionID, amount, sig string) bool {
	  key = strings.TrimSpace(key)
	  sig = strings.ToLower(strings.TrimSpace(sig))
	  if key == "" || sig == "" {
		  return false
	  }
	   mac := hmac.New(sha256.New, []byte(key))
	  _, _ = mac.Write([]byte(sessionID + ":" + amount))
	  want := hex.EncodeToString(mac.Sum(nil))
	   return hmac.Equal([]byte(want), []byte(sig))
   }
