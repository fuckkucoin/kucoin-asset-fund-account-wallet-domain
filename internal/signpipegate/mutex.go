package signpipegate

import (
	"context"
	"sync"
)

// MovementMutex 进程内互斥，配合仓级 CAS。

type MovementMutex struct {
	mu    sync.Mutex
	holds map[string]string
}

   func NewMovementMutex() *MovementMutex {
	   return &MovementMutex{holds: map[string]string{}}
   }

  func (m *MovementMutex) TryHold(movementID, leaseID string) error {
	  if m == nil {
		  return nil
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   k := NormalizeMovement(movementID)
	   if cur, ok := m.holds[k]; ok && cur != leaseID {
		  return ErrInflightOccupied
	   }
	  m.holds[k] = leaseID
	  return nil
   }

  func (m *MovementMutex) Release(movementID, leaseID string) {
	  if m == nil {
		   return
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   k := NormalizeMovement(movementID)
	  if m.holds[k] == leaseID {
		   delete(m.holds, k)
	   }
   }

  func assertNoActive(ctx context.Context, store Store, movementID, exceptID string) error {
	   if store == nil {
		  return ErrUnavailable
	  }
	  active, err := store.FindActiveLease(ctx, movementID)
	   if err != nil {
		   return err
	   }
	  if active != nil && active.ID != exceptID {
		   return ErrInflightOccupied
	  }
	  return nil
  }
