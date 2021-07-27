package incdef

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

type TimelineHash struct {
	Host string `json:"host"`
	Mode Mode `json:"mode"`
	N     int    `json:"n"`
	Head  string `json:"head"`
	Chain string `json:"chain"`
}

  func eventHash(prev string, e Event) string {
	  h := sha256.New()
	   _, _ = h.Write([]byte(fmt.Sprintf("%s|%d|%s|%s|%s|%s|%d",
		   prev, e.Seq, e.Kind, e.Actor, e.Subject, e.Body, e.At.UnixNano())))
	  return hex.EncodeToString(h.Sum(nil))
   }

// HashTimeline 把时间线收成哈希链。改历史会断链。

  func (s *Service) HashTimeline() TimelineHash {
	  tl := s.ExportTimeline()
	  prev := ""
	   for _, e := range tl.Events {
		   prev = eventHash(prev, e)
	  }
	   return TimelineHash{
		   Host: tl.Host, Mode: tl.Mode, N: len(tl.Events), Head: prev, Chain: prev,
	  }
   }

   func (s *Service) Since(kind string) []Event {
	  tl := s.ExportTimeline()
	   kind = strings.TrimSpace(kind)
	  if kind == "" {
		  return tl.Events
	  }
	   out := make([]Event, 0)
	   for _, e := range tl.Events {
		   if e.Kind == kind {
			   out = append(out, e)
		   }
	  }
	   return out
   }

  func (s *Service) LastKind(kind string) (Event, bool) {
	  rows := s.Since(kind)
	   if len(rows) == 0 {
		   return Event{}, false
	   }
	   return rows[len(rows)-1], true
  }
