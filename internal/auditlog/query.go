package auditlog

import "time"

// After 按 seq 游标向后翻。seq=0 从最早一条。

   func (s *Store) After(seq int64, limit int) []Entry {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  if limit <= 0 {
		   limit = 100
	  }
	   out := make([]Entry, 0, limit)
	  for _, e := range s.rows {
		   if e.Seq <= seq {
			   continue
		   }
		   out = append(out, e)
		   if len(out) >= limit {
			   break
		  }
	  }
	  return out
  }

   func (s *Store) Range(from, to time.Time, limit int) []Entry {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   if limit <= 0 {
		   limit = 100
	  }
	   out := make([]Entry, 0, limit)
	   for i := len(s.rows) - 1; i >= 0 && len(out) < limit; i-- {
		  e := s.rows[i]
		   if !from.IsZero() && e.At.Before(from) {
			  continue
		  }
		   if !to.IsZero() && e.At.After(to) {
			  continue
		  }
		  out = append(out, e)
	   }
	   return out
  }

   func (s *Store) ByActor(actor string, limit int) []Entry {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  if limit <= 0 {
		   limit = 100
	   }
	  out := make([]Entry, 0, limit)
	   for i := len(s.rows) - 1; i >= 0 && len(out) < limit; i-- {
		   if s.rows[i].Actor != actor {
			  continue
		  }
		  out = append(out, s.rows[i])
	  }
	  return out
  }

   func (s *Store) LastSeq() int64 {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  if len(s.rows) == 0 {
		  return 0
	   }
	   return s.rows[len(s.rows)-1].Seq
  }
