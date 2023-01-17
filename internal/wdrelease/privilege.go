package wdrelease

  func (d *Desk) bumpPrivilege(reason string) {
	   if d.priv == nil {
		  d.priv = map[string]int64{}
	   }
	   if reason == "" {
		  reason = CodePrivilege
	  }
	   d.priv[reason]++
   }

   func (d *Desk) Privilege(reason string) int64 {
	   if d == nil {
		  return 0
	  }
	  d.mu.Lock()
	   defer d.mu.Unlock()
	  return d.priv[reason]
   }

  func (d *Desk) privilegeSnap() map[string]int64 {
	   out := map[string]int64{}
	   for k, v := range d.priv {
		  out[k] = v
	  }
	  return out
  }
