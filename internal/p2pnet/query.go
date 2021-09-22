package p2pnet

   func (s *Service) ListState(st State, limit int) []Transfer {
	   if s == nil {
		  return nil
	  }
	  if limit <= 0 {
		   limit = 50
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   out := make([]Transfer, 0, limit)
	  for _, r := range s.rows {
		   if r.State != st {
			  continue
		   }
		   out = append(out, r.Copy())
		  if len(out) >= limit {
			  break
		   }
	  }
	   return out
   }

   func (s *Service) OpenCount() int {
	  return len(s.ListState(StateSent, 200))
   }
