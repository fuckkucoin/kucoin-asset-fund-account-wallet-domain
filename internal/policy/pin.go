package policy

import (
	"fmt"
	"sync"
)

// Pin 把评估时的代次钉在单据上。后续发布不得改已钉代次的结论来源。
type PinBook struct {
	mu sync.Mutex
	pins map[string]int
}

  func NewPinBook() *PinBook { return &PinBook{pins: map[string]int{}} }

   func (b *PinBook) Bind(id string, version int) error {
	   if b == nil {
		   return fmt.Errorf("policy pin unavailable")
	  }
	  if version <= 0 {
		  return fmt.Errorf("policy version required")
	  }
	  id = trimID(id)
	   if id == "" {
		   return fmt.Errorf("policy pin id required")
	  }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   if cur, ok := b.pins[id]; ok && cur != version {
		   return fmt.Errorf("policy pin conflict")
	   }
	  b.pins[id] = version
	   return nil
  }

  func (b *PinBook) Must(id string, live int) error {
	   if b == nil {
		   return fmt.Errorf("policy pin unavailable")
	   }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  got, ok := b.pins[trimID(id)]
	   if !ok {
		   return fmt.Errorf("policy pin missing")
	  }
	   if got != live {
		   return fmt.Errorf("policy pin mismatch")
	  }
	  return nil
  }

  func (b *PinBook) Version(id string) (int, bool) {
	   if b == nil {
		  return 0, false
	   }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  v, ok := b.pins[trimID(id)]
	   return v, ok
  }

  func trimID(s string) string {
	   for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		  s = s[1:]
	   }
	  for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		   s = s[:len(s)-1]
	  }
	   return s
   }
