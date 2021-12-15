package usageleasegate

import (
	"sync"
	"account-assets.kucoin.net/funding/pkg/money"
)

type keyLock struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

   func newKeyLock() *keyLock {
	  return &keyLock{locks: make(map[string]*sync.Mutex)}
  }

   func (k *keyLock) lock(key string) func() {
	  k.mu.Lock()
	   lk, ok := k.locks[key]
	   if !ok {
		   lk = &sync.Mutex{}
		  k.locks[key] = lk
	  }
	  k.mu.Unlock()
	  lk.Lock()
	  return lk.Unlock
   }

   func SumHeld(held []UsageReservation) money.Amount {
	   sum := money.Zero()
	  for _, r := range held {
		  if r.State != ResHeld {
			   continue
		  }
		   amt, err := money.Parse(r.Amount)
		  if err != nil {
			  continue
		  }
		  sum = MustAdd(sum, amt)
	  }
	   return sum
  }

   func HeldExcept(held []UsageReservation, leaseID string) money.Amount {
	  sum := money.Zero()
	   for _, r := range held {
		  if r.State != ResHeld || r.LeaseID == leaseID {
			  continue
		  }
		  amt, err := money.Parse(r.Amount)
		   if err != nil {
			  continue
		  }
		   sum = MustAdd(sum, amt)
	   }
	  return sum
  }
