package stablemintgate

import (
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/stablebr"
)

// BindSpec 指纹绑定规格。
type BindSpec struct {
	Kind   LeaseKind
	Asset  string
	Digest string
	Expiry time.Time
	MintGen  int64
	Required []string
}

  func SpecFromLease(l *MintLease) BindSpec {
	  if l == nil {
		   return BindSpec{}
	   }
	  return BindSpec{
		   Kind: l.Kind, Asset: l.Asset, Digest: l.Fingerprint.Digest,
		   Expiry:  l.Fingerprint.Expiry,
		   MintGen: l.MintGen, Required: append([]string(nil), l.Fingerprint.Items...),
	   }
  }

// CompareFingerprint 当前桥清单与租约规格。
   func CompareFingerprint(now time.Time, spec BindSpec, live stablebr.Fingerprint) error {
	   if spec.Digest == "" {
		  return ErrInvalidInput
	   }
	   if !live.Complete {
		  if live.Expiry.IsZero() || !live.Expiry.After(now) {
			  return ErrAttestExpired
		  }
		  return ErrAttestIncomplete
	  }
	  if !strings.EqualFold(live.Asset, spec.Asset) {
		   return ErrAssetMismatch
	  }
	  if string(live.Kind) != string(spec.Kind) {
		   return ErrKindMismatch
	  }
	   if live.Digest != spec.Digest {
		   return ErrFingerprintDrift
	  }
	  if spec.Expiry.IsZero() || !spec.Expiry.After(now) || !live.Expiry.After(now) {
		  return ErrAttestExpired
	  }
	   return nil
   }

// RequiredAligned 检查指纹条目与 kind 所需集合一致。

   func RequiredAligned(kind LeaseKind, items []string) bool {
	  need := RequiredBridgeItems(kind)
	  if len(need) != len(items) {
		  return false
	   }
	   seen := map[string]bool{}
	  for _, it := range items {
		  seen[it] = true
	   }
	  for _, it := range need {
		  if !seen[string(it)] {
			   return false
		   }
	  }
	  return true
  }

// DriftDetail 供证明仓写入。
  func DriftDetail(want, got string) string {
	  return fmt.Sprintf("fp want=%s got=%s", want, got)
   }
