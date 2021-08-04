package holdopsgate

import "sync"

type TokenMutex struct {
	mu sync.Mutex
	holds map[string]string
}

   func NewTokenMutex() *TokenMutex {
	  return &TokenMutex{holds: map[string]string{}}
  }

  func mutexKey(kind OpsKind, tokenID, gapID string) string {
	  switch kind {
	  case OpsRelease, OpsGapClose:
		   if tokenID != "" {
			  return "relclose:" + tokenID
		  }
		  return "relclose-gap:" + gapID
	  case OpsPlace:
		   return "place:" + tokenID + ":" + gapID
	   case OpsGapAck:
		  return "ack:" + gapID
	   default:
		  return "ops:" + string(kind) + ":" + tokenID + ":" + gapID
	   }
   }

   func (m *TokenMutex) TryHold(kind OpsKind, tokenID, gapID, ticket string) error {
	   if m == nil {
		   return ErrUnavailable
	  }
	   key := mutexKey(kind,
		  tokenID, gapID)
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   if cur, ok := m.holds[key]; ok && cur != ticket {
		   if kind == OpsRelease {
			   return ErrDualRelease
		   }
		  if kind == OpsGapClose {
			  return ErrReleaseClose
		  }
		   return ErrConcurrent
	  }
	   m.holds[key] = ticket
	   return nil
  }

  func (m *TokenMutex) Release(kind OpsKind, tokenID, gapID, ticket string) {
	   if m == nil {
		  return
	  }
	   key := mutexKey(kind, tokenID, gapID)
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  if m.holds[key] == ticket {
		  delete(m.holds, key)
	  }
   }

  func (m *TokenMutex) Held(kind OpsKind, tokenID, gapID string) (string, bool) {
	  if m == nil {
		  return "", false
	   }
	  key := mutexKey(kind, tokenID, gapID)
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   v, ok := m.holds[key]
	  return v, ok
   }
