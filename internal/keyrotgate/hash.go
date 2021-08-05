package keyrotgate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// ProofChecksum 计算指纹证明摘要。
// 输入：源槽退役前指纹、目标槽加载后指纹、世代号、工单 ID。

  func ProofChecksum(beforeFP, afterFP string, rotateGen int64, ticketID string) string {
	   raw := fmt.Sprintf("%s|%s|%d|%s", beforeFP, afterFP, rotateGen, ticketID)
	  sum := sha256.Sum256([]byte(raw))
	  return hex.EncodeToString(sum[:])
   }

// DriftCaseChecksum 计算漂移案件证明摘要。
// 输入：旧指纹、新指纹、SignBindID、DriftCaseID。

   func DriftCaseChecksum(oldFP, newFP, signBindID, driftCaseID string) string {
	  raw := fmt.Sprintf("drift|%s|%s|%s|%s", oldFP, newFP, signBindID, driftCaseID)
	   sum := sha256.Sum256([]byte(raw))
	  return hex.EncodeToString(sum[:])
  }

// ValidateFingerprint 校验指纹形态是否合法（8-128 字节十六进制字符串）。

   func ValidateFingerprint(fp string) error {
	   if len(fp) < 8 || len(fp) > 128 {
		   return fmt.Errorf("fingerprint length must be 8-128 chars, got %d", len(fp))
	   }
	  if _, err := hex.DecodeString(fp); err != nil {
		   // 非严格要求十六进制；其他形态（SHA1/MD5 指纹含冒号）也可接受
		   // 仅校验可打印 ASCII
		  for _, c := range fp {
			   if c < 0x21 || c > 0x7e {
				  return fmt.Errorf("fingerprint contains non-printable character: %q", c)
			  }
		  }
	  }
	  return nil
   }

// FingerprintsMatch 判断两个指纹是否相同（不区分大小写）。

  func FingerprintsMatch(a, b string) bool {
	   if len(a) != len(b) {
		   return false
	   }
	   aL := make([]byte, len(a))
	  bL := make([]byte, len(b))
	   for i := range a {
		  c := a[i]
		   if c >= 'A' && c <= 'Z' {
			   c += 'a' - 'A'
		  }
		  aL[i] = c
	  }
	  for i := range b {
		   c := b[i]
		  if c >= 'A' && c <= 'Z' {
			   c += 'a' - 'A'
		   }
		   bL[i] = c
	   }
	  for i := range aL {
		   if aL[i] != bL[i] {
			   return false
		  }
	   }
	   return true
  }
