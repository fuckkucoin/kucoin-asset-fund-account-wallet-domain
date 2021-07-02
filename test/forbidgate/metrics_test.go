package forbidgate_test

import (
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/forbidgate"
)

// TestMetrics_NilSafe nil Metrics 的所有方法不应 panic。
  func TestMetrics_NilSafe(t *testing.T) {
	   var m *forbidgate.Metrics
	   m.IncrGenBump()
	  m.IncrBindIssued()
	   m.IncrBindRevoked()
	  m.IncrBindConsumed()
	  m.IncrDriftCaseOpen()
	  m.IncrDriftCaseClose(time.Now())
	   if m.AvgDriftDurationMs() != 0 {
		  t.Error("nil metrics avg should be 0")
	  }
	   if m.Snapshot() != nil {
		  t.Error("nil metrics snapshot should be nil")
	   }
   }

// TestMetrics_IncrAndSnapshot 计数器增加后快照值正确。

   func TestMetrics_IncrAndSnapshot(t *testing.T) {
	  m := &forbidgate.Metrics{}
	  cases := []struct {
		   name   string
		  fn     func()
		  key    string
		   expect int64
	   }{
		  {"gen_bump", m.IncrGenBump, "gen_bump_total", 1},
		  {"bind_issued", m.IncrBindIssued, "bind_issued_total", 1},
		   {"bind_revoked", m.IncrBindRevoked, "bind_revoked_total", 1},
		   {"bind_consumed", m.IncrBindConsumed, "bind_consumed_total", 1},
		  {"drift_open", m.IncrDriftCaseOpen, "drift_case_open_total", 1},
	   }
	   for _, tc := range cases {
		  tc.fn()
		   snap := m.Snapshot()
		  if v := snap[tc.key]; v != tc.expect {
			  t.Errorf("%s: want %d, got %d", tc.name, tc.expect, v)
		   }
	  }
   }

// TestMetrics_DriftDuration 漂移持续时长平均值计算正确。

   func TestMetrics_DriftDuration(t *testing.T) {
	   m := &forbidgate.Metrics{}
	  // 记录两次漂移关闭，持续时间约 0（单元中 time.Since 极小）。
	  m.IncrDriftCaseClose(time.Now())
	  m.IncrDriftCaseClose(time.Now())
	   snap := m.Snapshot()
	   if snap["drift_case_close_total"] != 2 {
		   t.Errorf("expected 2 closes, got %d", snap["drift_case_close_total"])
	   }
	   // 平均时长 >= 0。
	   if avg := m.AvgDriftDurationMs(); avg < 0 {
		   t.Errorf("avg drift duration should be >= 0, got %d", avg)
	   }
   }

// TestMetrics_ConcurrentIncr 并发递增计数器线程安全。

   func TestMetrics_ConcurrentIncr(t *testing.T) {
	  m := &forbidgate.Metrics{}
	  const n = 100
	  done := make(chan struct{}, n)
	  for i := 0; i < n; i++ {
		   go func() {
			   m.IncrGenBump()
			  m.IncrBindIssued()
			  done <- struct{}{}
		   }()
	  }
	   for i := 0; i < n; i++ {
		  <-done
	   }
	  snap := m.Snapshot()
	   if snap["gen_bump_total"] != n {
		   t.Errorf("want gen_bump=%d, got %d", n, snap["gen_bump_total"])
	   }
	  if snap["bind_issued_total"] != n {
		   t.Errorf("want bind_issued=%d, got %d", n, snap["bind_issued_total"])
	   }
  }
