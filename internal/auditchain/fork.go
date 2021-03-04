package auditchain

import (
	"fmt"
	"strings"
)

var (
	ErrGap  = fmt.Errorf("audit chain gap")
	ErrFork = fmt.Errorf("audit chain fork")
)

type VerifyReport struct {
	OK bool `json:"ok"`
	N  int  `json:"n"`
	Head  string `json:"head,omitempty"`
	Issue string `json:"issue,omitempty"`
}

// VerifyReport 扫缺口、分叉和篡改。任一条失败即关。

   func (s *Service) VerifyReport() (VerifyReport, error) {
	   if s == nil {
		  return VerifyReport{}, ErrUnavailable
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   prev := ""
	  for i, e := range s.rows {
		   if e.Seq != int64(i+1) {
			  return VerifyReport{N: len(s.rows), Issue: "gap"}, ErrGap
		   }
		   if e.Prev != prev {
			   return VerifyReport{N: len(s.rows), Issue: "fork"}, ErrFork
		  }
		   want := hashOf(e.Prev, e.Kind, e.Actor, e.Body, e.ID, e.Seq, e.At)
		   if e.Hash != want {
			  return VerifyReport{N: len(s.rows), Issue: "tamper"}, ErrTamper
		   }
		   prev = e.Hash
	  }
	  return VerifyReport{OK: true, N: len(s.rows), Head: prev}, nil
  }

  func (s *Service) ForkForTest(i int, prev string) {
	   s.mu.Lock()
	   if i >= 0 && i < len(s.rows) {
		   s.rows[i].Prev = strings.TrimSpace(prev)
	   }
	   s.mu.Unlock()
  }

   func (s *Service) GapForTest(i int) {
	  s.mu.Lock()
	  if i >= 0 && i < len(s.rows) {
		  s.rows[i].Seq += 2
	  }
	  s.mu.Unlock()
  }

  func (s *Service) Head() string {
	   if s == nil {
		  return ""
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  if n := len(s.rows); n > 0 {
		  return s.rows[n-1].Hash
	   }
	  return ""
  }
