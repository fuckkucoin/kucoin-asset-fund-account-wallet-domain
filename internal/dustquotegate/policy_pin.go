package dustquotegate

import "strings"

// PinRecheck 复验 floor/min/cap 钉扎。

   func PinRecheck(quoted PolicyRateFingerprint,
	   live PolicyRateFingerprint) error {
	   if strings.TrimSpace(quoted.FloorPinned) != strings.TrimSpace(live.FloorPinned) {
		  return ErrPolicyDrift
	  }
	  if strings.TrimSpace(quoted.MinPinned) != strings.TrimSpace(live.MinPinned) {
		   return ErrPolicyDrift
	  }
	   if quoted.CapBPS != live.CapBPS {
		   return ErrCapDrift
	   }
	   return nil
  }

// MustSameDigest 签发后 digest 不得被改写。
   func MustSameDigest(lease DustQuoteLease) error {
	  want := DigestFingerprint(lease.Fingerprint)
	  if want != lease.Digest {
		   return ErrLeaseMismatch
	  }
	  return nil
  }
