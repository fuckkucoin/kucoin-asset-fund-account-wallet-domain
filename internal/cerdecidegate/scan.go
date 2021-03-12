package cerdecidegate

import "time"

// Deadline 租约失效时刻。
  func Deadline(l DecideLease) time.Time {
	   if l.IssuedAt.IsZero() {
		  return time.Time{}
	   }
	  if l.TTLSec <= 0 {
		   return l.IssuedAt
	   }
	  return l.IssuedAt.Add(time.Duration(l.TTLSec) * time.Second)
   }

// ExpiredAt 在 now 是否过期。
   func ExpiredAt(l DecideLease, now time.Time) bool {
	  if !l.State.Open() {
		  return false
	  }
	   d := Deadline(l)
	   if d.IsZero() {
		  return false
	   }
	  return !now.Before(d)
   }

// OpenCandidates 过滤可过期/可吊销集合。

  func OpenCandidates(in []DecideLease) []DecideLease {
	   out := make([]DecideLease, 0, len(in))
	   for _, l := range in {
		  if l.State.Open() {
			   out = append(out, l)
		  }
	  }
	  return out
  }

// MatchPolicy 策略号匹配。
  func MatchPolicy(l DecideLease, policyID string) bool {
	  if policyID == "" {
		   return true
	  }
	   return l.PolicyID == policyID
  }

// MatchBatch 批次匹配。

   func MatchBatch(l DecideLease, batchID string) bool {
	   if batchID == "" {
		  return true
	   }
	  return l.BatchID == batchID
  }
