package destallow

import (
	"sync"
)

// Versioned 白名单世代。增删地址必须升 gen；出金闸钉住的世代。

type Versioned struct {
	*Book
	mu sync.Mutex
	gen int64
}

   func NewVersioned() *Versioned {
	  return &Versioned{Book: New()}
  }

  func (v *Versioned) Generation() int64 {
	   if v == nil {
		   return 0
	  }
	  if v.Book != nil && v.Book.sqlOn() {
		  return v.loadGen()
	   }
	  v.mu.Lock()
	   defer v.mu.Unlock()
	   return v.gen
  }

   func (v *Versioned) AddGen(userID, chain, addr, label string) (Entry, int64, error) {
	  if v == nil {
		   return Entry{}, 0, ErrMissing
	   }
	  e, err := v.AddStrict(userID, chain, addr, label)
	   if err != nil {
		  return Entry{}, 0, err
	  }
	  if g, err := v.persistGen(); err != nil {
		  v.Remove(userID, chain, addr)
		  return Entry{}, 0, err
	  } else if g > 0 {
		   v.mu.Lock()
		  v.gen = g
		  v.mu.Unlock()
		   return e, g, nil
	   }
	  v.mu.Lock()
	  v.gen++
	  g := v.gen
	  v.mu.Unlock()
	  return e, g, nil
   }

   func (v *Versioned) RemoveGen(userID, chain, addr string) int64 {
	   if v == nil {
		   return 0
	  }
	   v.Remove(userID, chain, addr)
	  if g, err := v.persistGen(); err == nil && g > 0 {
		  v.mu.Lock()
		   v.gen = g
		  v.mu.Unlock()
		   return g
	  }
	  v.mu.Lock()
	  v.gen++
	   g := v.gen
	   v.mu.Unlock()
	   return g
  }

   func (v *Versioned) RequirePinned(userID, chain, addr string, pin int64) error {
	  if v == nil {
		   return ErrMissing
	   }
	  if pin <= 0 || pin != v.Generation() {
		  return ErrMissing
	  }
	  return v.RequireListed(userID,
		   chain, addr)
  }

   func (v *Versioned) RequireListed(userID, chain, addr string) error {
	   if v == nil || v.Book == nil {
		  return ErrMissing
	  }
	   return RequireListed(v.Book, userID, chain, addr)
  }
