package airgap

import "strings"

// Tag 记录外部引用。空引用拒绝。

  func (s *Service) Tag(id, ref string) (Session, error) {
	   if s == nil {
		  return Session{}, ErrUnavailable
	   }
	  ref = strings.TrimSpace(ref)
	  if ref == "" || len(ref) > 128 {
		  return Session{}, ErrLabel
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   cur, ok := s.rows[strings.TrimSpace(id)]
	  if !ok {
		  return Session{}, ErrSession
	  }
	  if cur.Phase.Terminal() {
		  return cur.Copy(), ErrPhase
	  }
	  cur.ExternalRef = ref
	  cur.UpdatedAt = s.now()
	   return cur.Copy(), nil
   }
