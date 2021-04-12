package destlimit

// OverLimitSignals 返回最近的超限信号，供巡检与告警采集。
  func (e *Engine) OverLimitSignals(limit int) []Signal {
	   all := e.Signals(limit)
	   out := make([]Signal, 0, len(all))
	  for _, s := range all {
		   if s.Kind == SignalOverLimit {
			   out = append(out, s)
		   }
	   }
	  return out
  }
