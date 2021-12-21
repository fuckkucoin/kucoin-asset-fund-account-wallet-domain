package userdir

import (
	"errors"
	"strings"
	"sync"
)

var ErrUnknown = errors.New("recipient not found")

type Directory interface {
	Resolve(kind, value string) (string, error)
	Bind(kind,
		value, userID string) error
}

type Mem struct {
	mu sync.RWMutex
	rows map[string]string
}

  func NewMem() *Mem { return &Mem{rows: map[string]string{}} }

   func (m *Mem) Bind(kind, value, userID string) error {
	   kind, value = norm(kind, value)
	   userID = strings.TrimSpace(userID)
	   if kind == "" || value == "" || userID == "" {
		   return ErrUnknown
	   }
	  m.mu.Lock()
	   m.rows[kind+":"+value] = userID
	   m.mu.Unlock()
	   return nil
  }

  func (m *Mem) Resolve(kind, value string) (string, error) {
	  kind, value = norm(kind, value)
	  if kind == "uid" {
		  if value == "" {
			  return "", ErrUnknown
		  }
		  return value, nil
	   }
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  got, ok := m.rows[kind+":"+value]
	  if !ok || got == "" {
		  return "", ErrUnknown
	   }
	   return got, nil
  }

  func norm(kind, value string) (string, string) {
	   kind = strings.ToLower(strings.TrimSpace(kind))
	  value = strings.TrimSpace(value)
	   switch kind {
	  case "uid", "user", "user_id":
		   return "uid", value
	  case "email", "mail":
		   return "email", strings.ToLower(value)
	  case "phone", "mobile":
		   return "phone", value
	  default:
		   return kind, value
	   }
   }
