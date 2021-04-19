package disputedesk

   func (s *Service) Counts() map[string]int {
	   if s == nil {
		   return nil
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  out := map[string]int{
		   "open": 0, "evidence": 0, "review": 0, "resolved": 0, "rejected": 0,
		   "fiat": 0, "p2p": 0,
	   }
	   for _, c := range s.cases {
		  out[string(c.Status)]++
		  out[string(c.Channel)]++
	   }
	   return out
   }

   func (s *Service) OpenCount(channel Channel) int {
	  if s == nil {
		   return 0
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   n := 0
	   for _, c := range s.cases {
		  if channel != "" && c.Channel != channel {
			   continue
		  }
		   if !c.Status.Terminal() {
			   n++
		  }
	  }
	   return n
  }
