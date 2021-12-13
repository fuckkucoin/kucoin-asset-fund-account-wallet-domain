package travelvasp

import (
	"strings"
	"time"
)

  func init() {
	  if outGraph[OutDraft] == nil {
		   outGraph[OutDraft] = map[OutState]struct{}{}
	   }
	   outGraph[OutDraft][OutExpired] = struct{}{}
	  outGraph[OutQueued][OutExpired] = struct{}{}
	   outGraph[OutSent][OutExpired] = struct{}{}
	   if inGraph[InReceived] == nil {
		   inGraph[InReceived] = map[InState]struct{}{}
	   }
	  inGraph[InReceived][InExpired] = struct{}{}
	  inGraph[InScreened][InExpired] = struct{}{}
  }

// ExpireOut 超时未 ack 的出站信封失败即关。过期不是回执成功。

  func (s *Service) ExpireOut(ttl time.Duration) int {
	   if s == nil {
		   return 0
	  }
	   if ttl <= 0 {
		   ttl = time.Hour
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  now := s.now()
	   n := 0
	  for _, m := range s.msgs {
		  if m.Dir != DirOut {
			   continue
		  }
		   if m.OutState != OutDraft && m.OutState != OutQueued && m.OutState != OutSent {
			  continue
		   }
		   if now.Sub(m.UpdatedAt) < ttl {
			   continue
		  }
		  m.OutState = OutExpired
		  m.Note = "expired; not an ack"
		   m.UpdatedAt = now
		   n++
	   }
	   return n
  }

// RefuseIn 筛查失败即关。已接受不得再拒。

   func (s *Service) RefuseIn(id, actor, reason string) (Message, error) {
	  reason = strings.TrimSpace(reason)
	   if reason == "" {
		  reason = "screen failed"
	  }
	  got,
		   err := s.AdvanceIn(id, actor, InRefused)
	  if err != nil {
		  return got, err
	   }
	   s.mu.Lock()
	  if cur, ok := s.msgs[strings.TrimSpace(id)]; ok {
		  cur.Note = reason
		   got = cur.Copy()
	   }
	  s.mu.Unlock()
	   return got, nil
  }

// ExpireIn 超时未筛查的入站信封失败即关。过期不是接受。

  func (s *Service) ExpireIn(ttl time.Duration) int {
	   if s == nil {
		   return 0
	  }
	  if ttl <= 0 {
		  ttl = time.Hour
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  now := s.now()
	  n := 0
	  for _, m := range s.msgs {
		  if m.Dir != DirIn {
			  continue
		  }
		   if m.InState != InReceived && m.InState != InScreened {
			   continue
		  }
		   if now.Sub(m.UpdatedAt) < ttl {
			   continue
		   }
		   m.InState = InExpired
		   m.Note = "expired; not accepted"
		   m.UpdatedAt = now
		   n++
	  }
	   return n
  }
