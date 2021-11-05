package screenleasegate

import "sync"

type LeaseMutex struct {
	mu sync.Mutex
	holds map[string]string
}

   func NewLeaseMutex() *LeaseMutex {
	   return &LeaseMutex{holds: map[string]string{}}
   }

  func (m *LeaseMutex) TryHold(leaseID, ticket string) error {
	   if m == nil {
		  return ErrUnavailable
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   if cur, ok := m.holds[leaseID]; ok && cur != ticket {
		  return ErrConcurrent
	   }
	   m.holds[leaseID] = ticket
	   return nil
  }

  func (m *LeaseMutex) Release(leaseID, ticket string) {
	  if m == nil {
		  return
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   if m.holds[leaseID] == ticket {
		  delete(m.holds, leaseID)
	   }
  }
