package disputedesk

import "time"

type Event struct {
	CaseID string `json:"case_id"`
	Actor  string `json:"actor"`
	Kind   string `json:"kind"`
	Note string    `json:"note,omitempty"`
	At   time.Time `json:"at"`
}

  func (e Event) Copy() Event { return e }

   func (s *Service) noteLocked(id, actor, kind, note string) {
	   s.events = append(s.events, Event{
		  CaseID: id, Actor: actor, Kind: kind, Note: note, At: s.now(),
	   })
   }

  func (s *Service) Timeline(id string) []Event {
	  if s == nil {
		  return nil
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   out := make([]Event, 0)
	   for _, e := range s.events {
		   if id != "" && e.CaseID != id {
			  continue
		  }
		  out = append(out, e.Copy())
	  }
	   return out
   }
