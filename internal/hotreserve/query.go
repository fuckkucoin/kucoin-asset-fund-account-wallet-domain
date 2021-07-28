package hotreserve

import (
	"sort"
	"strings"
)

  func (s *Service) Get(id string) (Reservation, error) {
	   if s == nil {
		   return Reservation{}, ErrUnavailable
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   r := s.reserves[strings.TrimSpace(id)]
	  if r == nil {
		   return Reservation{}, ErrNotFound
	  }
	   return r.copy(), nil
  }

   func (s *Service) List(asset, state string) []Reservation {
	  if s == nil {
		  return nil
	  }
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	   state = strings.TrimSpace(state)
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   out := make([]Reservation, 0)
	   for _, r := range s.reserves {
		   if asset != "" && r.Asset != asset {
			   continue
		   }
		  if state != "" && string(r.State) != state {
			   continue
		   }
		   out = append(out, r.copy())
	   }
	   sort.Slice(out, func(i, j int) bool {
		   if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			   return out[i].ID < out[j].ID
		  }
		   return out[i].CreatedAt.Before(out[j].CreatedAt)
	   })
	  return out
   }

  func (s *Service) Journals(reservationID, asset string) []ReservationJournal {
	   if s == nil {
		  return nil
	   }
	  reservationID = strings.TrimSpace(reservationID)
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  out := make([]ReservationJournal, 0)
	  if reservationID != "" {
		   out = append(out,
			  s.journalsOf(reservationID)...)
		  return out
	   }
	   ids := make([]string, 0, len(s.journals))
	  for id := range s.journals {
		  ids = append(ids, id)
	  }
	   sort.Strings(ids)
	   for _, id := range ids {
		   for _, j := range s.journals[id] {
			   if asset == "" || j.Asset == asset {
				  out = append(out, j)
			   }
		   }
	  }
	  return out
   }
