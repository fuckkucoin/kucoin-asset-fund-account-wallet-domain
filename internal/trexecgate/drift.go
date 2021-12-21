package trexecgate

import (
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/treasury"
)

// DriftTolerance 相对漂移容忍率，超过此值即视为漂移过大。
// 5% = 0.05；即 Record 时 HotAvail 为 100，复核时低于 95 则触发拒绝。

const DriftTolerance = "0.05"

// DriftResult 漂移检测结果。
type DriftResult struct {
	Exceeded bool
	RecordSnap  SnapshotDigest
	RecheckSnap SnapshotDigest
	DriftAbs    string
	DriftRatio  string
	Reason string
}

// CheckDrift 核对 Record 时快照与当前 Observe 快照，判断漂移是否超限。
// 功能：
// 1. 热账低于 Band.Target 直接触发拒绝（无论漂移比例）。
// 2. 相对漂移 (recordHot - recheckHot) / recordHot 超过 DriftTolerance 触发拒绝。

   func CheckDrift(recordSnap SnapshotDigest, recheckSnap SnapshotDigest, band treasury.Band) (DriftResult, error) {
	  recordHot,
		   err := decimal.NewFromString(orZ(recordSnap.HotAvail))
	  if err != nil {
		   return DriftResult{}, fmt.Errorf("%w: record hot_avail parse: %v", ErrSnapshotUnavailable, err)
	   }
	  recheckHot,
		  err := decimal.NewFromString(orZ(recheckSnap.HotAvail))
	  if err != nil {
		  return DriftResult{}, fmt.Errorf("%w: recheck hot_avail parse: %v", ErrSnapshotUnavailable, err)
	  }
	   target, err := decimal.NewFromString(orZ(band.Target))
	  if err != nil {
		  return DriftResult{}, fmt.Errorf("%w: band target parse: %v", ErrSnapshotUnavailable, err)
	  }
	  tolerance, _ := decimal.NewFromString(DriftTolerance)
	   driftAbs := recordHot.Sub(recheckHot)
	  var driftRatio decimal.Decimal
	   if recordHot.IsPositive() {
		  driftRatio = driftAbs.Div(recordHot)
	  }
	  res := DriftResult{
		   RecordSnap:  recordSnap,
		  RecheckSnap: recheckSnap,
		   DriftAbs: driftAbs.String(),
		   DriftRatio: driftRatio.String(),
	  }
	  // 热账低于 target：归集目标本身不再满足，直接拒绝。
	  if recheckHot.LessThan(target) {
		   res.Exceeded = true
		  res.Reason = fmt.Sprintf("hot_avail %s dropped below band target %s", recheckHot.String(), target.String())
		  return res, nil
	  }
	   // 相对下跌超过容忍率拒绝。
	  if driftRatio.GreaterThan(tolerance) {
		   res.Exceeded = true
		  res.Reason = fmt.Sprintf("relative drift %s exceeds tolerance %s", driftRatio.String(), DriftTolerance)
		   return res, nil
	   }
	   return res, nil
   }

// BuildDriftProof 构造 TreasDriftProof。
// 功能：将检测结果固化为可追溯证明；ProofID 使用 UUID。

   func BuildDriftProof(leaseID, execID, asset string, dr DriftResult, now time.Time) *TreasDriftProof {
	   decision := "blocked"
	  if !dr.Exceeded {
		  decision = "pass"
	  }
	   return &TreasDriftProof{
		   ProofID: uuid.NewString(),
		  LeaseID: leaseID,
		  ExecID: execID,
		  Asset: strings.ToUpper(strings.TrimSpace(asset)),
		  RecordDigest: dr.RecordSnap,
		  RecheckDigest: dr.RecheckSnap,
		   DriftAbs:      dr.DriftAbs,
		   DriftRatio: dr.DriftRatio,
		   Decision:  decision,
		   Reason:    dr.Reason,
		   CreatedAt: now.UTC(),
	   }
  }

// orZ 空字符串转零，避免解析报错。
   func orZ(s string) string {
	   s = strings.TrimSpace(s)
	  if s == "" {
		   return "0"
	  }
	   return s
   }
