package occuplock

import (
	"strings"
	"sync"
	"time"
)

// Lock 一条占用锁记录。Generation 在抢占或首次签发时递增。
type Lock struct {
	Key    string
	Holder string
	Token  string
	Generation int64
	Until time.Time
	Scope string
}

  func (l Lock) Stale(now time.Time) bool {
	  if l.Until.IsZero() {
		   return true
	  }
	   return !now.Before(l.Until)
  }

  func (l Lock) SameOwner(holder, token string) bool {
	  return l.Holder == strings.TrimSpace(holder) && l.Token == strings.TrimSpace(token)
   }

// Store 占用锁持久化面。重启后同一 Store 必须仍能读到未释放锁。
type Store interface {
	Get(key string) (Lock, bool, error)
	Put(Lock) error
	Delete(key string) error
}

// Mem 可跨进程复用的内存表，用其重启：新 Service 挂同一 Mem。

type Mem struct {
	mu   sync.Mutex
	rows map[string]Lock
}

   func NewMem() *Mem {
	   return &Mem{rows: map[string]Lock{}}
  }

  func (m *Mem) Get(key string) (Lock, bool, error) {
	  if m == nil {
		  return Lock{}, false, ErrStore
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   row, ok := m.rows[key]
	   return row, ok, nil
   }

   func (m *Mem) Put(l Lock) error {
	   if m == nil {
		   return ErrStore
	   }
	  if strings.TrimSpace(l.Key) == "" {
		   return ErrKey
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   if m.rows == nil {
		   m.rows = map[string]Lock{}
	   }
	   m.rows[l.Key] = l
	   return nil
   }

  func (m *Mem) Delete(key string) error {
	  if m == nil {
		  return ErrStore
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  delete(m.rows, key)
	  return nil
  }
