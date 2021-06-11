package fundmat

import "time"

// Contiguous 返回已应用序号里从 1 开始的最长无缺口前缀。空集为 0。

  func Contiguous(applied []int64) int64 {
	  if len(applied) == 0 {
		   return 0
	   }
	  set := make(map[int64]struct{}, len(applied))
	  for _, s := range applied {
		  if s > 0 {
			   set[s] = struct{}{}
		  }
	   }
	   var n int64
	   for {
		  if _, ok := set[n+1]; !ok {
			   return n
		   }
		   n++
	  }
   }

// HoleCount 统计 1..head 之间尚未应用的序号个数。head 本身未出现在日志中时不计入。
  func HoleCount(head int64, applied []int64) int64 {
	   if head <= 0 {
		   return 0
	   }
	   set := make(map[int64]struct{}, len(applied))
	   for _, s := range applied {
		   if s > 0 && s <= head {
			   set[s] = struct{}{}
		   }
	  }
	  var holes int64
	  for s := int64(1); s <= head; s++ {
		   if _, ok := set[s]; !ok {
			  holes++
		  }
	  }
	  return holes
  }

// Measure 根据日志头、已应用序号和最早未应用事件时间计算水位。
// sla 用于 WithinSLA：没有滞后时视为满足；有滞后时要求滞后毫秒不超过 sla。
  func Measure(stream Stream, epoch, head int64, applied []int64, oldestUnapplied time.Time, now time.Time, sla time.Duration) MatWatermark {
	  if epoch <= 0 {
		   epoch = 1
	  }
	   if head < 0 {
		  head = 0
	  }
	   cont := Contiguous(applied)
	  if cont > head {
		   cont = head
	  }
	  holes := HoleCount(head, applied)
	  lagEv := head - cont
	  if lagEv < 0 {
		  lagEv = 0
	  }
	   var lagMs int64
	  if lagEv > 0 && !oldestUnapplied.IsZero() {
		   lagMs = now.Sub(oldestUnapplied).Milliseconds()
		   if lagMs < 0 {
			   lagMs = 0
		   }
	  }
	  slaMs := sla.Milliseconds()
	   if slaMs < 0 {
		  slaMs = 0
	   }
	   within := lagEv == 0 || (slaMs > 0 && lagMs <= slaMs)
	  if lagEv > 0 && slaMs == 0 {
		  within = false
	  }
	  return MatWatermark{
		   Stream: stream, Epoch: epoch, HeadSeq: head, ContiguousSeq: cont,
		  AppliedCount: int64(len(applied)), HoleCount: holes,
		  LagEvents: lagEv, LagMs: lagMs, SLAMs: slaMs, WithinSLA: within,
		   UpdatedAt: now.UTC(),
	  }
  }

// OldestUnapplied 在待应用事件里取最早发生时间，供滞后毫秒计算。
  func OldestUnapplied(evs []FundingEvent) time.Time {
	   var t time.Time
	   for _, ev := range evs {
		   if ev.OccurredAt.IsZero() {
			   continue
		   }
		   if t.IsZero() || ev.OccurredAt.Before(t) {
			  t = ev.OccurredAt
		   }
	   }
	  return t
   }

// Report 汇总多流水位。任一流未追平则 CaughtUp 为假。

   func Report(streams []MatWatermark, paused bool, openAlerts int, sla time.Duration) LagReport {
	  rep := LagReport{
		   CaughtUp: true, WithinSLA: true, Paused: paused,
		   SLAMs: sla.Milliseconds(), Streams: streams, OpenAlerts: openAlerts,
	  }
	   if len(streams) == 0 {
		   return rep
	   }
	  for _, wm := range streams {
		  if !wm.CaughtUp() {
			  rep.CaughtUp = false
		   }
		  if !wm.WithinSLA {
			   rep.WithinSLA = false
		  }
		  if wm.LagEvents > rep.MaxLagEvents {
			   rep.MaxLagEvents = wm.LagEvents
		  }
		  if wm.LagMs > rep.MaxLagMs {
			  rep.MaxLagMs = wm.LagMs
		  }
	  }
	   return rep
   }
