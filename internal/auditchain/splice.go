package auditchain

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Refuse struct {
	OK bool `json:"ok"`
	Reason string `json:"reason,omitempty"`
	Err string `json:"error,omitempty"`
}

   func RefuseOf(err error, rep VerifyReport) Refuse {
	  if err == nil && rep.OK {
		  return Refuse{OK: true}
	   }
	   out := Refuse{Reason: rep.Issue}
	  if err != nil {
		   out.Err = err.Error()
	   }
	  if out.Reason == "" && err != nil {
		   switch {
		   case err == ErrTamper:
			   out.Reason = "tamper"
		  case err == ErrFork:
			   out.Reason = "fork"
		  case err == ErrGap:
			  out.Reason = "gap"
		   default:
			   out.Reason = "refuse"
		   }
	   }
	   return out
   }

   func (s *Service) VerifyRefuse() (Refuse, error) {
	   rep, err := s.VerifyReport()
	   return RefuseOf(err, rep), err
   }

// SpliceForTest 在中间插入一条未链接记录，改历史。

   func (s *Service) SpliceForTest(i int, kind, actor, body string) {
	   if s == nil {
		  return
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   if i < 0 || i > len(s.rows) {
		   return
	   }
	  e := Entry{Seq: int64(i + 1), ID: "spliced", Kind: kind, Actor: actor, Body: body, Prev: "deadbeef"}
	   e.Hash = hashOf(e.Prev, e.Kind, e.Actor, e.Body, e.ID, e.Seq, e.At)
	  tail := append([]Entry{}, s.rows[i:]...)
	  s.rows = append(s.rows[:i], e)
	  s.rows = append(s.rows, tail...)
   }

   func (s *Service) DropForTest(i int) {
	  if s == nil {
		   return
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  if i < 0 || i >= len(s.rows) {
		   return
	   }
	  s.rows = append(s.rows[:i], s.rows[i+1:]...)
  }

   func (s *Service) HashForTest(i int, h string) {
	  if s == nil {
		   return
	  }
	   s.mu.Lock()
	  if i >= 0 && i < len(s.rows) {
		  s.rows[i].Hash = strings.TrimSpace(h)
	   }
	  s.mu.Unlock()
  }

   func (r Refuse) JSON() ([]byte, error) { return json.Marshal(r) }

  func CrossHeads(a, b *Service) error {
	   if a == nil || b == nil {
		   return ErrUnavailable
	   }
	  if a.Head() != b.Head() {
		   return fmt.Errorf("%w: heads diverge", ErrFork)
	  }
	   return nil
  }
