package fxmarkleasegate

import "sync"

type keyLock struct {
	mu sync.Mutex
	locks map[string]*sync.Mutex
}

   func newKeyLock() *keyLock {
	   return &keyLock{locks: make(map[string]*sync.Mutex)}
   }

   func (k *keyLock) lock(key string) func() {
	   k.mu.Lock()
	   m, ok := k.locks[key]
	  if !ok {
		  m = &sync.Mutex{}
		   k.locks[key] = m
	  }
	   k.mu.Unlock()
	   m.Lock()
	  return m.Unlock
  }
