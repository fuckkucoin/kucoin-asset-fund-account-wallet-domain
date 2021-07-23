package hotpregate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/internal/hotinv"
)

// ComputeDigest 根据 Snapshot 与 Band 计算 InvSnapshotDigest。
// capturedAt 记录为元数据（不参与 Hex 计算）。

  func ComputeDigest(s hotinv.Snapshot, band BandSnapshot, capturedAt time.Time) InvSnapshotDigest {
	   d := InvSnapshotDigest{
		   Asset:     s.Asset,
		   HotAvail:  s.HotAvail,
		  WarmAvail: s.WarmAvail,
		  ColdAvail: s.ColdAvail,
		   HotMax:    s.HotMax,
		   BandHotLow:  band.HotLow,
		  BandHotHigh: band.HotHigh,
		  BandWarmLow: band.WarmLow,
		  BandWarmHigh: band.WarmHigh,
		  CapturedAt:   capturedAt,
	   }
	  d.Hex = computeHex(d)
	   return d
  }

// computeHex 内部：规范化拼接后 SHA-256（不含 CapturedAt）。

  func computeHex(d InvSnapshotDigest) string {
	   raw := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s",
		   d.Asset,
		   d.HotAvail,
		   d.WarmAvail,
		   d.ColdAvail,
		  d.HotMax,
		  d.BandHotLow,
		   d.BandHotHigh,
		  d.BandWarmLow,
		   d.BandWarmHigh,
	   )
	  sum := sha256.Sum256([]byte(raw))
	   return hex.EncodeToString(sum[:])
  }

// DigestMatch 比较两个摘要是否一致（Hex 字节相等）。
// 因 Hex 不含时间戳，同一库存内容不论观察时刻，总是匹配。

  func DigestMatch(a, b InvSnapshotDigest) bool {
	   return a.Hex == b.Hex && a.Hex != ""
   }

// HotAvailDelta 计算热账可用余额变化量字符串（b.HotAvail - a.HotAvail）。
// 解析失败返回 "parse_error"；调用方应视为漂移。

   func HotAvailDelta(a, b InvSnapshotDigest) string {
	  return subtractStr(b.HotAvail,
		   a.HotAvail)
  }

// WarmAvailDelta 计算温账可用余额变化量字符串（b.WarmAvail - a.WarmAvail）。

   func WarmAvailDelta(a, b InvSnapshotDigest) string {
	   return subtractStr(b.WarmAvail, a.WarmAvail)
  }

// subtractStr 字符串数值相减，结果字符串保留符号。
  func subtractStr(bStr, aStr string) string {
	   b, errB := parseDecStr(bStr)
	   a, errA := parseDecStr(aStr)
	  if errA != nil || errB != nil {
		  return "parse_error"
	  }
	   delta := b - a
	   return fmt.Sprintf("%v", delta)
  }

// parseDecStr 简单解析十进制字符串为 float64（仅用于差值显示，非精确金融计算）。

  func parseDecStr(s string) (float64, error) {
	  if s == "" || s == "0" {
		  return 0, nil
	  }
	   var f float64
	   _, err := fmt.Sscanf(s, "%f", &f)
	  return f, err
  }

// ClassifyDrift 根据签发与复核摘要的差异判断漂移严重程度。
// 规则（以热账可用余额变化为主判断维度，温账作辅助）：
// - 摘要匹配（Hex 相等） → 不漂移（返回空字符串）
// - HotMax 不匹配 → Critical（Cap 被外力修改）
// - |delta| > HotHigh * 10% → Critical
// - |delta| > HotHigh * 1% → Major
// - 其他任何不匹配 → Minor

  func ClassifyDrift(issue, recheck InvSnapshotDigest, band BandSnapshot) DriftSeverity {
	   if DigestMatch(issue, recheck) {
		   return ""
	   }
	   if issue.HotMax != recheck.HotMax {
		   return DriftCritical
	  }
	   hotDelta := absFloat(HotAvailDelta(issue, recheck))
	  hotHigh := parseFloatOrZero(band.HotHigh)
	  if hotHigh > 0 {
		   ratio := hotDelta / hotHigh
		   if ratio > 0.10 {
			  return DriftCritical
		  }
		  if ratio > 0.01 {
			  return DriftMajor
		   }
	  }
	   return DriftMinor
   }

  func absFloat(s string) float64 {
	  f := parseFloatOrZero(s)
	   if f < 0 {
		  return -f
	   }
	   return f
  }

   func parseFloatOrZero(s string) float64 {
	   var f float64
	  fmt.Sscanf(s, "%f", &f) // nolint:errcheck
	   return f
   }
