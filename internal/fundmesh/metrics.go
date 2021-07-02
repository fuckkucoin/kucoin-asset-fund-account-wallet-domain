// 重点：取消成功/拒绝、旁路未知、漂移封闭分开计数。
// 风险点：观察 ping 不得增加写计数。

// Author: EliotVoss
// Date: 2021.06.14

package fundmesh

import "sync/atomic"

type Counters struct {
	CancelOK   atomic.Int64
	CancelDeny atomic.Int64
	Idempotent  atomic.Int64
	UnknownSide atomic.Int64
	DriftClosed  atomic.Int64
	RemoteFail   atomic.Int64
	SilentDenied atomic.Int64
	ObservePing  atomic.Int64
	OutboundOK   atomic.Int64
	OutboundReplay atomic.Int64
	OutboundDeny   atomic.Int64
}

  func (c *Counters) View() map[string]int64 {
	  if c == nil {
		  return map[string]int64{}
	  }
	   return map[string]int64{
		   "cancel_ok":   c.CancelOK.Load(),
		  "cancel_deny": c.CancelDeny.Load(),
		   "idempotent":   c.Idempotent.Load(),
		  "unknown_side": c.UnknownSide.Load(),
		   "drift_closed": c.DriftClosed.Load(),
		   "remote_fail":   c.RemoteFail.Load(),
		  "silent_denied": c.SilentDenied.Load(),
		  "observe_ping":  c.ObservePing.Load(),
		   "outbound_ok":     c.OutboundOK.Load(),
		   "outbound_replay": c.OutboundReplay.Load(),
		   "outbound_deny": c.OutboundDeny.Load(),
	  }
  }
