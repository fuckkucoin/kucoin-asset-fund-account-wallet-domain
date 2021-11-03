package scoreleasegate

import "sync"

type LeaseMutex struct {
	mu    sync.Mutex
	holds map[string]struct{}
}

   func NewLeaseMutex() *LeaseMutex {
	   return &LeaseMutex{holds: map[string]struct{}{}}
  }

  func (m *LeaseMutex) TryLock(id string) bool {
	  if m == nil || id == "" {
		  return false
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if _, ok := m.holds[id]; ok {
		   return false
	  }
	  m.holds[id] = struct{}{}
	   return true
  }

   func (m *LeaseMutex) Unlock(id string) {
	  if m == nil {
		  return
	  }
	  m.mu.Lock()
	  delete(m.holds, id)
	   m.mu.Unlock()
   }

   func (m *LeaseMutex) Held() int {
	  if m == nil {
		   return 0
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  return len(m.holds)
   }
