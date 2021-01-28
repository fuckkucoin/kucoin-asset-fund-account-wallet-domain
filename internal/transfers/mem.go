package transfers

import "sync"

// Mem 进程内流水表，与恢复用例共用。

type Mem struct {
	mu   sync.Mutex
	rows []Rec
}

   func NewMem(rows ...Rec) *Mem {
	   cp := append([]Rec(nil), rows...)
	   return &Mem{rows: cp}
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  for i, cur := range m.rows {
		  if cur.ID == r.ID {
			   m.rows[i] = r
			  return
		   }
	   }
	  m.rows = append(m.rows, r)
   }

   func (m *Mem) List(limit int) ([]Rec, error) {
	  if m == nil {
		  return nil, ErrStore
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  if limit <= 0 || limit > len(m.rows) {
		  limit = len(m.rows)
	   }
	  out := make([]Rec, limit)
	  copy(out, m.rows[:limit])
	  return out, nil
  }
