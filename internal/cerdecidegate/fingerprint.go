package cerdecidegate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// LineDigest 行事实指纹。
   func LineDigest(ln LineFact) string {
	   raw := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%t",
		  strings.TrimSpace(ln.BatchID),
		   strings.ToUpper(strings.TrimSpace(ln.Asset)),
		   strings.ToUpper(strings.TrimSpace(ln.Chain)),
		   strings.TrimSpace(ln.Amount),
		   strings.TrimSpace(ln.Dest),
		   strings.ToLower(strings.TrimSpace(ln.DestRisk)),
		   strings.ToLower(strings.TrimSpace(ln.Side)),
		  strings.ToLower(strings.TrimSpace(ln.Purpose)),
		  ln.Approved,
	  )
	  sum := sha256.Sum256([]byte("cerdecide|line|" + raw))
	   return hex.EncodeToString(sum[:])
  }

// PackDigest 策略包指纹（仅钉扎字段，不解释排序）。

  func PackDigest(policyID string, force bool, threshold, shares int, destRisk, bindMode string, required bool) string {
	   raw := fmt.Sprintf("%s|%t|%d|%d|%s|%s|%t",
		   strings.TrimSpace(policyID), force, threshold, shares,
		   strings.ToLower(strings.TrimSpace(destRisk)),
		  strings.TrimSpace(bindMode), required)
	  sum := sha256.Sum256([]byte("cerdecide|pack|" + raw))
	   return hex.EncodeToString(sum[:])
  }

// ComposeFingerprint 合成租约指纹。
   func ComposeFingerprint(pin DecidePin, ln LineFact, packDigest, snapDigest string, gen int64) PolicyFingerprint {
	  if packDigest == "" {
		   packDigest = PackDigest(pin.PolicyID,
			  pin.Required, pin.Threshold, pin.Shares, pin.DestRisk, pin.BindMode, pin.Required)
	   }
	  line := LineDigest(ln)
	   if snapDigest == "" {
		  snapDigest = line
	   }
	  fp := PolicyFingerprint{
		   PolicyID:  strings.TrimSpace(pin.PolicyID),
		   ForceCold: pin.Required,
		  Threshold: pin.Threshold,
		   Shares: pin.Shares,
		  BindMode: strings.TrimSpace(pin.BindMode),
		  DestRisk:   strings.ToLower(strings.TrimSpace(pin.DestRisk)),
		   Required:   pin.Required,
		   PackDigest: packDigest,
		   LineDigest: line,
		   SnapDigest: snapDigest,
		   Generation: gen,
	   }
	   fp.Digest = FingerprintDigest(fp)
	   return fp
  }

// FingerprintDigest 整枚指纹摘要。
   func FingerprintDigest(fp PolicyFingerprint) string {
	  raw := fmt.Sprintf("%s|%t|%d|%d|%s|%s|%t|%s|%s|%s|%d",
		   fp.PolicyID, fp.ForceCold, fp.Threshold, fp.Shares, fp.BindMode, fp.DestRisk,
		   fp.Required, fp.PackDigest, fp.LineDigest, fp.SnapDigest, fp.Generation)
	   sum := sha256.Sum256([]byte("cerdecide|fp|" + raw))
	   return hex.EncodeToString(sum[:])
   }

// CompareLive 复验活指纹与租约钉扎。
// 做什么：按先级返回最具体漂移错误。
// 不重算 Decide 排序。
// 不变量：风险档抬升先于字段漂移。
   func CompareLive(quoted PolicyFingerprint, live PolicyFingerprint) error {
	   if quoted.Digest == live.Digest && quoted.Digest != "" {
		  return nil
	  }
	   if live.Generation > 0 && quoted.Generation > 0 && live.Generation != quoted.Generation {
		   return ErrGenStale
	   }
	  if RiskRank(live.DestRisk) > RiskRank(quoted.DestRisk) {
		   return ErrRiskRaise
	   }
	  if quoted.ForceCold && !live.ForceCold {
		  return ErrForceFlip
	  }
	   if live.ForceCold != quoted.ForceCold {
		   return ErrForceFlip
	  }
	  if live.Threshold != quoted.Threshold {
		   return ErrThresholdDrift
	   }
	   if live.Shares != quoted.Shares {
		   return ErrSharesDrift
	  }
	   if live.PolicyID != quoted.PolicyID {
		  return ErrPolicyDrift
	   }
	  if live.SnapDigest != "" && quoted.SnapDigest != "" && live.SnapDigest != quoted.SnapDigest {
		   return ErrSnapDrift
	  }
	  if live.LineDigest != "" && quoted.LineDigest != "" && live.LineDigest != quoted.LineDigest {
		  return ErrFieldDrift
	   }
	  if live.PackDigest != "" && quoted.PackDigest != "" && live.PackDigest != quoted.PackDigest {
		  return ErrPolicyDrift
	   }
	   if live.Digest != quoted.Digest {
		   return ErrLeaseMismatch
	  }
	   return nil
  }

// ProofKindOf 把复验错误映射到证明类型。
   func ProofKindOf(err error) ProofKind {
	   if err == nil {
		  return ProofBindOK
	  }
	  switch err {
	   case ErrPolicyDrift:
		  return ProofPolicyDrift
	  case ErrThresholdDrift:
		  return ProofThreshDrift
	  case ErrSharesDrift:
		  return ProofSharesDrift
	   case ErrRiskRaise:
		   return ProofRiskRaise
	   case ErrForceFlip:
		   return ProofForceFlip
	   case ErrSnapDrift:
		   return ProofSnapDrift
	  case ErrFieldDrift:
		   return ProofFieldDrift
	  case ErrLeaseExpired:
		  return ProofExpired
	  case ErrLeaseRevoked:
		  return ProofRevoked
	  case ErrNoLease:
		   return ProofNoLease
	   case ErrExemptMisuse:
		  return ProofExemptMisuse
	   case ErrGenStale:
		   return ProofGenStale
	   case ErrTTL:
		  return ProofTTLExpired
	  default:
		   return ProofFieldDrift
	  }
   }

// ScopeKey 世代作用域：批次先，否则资产+链。
  func ScopeKey(ln LineFact) string {
	  if b := strings.TrimSpace(ln.BatchID); b != "" {
		  return "batch:" + b
	   }
	  a := strings.ToUpper(strings.TrimSpace(ln.Asset))
	   c := strings.ToUpper(strings.TrimSpace(ln.Chain))
	   if a == "" {
		   a = "*"
	   }
	  if c == "" {
		   c = "*"
	   }
	   return "asset:" + a + ":" + c
   }

// PolicyScope 按策略号吊销。

  func PolicyScope(policyID string) string {
	  id := strings.TrimSpace(policyID)
	  if id == "" {
		   return "policy:*"
	   }
	  return "policy:" + id
  }
