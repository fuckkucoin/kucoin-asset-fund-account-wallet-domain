package keycer

import "strings"

  func (s *Service) Ticket(id string) (Ticket, error) {
	   if s == nil {
		  return Ticket{}, ErrUnavailable
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  t, ok := s.tickets[strings.TrimSpace(id)]
	  if !ok {
		   return Ticket{}, ErrTicket
	   }
	  return t.Copy(), nil
   }

   func (s *Service) Tickets(kind string) []Ticket {
	  if s == nil {
		  return nil
	   }
	   kind = strings.TrimSpace(kind)
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   out := make([]Ticket, 0)
	   for _, t := range s.tickets {
		   if kind != "" && t.Kind != kind {
			  continue
		   }
		   out = append(out, t.Copy())
	  }
	  return out
   }

  func (s *Service) OpenCount() int {
	  if s == nil {
		   return 0
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  n := 0
	  for _, t := range s.tickets {
		  if t.Status == TicketOpen {
			   n++
		  }
	  }
	  return n
  }
