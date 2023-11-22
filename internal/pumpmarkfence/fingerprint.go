package pumpmarkfence

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode"
)

const (
	hashDomainPayload = "outbox.pump.payload.v1"
	hashDomainEffect = "outbox.pump.effect.v1"
	hashDomainResume = "outbox.pump.resume.v1"
)

// NormalizePayload 折叠首尾空白，保留正文内部换行。

  func NormalizePayload(s string) string {
	  s = strings.TrimSpace(s)
	  if s == "" {
		   return ""
	   }
	   var b strings.Builder
	  b.Grow(len(s))
	   prevSpace := false
	   for _, r := range s {
		  if r == '\n' || r == '\r' || r == '\t' {
			  if !prevSpace {
				  b.WriteByte(' ')
				   prevSpace = true
			   }
			  continue
		  }
		  if unicode.IsSpace(r) && r != ' ' {
			   if !prevSpace {
				  b.WriteByte(' ')
				  prevSpace = true
			   }
			  continue
		   }
		  if r == ' ' {
			  if !prevSpace {
				   b.WriteByte(' ')
				   prevSpace = true
			   }
			  continue
		   }
		   prevSpace = false
		   b.WriteRune(r)
	  }
	   return strings.TrimSpace(b.String())
   }

// PayloadHash 规范化 payload 的世代指纹。
   func PayloadHash(kind, key, payload string) string {
	  kind = strings.TrimSpace(kind)
	   key = strings.TrimSpace(key)
	  payload = NormalizePayload(payload)
	   h := sha256.New()
	   _,
		  _ = h.Write([]byte(hashDomainPayload))
	   _, _ = h.Write([]byte{'\n'})
	   _, _ = h.Write([]byte(kind))
	  _, _ = h.Write([]byte{'\n'})
	  _, _ = h.Write([]byte(key))
	  _, _ = h.Write([]byte{'\n'})
	  _, _ = h.Write([]byte(payload))
	   return hex.EncodeToString(h.Sum(nil))
   }

// EffectKey 业务效果互斥键：outbox_id + payload_hash + 初始世代域。
// 同一出箱事件的全额效果只允许落地一次，毒丸重投不得另开键。

   func EffectKey(outboxID, payloadHash string) string {
	  outboxID = strings.TrimSpace(outboxID)
	   payloadHash = strings.TrimSpace(payloadHash)
	   h := sha256.New()
	   _, _ = h.Write([]byte(hashDomainEffect))
	  _, _ = h.Write([]byte{'\n'})
	  _, _ = h.Write([]byte(outboxID))
	  _, _ = h.Write([]byte{'\n'})
	  _, _ = h.Write([]byte(payloadHash))
	  return hex.EncodeToString(h.Sum(nil))
   }

// ResumeToken 半截续提互斥令牌。
  func ResumeToken(caseID, genID, holder string) string {
	   h := sha256.New()
	   _, _ = h.Write([]byte(hashDomainResume))
	   _, _ = h.Write([]byte{'\n'})
	   _, _ = h.Write([]byte(strings.TrimSpace(caseID)))
	   _, _ = h.Write([]byte{'\n'})
	   _, _ = h.Write([]byte(strings.TrimSpace(genID)))
	  _, _ = h.Write([]byte{'\n'})
	  _, _ = h.Write([]byte(strings.TrimSpace(holder)))
	  return hex.EncodeToString(h.Sum(nil))
   }

// FingerprintMatch 严格。

  func FingerprintMatch(stored, computed string) bool {
	  stored = strings.TrimSpace(stored)
	  computed = strings.TrimSpace(computed)
	   if stored == "" || computed == "" {
		  return false
	   }
	   return stored == computed
  }

  func requireIDs(outboxID, kind, key, payload string) error {
	  if strings.TrimSpace(outboxID) == "" {
		   return ErrInvalidInput
	  }
	  if strings.TrimSpace(kind) == "" || strings.TrimSpace(key) == "" {
		   return ErrInvalidInput
	  }
	   if strings.TrimSpace(payload) == "" {
		  return ErrInvalidInput
	  }
	   return nil
  }
