package auditchain

import "fmt"

// Gaps 检查序号连续。缺号或跳号失败即关。

  func (s *Service) Gaps() error {
	   if s == nil {
		  return ErrUnavailable
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  for i, e := range s.rows {
		  want := int64(i + 1)
		  if e.Seq != want {
			  return fmt.Errorf("%w: have %d want %d", ErrGap, e.Seq, want)
		   }
		  if i > 0 && e.Prev != s.rows[i-1].Hash {
			  return ErrTamper
		  }
	  }
	  return nil
  }

// Missing 返回尚未落链的 kind。空列表表示覆盖完整。
   func (s *Service) Missing(kinds ...string) []string {
	  if s == nil {
		   return append([]string(nil), kinds...)
	  }
	  if err := s.Covered(kinds...); err == nil {
		   return nil
	   }
	  out := make([]string, 0)
	  s.mu.Lock()
	   seen := map[string]struct{}{}
	  for _, e := range s.rows {
		  seen[e.Kind] = struct{}{}
	  }
	   s.mu.Unlock()
	   for _, k := range kinds {
		   if _, ok := seen[k]; !ok {
			   out = append(out, k)
		  }
	   }
	  return out
   }

// PunchSeqForTest 把一条序号改坏，只给缺口用。

   func (s *Service) PunchSeqForTest(i int, seq int64) {
	   if s == nil {
		   return
	   }
	  s.mu.Lock()
	  if i >= 0 && i < len(s.rows) {
		   s.rows[i].Seq = seq
	  }
	   s.mu.Unlock()
   }
