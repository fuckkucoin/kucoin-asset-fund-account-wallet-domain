package depoccup

   func (s *Service) auditLocked(a Audit) {
	  if a.ID == "" {
		  a.ID = s.nextID("aud")
	   }
	  if a.At.IsZero() {
		   a.At = s.now()
	  }
	   if a.Actor == "" {
		  a.Actor = "occupancy"
	   }
	  s.audits = append(s.audits, a)
	  if len(s.audits) > 5000 {
		  s.audits = s.audits[len(s.audits)-5000:]
	   }
	   if err := s.saveAudit(a); err != nil {
		   s.saveErr = err.Error()
	   }
  }

   func (s *Service) Audits(lotID string, limit int) []Audit {
	  if s == nil {
		   return nil
	   }
	   if limit <= 0 || limit > 200 {
		   limit = 50
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  out := make([]Audit, 0)
	  for i := len(s.audits) - 1; i >= 0; i-- {
		   a := s.audits[i]
		   if lotID != "" && a.LotID != lotID && a.UserID != lotID {
			   continue
		   }
		   out = append(out, a)
		   if len(out) >= limit {
			   break
		   }
	  }
	  return out
   }
