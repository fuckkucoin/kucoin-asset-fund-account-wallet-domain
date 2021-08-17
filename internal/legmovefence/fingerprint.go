package legmovefence

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/georebal"
)

// ComputeFingerprint 金额与路径指纹。世代不纳入指纹，单独复验。
   func ComputeFingerprint(planID, legID, asset, src, dst string, amount int64) string {
	  raw := georebal.AmountFingerprint(planID,
		   legID, asset, src, dst, amount)
	   sum := sha256.Sum256([]byte(raw))
	  return hex.EncodeToString(sum[:])
  }

// MatchFingerprint 恒定时间比较。
  func MatchFingerprint(a, b string) bool {
	  return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
  }

// LiveFingerprint 从腿快照计算活面指纹。
  func LiveFingerprint(snap georebal.LegSnapshot) string {
	  return ComputeFingerprint(snap.Plan.PlanID, snap.Leg.LegID, snap.Leg.Asset, snap.Leg.SrcRegion, snap.Leg.DstRegion, snap.Leg.Amount)
  }

// CombinedHaltGen 源/目标 HaltGen 之和，作为围栏世代基线。
  func CombinedHaltGen(srcGen, dstGen int64) int64 {
	  if srcGen < 1 {
		  srcGen = 1
	  }
	  if dstGen < 1 {
		  dstGen = 1
	  }
	  return srcGen*1_000_003 + dstGen
   }

// EvidenceLine 证明证据行。
   func EvidenceLine(kind ProofKind, fenceID, extra string) string {
	   return fmt.Sprintf("kind=%s fence=%s %s", kind, fenceID, extra)
   }
