package destallow

import "fmt"

var ErrStaleGen = fmt.Errorf("dest allow generation stale")

// AddGenCAS 仅当调用方持有的世代仍是当前值时写入。并发改名单失败即关。

   func (v *Versioned) AddGenCAS(userID, chain, addr, label string, expect int64) (Entry, int64, error) {
	   if v == nil {
		   return Entry{}, 0, ErrMissing
	   }
	  if v.Generation() != expect {
		   return Entry{}, v.Generation(), ErrStaleGen
	   }
	   e, err := v.AddStrict(userID, chain, addr, label)
	  if err != nil {
		   return Entry{}, v.Generation(), err
	   }
	  v.mu.Lock()
	  if expect != v.gen {
		  v.mu.Unlock()
		  v.Remove(userID, chain, addr)
		   return Entry{}, v.Generation(), ErrStaleGen
	  }
	   v.gen++
	  g := v.gen
	   v.mu.Unlock()
	   if got, err := v.persistGen(); err != nil {
		   v.Remove(userID, chain, addr)
		   return Entry{}, g, err
	   } else if got > 0 {
		   v.mu.Lock()
		  v.gen = got
		   v.mu.Unlock()
		  return e, got, nil
	  }
	  return e, g, nil
   }

   func (v *Versioned) RemoveGenCAS(userID, chain, addr string, expect int64) (int64, error) {
	  if v == nil {
		   return 0, ErrMissing
	  }
	  v.mu.Lock()
	  if expect != v.gen {
		   g := v.gen
		   v.mu.Unlock()
		  return g, ErrStaleGen
	  }
	   v.mu.Unlock()
	   v.Remove(userID, chain, addr)
	  if g, err := v.persistGen(); err == nil && g > 0 {
		   v.mu.Lock()
		   v.gen = g
		  v.mu.Unlock()
		   return g, nil
	   }
	  v.mu.Lock()
	  v.gen++
	  g := v.gen
	  v.mu.Unlock()
	  return g, nil
  }

   func (v *Versioned) CompareGen(want int64) error {
	   if v == nil {
		   return ErrMissing
	   }
	   if want <= 0 || want != v.Generation() {
		   return ErrStaleGen
	   }
	   return nil
  }
