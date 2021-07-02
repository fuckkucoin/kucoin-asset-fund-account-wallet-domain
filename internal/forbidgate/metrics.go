package forbidgate

import (
	"sync/atomic"
	"time"
)

// Metrics 禁提闸业务指标（轻量计数，不依赖 Prometheus 包以避免额外依赖）。
// 中可替换为 prometheus.Counter 等，此处用原子计数器做基线。
type Metrics struct {
	GenBumpTotal int64 // SetForbiddenGen 调用次数
	BindIssuedTotal     int64 // IssueWdPrefightBind 调用次数
	BindRevokedTotal    int64 // RevokeBind 调用次数
	BindConsumedTotal   int64 // CheckAndConsumeBind 成功消费次数
	DriftCaseOpenTotal  int64 // 新建 ForbidDriftCase 次数
	DriftCaseCloseTotal int64 // ForbidDriftCase 关闭次数
	// driftDurationSumMs 漂移案例从 open 到 close 的累计毫秒数，用于均值计算。
	driftDurationSumMs int64
	driftDurationCount int64
}

// IncrGenBump 禁提世代抬升计数。
  func (m *Metrics) IncrGenBump() {
	   if m == nil {
		  return
	   }
	  atomic.AddInt64(&m.GenBumpTotal, 1)
  }

// IncrBindIssued 预检绑定发放计数。

   func (m *Metrics) IncrBindIssued() {
	  if m == nil {
		  return
	  }
	   atomic.AddInt64(&m.BindIssuedTotal,
		  1)
  }

// IncrBindRevoked 预检绑定吊销计数。
  func (m *Metrics) IncrBindRevoked() {
	   if m == nil {
		  return
	  }
	  atomic.AddInt64(&m.BindRevokedTotal, 1)
   }

// IncrBindConsumed 预检绑定消费计数。

   func (m *Metrics) IncrBindConsumed() {
	   if m == nil {
		  return
	   }
	  atomic.AddInt64(&m.BindConsumedTotal, 1)
   }

// IncrDriftCaseOpen 漂移案例开启计数。
  func (m *Metrics) IncrDriftCaseOpen() {
	   if m == nil {
		  return
	   }
	  atomic.AddInt64(&m.DriftCaseOpenTotal, 1)
  }

// IncrDriftCaseClose 漂移案例关闭计数，记录持续时长。
  func (m *Metrics) IncrDriftCaseClose(detectedAt time.Time) {
	   if m == nil {
		   return
	   }
	   atomic.AddInt64(&m.DriftCaseCloseTotal,
		  1)
	  dur := time.Since(detectedAt).Milliseconds()
	  atomic.AddInt64(&m.driftDurationSumMs, dur)
	  atomic.AddInt64(&m.driftDurationCount, 1)
   }

// AvgDriftDurationMs 漂移案例平均持续时长（毫秒）；无数据时返回 0。
   func (m *Metrics) AvgDriftDurationMs() int64 {
	  if m == nil {
		   return 0
	   }
	   cnt := atomic.LoadInt64(&m.driftDurationCount)
	  if cnt == 0 {
		  return 0
	  }
	  return atomic.LoadInt64(&m.driftDurationSumMs) / cnt
   }

// Snapshot 返回当前计数快照（copy-on-read，线程安全）。

   func (m *Metrics) Snapshot() map[string]int64 {
	  if m == nil {
		  return nil
	  }
	  return map[string]int64{
		   "gen_bump_total":     atomic.LoadInt64(&m.GenBumpTotal),
		  "bind_issued_total":  atomic.LoadInt64(&m.BindIssuedTotal),
		  "bind_revoked_total": atomic.LoadInt64(&m.BindRevokedTotal),
		  "bind_consumed_total":    atomic.LoadInt64(&m.BindConsumedTotal),
		  "drift_case_open_total":  atomic.LoadInt64(&m.DriftCaseOpenTotal),
		  "drift_case_close_total": atomic.LoadInt64(&m.DriftCaseCloseTotal),
		   "avg_drift_duration_ms": m.AvgDriftDurationMs(),
	   }
   }
