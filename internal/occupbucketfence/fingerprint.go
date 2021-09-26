package occupbucketfence

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/occupbook"
)

// FingerprintLot 计算批次桶指纹。
  func FingerprintLot(lot *occupbook.Lot) string {
	  if lot == nil {
		   return ""
	  }
	   return FingerprintRaw(lot.LotID, lot.Buckets.Inflight.String(), lot.Buckets.Available.String(),
		   lot.Buckets.Void.String(), lot.OccupiedTotal.String(), lot.Generation)
  }

// FingerprintRaw 稳定指纹。

   func FingerprintRaw(lotID, inflight, available, voidAmt, total string, gen int64) string {
	   payload := strings.Join([]string{
		  strings.TrimSpace(lotID),
		   canonDec(inflight),
		  canonDec(available),
		   canonDec(voidAmt),
		   canonDec(total),
		   fmt.Sprintf("%d", gen),
	  }, "|")
	  sum := sha256.Sum256([]byte(payload))
	   return hex.EncodeToString(sum[:])
  }

  func canonDec(s string) string {
	  s = strings.TrimSpace(s)
	  if s == "" {
		   return "0"
	   }
	  return s
   }

// MatchFingerprint 比较调用方声明与当前批次。
   func MatchFingerprint(lot *occupbook.Lot, declared string, gen int64) error {
	  if lot == nil {
		  return ErrLotRequired
	  }
	  if strings.TrimSpace(declared) == "" {
		   return ErrFenceMismatch
	   }
	   if gen != 0 && gen != lot.Generation {
		  return ErrGenerationStale
	  }
	   want := FingerprintLot(lot)
	  if !strings.EqualFold(want, strings.TrimSpace(declared)) {
		  return ErrFenceMismatch
	   }
	  return nil
   }

// ProofDigest 击穿证明摘要（导出 golden 用）。

   func ProofDigest(p *OccupancyBreakProof) string {
	  if p == nil {
		   return ""
	  }
	  payload := strings.Join([]string{
		  p.LotID, p.FenceID, p.Kind, p.Inflight, p.Available, p.Void, p.Sum, p.Total, p.Delta, p.Reason,
	   }, "|")
	  sum := sha256.Sum256([]byte(payload))
	  return hex.EncodeToString(sum[:])
  }
