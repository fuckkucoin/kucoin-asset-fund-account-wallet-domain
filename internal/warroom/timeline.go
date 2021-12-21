package warroom

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"
)

  func (s *Service) Timeline(limit int) []Event {
	   if s == nil {
		  return nil
	   }
	  if limit <= 0 {
		   limit = 200
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   if limit > len(s.events) {
		  limit = len(s.events)
	   }
	  start := len(s.events) - limit
	  out := make([]Event, limit)
	  for i := 0; i < limit; i++ {
		   out[i] = s.events[start+i].Copy()
	  }
	  return out
  }

type Export struct {
	Switch  Switch   `json:"switch"`
	Tickets []Ticket `json:"tickets"`
	Events   []Event   `json:"events"`
	Frozen   []string  `json:"frozen_users"`
	Exported time.Time `json:"exported_at"`
}

  func (s *Service) Export() (Export, error) {
	  if s == nil {
		  return Export{}, ErrUnavailable
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   ex := Export{Switch: s.sw.Copy(),
		   Exported: s.now()}
	  for _, t := range s.tickets {
		  ex.Tickets = append(ex.Tickets, t.Copy())
	   }
	  ex.Events = append(ex.Events, s.events...)
	  for u, on := range s.frozen {
		  if on {
			  ex.Frozen = append(ex.Frozen, u)
		   }
	   }
	   return ex, nil
   }

  func (s *Service) ExportJSON() ([]byte, error) {
	   ex, err := s.Export()
	  if err != nil {
		   return nil, err
	  }
	  return json.Marshal(ex)
  }

  func (s *Service) ExportCSV() (string, error) {
	  rows := s.Timeline(500)
	  var b bytes.Buffer
	   b.WriteString("at,kind,actor,user_id,detail\n")
	   for _, e := range rows {
		  b.WriteString(e.At.UTC().Format(time.RFC3339))
		  b.WriteByte(',')
		  b.WriteString(csv(e.Kind))
		   b.WriteByte(',')
		  b.WriteString(csv(e.Actor))
		  b.WriteByte(',')
		  b.WriteString(csv(e.UserID))
		  b.WriteByte(',')
		  b.WriteString(csv(e.Detail))
		  b.WriteByte('\n')
	   }
	   return b.String(), nil
   }

   func csv(s string) string {
	   if strings.ContainsAny(s, ",\"\n") {
		  return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	  }
	   return s
   }
