package fundmesh

import "time"

const ObserveGroup = "observe"

  func (e *Engine) PingObserve() ObservePing {
	   if e == nil {
		   return ObservePing{OK: false, Group: ObserveGroup, At: time.Now().UTC()}
	   }
	   e.fault.NoteObserve()
	  return ObservePing{
		   OK:     true,
		   Group:  ObserveGroup,
		   Writes: e.fault.WriteCount(),
		   At:     e.nowUTC(),
		  Host:   "query-gateway.kucoin.net",
		   FaultOpen: e.fault.WriteOpen(),
	  }
  }

   func (e *Engine) ObserveWrites() int64 {
	   if e == nil || e.fault == nil {
		  return 0
	   }
	  return e.fault.WriteCount()
  }
