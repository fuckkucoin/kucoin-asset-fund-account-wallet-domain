package auditchain

import (
	"fmt"
	"strings"
)

var ErrCover = fmt.Errorf("audit chain coverage missing")

// VerifyRange 核一段序号。缺号或哈希对不上失败即关。

   func (s *Service) VerifyRange(from, to int64) error {
	   if s == nil {
		  return ErrUnavailable
	   }
	  if from <= 0 || to < from {
		   return ErrTamper
	   }
	   if err := s.Verify(); err != nil {
		   return err
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   if int64(len(s.rows)) < to {
		   return ErrTamper
	  }
	   for i := from; i <= to; i++ {
		   e := s.rows[i-1]
		  if e.Seq != i {
			  return ErrTamper
		  }
	  }
	   return nil
   }

// Covered 要求这些 kind 都至少落过一条。运维动作漏记不得当已覆盖。
  func (s *Service) Covered(kinds ...string) error {
	  if s == nil {
		  return ErrUnavailable
	  }
	  if len(kinds) == 0 {
		   return ErrCover
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  seen := map[string]struct{}{}
	  for _, e := range s.rows {
		   seen[e.Kind] = struct{}{}
	  }
	  for _, k := range kinds {
		  k = strings.TrimSpace(k)
		   if k == "" {
			   return ErrKind
		   }
		  if _, ok := seen[k]; !ok {
			   return fmt.Errorf("%w: %s", ErrCover, k)
		  }
	  }
	  return nil
  }

  func (s *Service) Len() int {
	   if s == nil {
		  return 0
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  return len(s.rows)
   }
