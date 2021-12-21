package warroom

import "time"

// GlobalFrozenAll 返回当前全站冻结布尔值及操作者/原因/时间（只读）。
// 实现 wrfreeze.LegacySwitchReader 接口。

   func (s *Service) GlobalFrozenAll() (frozen bool, actor, reason string, at time.Time) {
	  if s == nil {
		   return false, "", "", time.Time{}
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  sw := s.sw
	   return sw.FrozenAll, sw.Actor, sw.Reason, sw.At
  }
