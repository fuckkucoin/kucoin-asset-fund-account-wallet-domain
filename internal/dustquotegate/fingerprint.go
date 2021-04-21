package dustquotegate

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
)

// DigestFingerprint 计算策略牌价指纹摘要。
   func DigestFingerprint(f PolicyRateFingerprint) string {
	  var b strings.Builder
	   b.WriteString(strings.ToUpper(strings.TrimSpace(f.Asset)))
	   b.WriteByte('|')
	  b.WriteString(strings.TrimSpace(f.Rate))
	   b.WriteByte('|')
	   b.WriteString(strconv.FormatInt(f.PolicyGen,
		  10))
	  b.WriteByte('|')
	   b.WriteString(strconv.FormatInt(f.CapBPS, 10))
	   b.WriteByte('|')
	  b.WriteString(strings.TrimSpace(f.FloorPinned))
	   b.WriteByte('|')
	   b.WriteString(strings.TrimSpace(f.MinPinned))
	   b.WriteByte('|')
	  b.WriteString(strings.TrimSpace(f.BookFP))
	  b.WriteByte('|')
	   b.WriteString(strings.TrimSpace(f.BookAvail))
	   sum := sha256.Sum256([]byte(b.String()))
	  return hex.EncodeToString(sum[:])
   }

// BookFingerprint 用户资产账面可用钉扎。

  func BookFingerprint(user, asset, avail string) string {
	  raw := strings.TrimSpace(user) + "|" + strings.ToUpper(strings.TrimSpace(asset)) + "|" + strings.TrimSpace(avail)
	   sum := sha256.Sum256([]byte(raw))
	  return hex.EncodeToString(sum[:12])
  }

// CompareLive 把租约指纹与活快照，返回第一处漂移错误。

  func CompareLive(quoted PolicyRateFingerprint, live PolicyRateFingerprint) error {
	  if strings.TrimSpace(quoted.Rate) != strings.TrimSpace(live.Rate) {
		  return ErrRateDrift
	   }
	   if quoted.PolicyGen != live.PolicyGen {
		  return ErrPolicyDrift
	   }
	   if quoted.CapBPS != live.CapBPS {
		   return ErrCapDrift
	  }
	  if strings.TrimSpace(quoted.BookFP) != strings.TrimSpace(live.BookFP) {
		   return ErrBookDrift
	   }
	  if DigestFingerprint(quoted) != DigestFingerprint(live) {
		   return ErrLeaseMismatch
	  }
	   return nil
  }

// ProofKindOf 把错误映射到证明类型。
  func ProofKindOf(err error) ProofKind {
	  switch err {
	   case ErrRateDrift:
		  return ProofRateDrift
	  case ErrPolicyDrift:
		   return ProofPolicyDrift
	  case ErrBookDrift:
		   return ProofBookDrift
	  case ErrCapDrift:
		   return ProofCapDrift
	  case ErrLeaseExpired:
		  return ProofExpired
	  case ErrLeaseRevoked:
		   return ProofRevoked
	   case ErrConserve:
		  return ProofConserveFail
	  case ErrScanHalted:
		   return ProofScanHalt
	  case ErrNoLease:
		   return ProofNoLease
	  case ErrDupBind:
		   return ProofDupBind
	   default:
		   return ProofRevoked
	   }
   }

// SameIdemPayload 幂等键冲突检测：用户/资产/数量必须一致。

   func SameIdemPayload(a, b DustQuoteLease) bool {
	   return a.UserID == b.UserID && a.Asset == b.Asset && a.Amount == b.Amount
  }
