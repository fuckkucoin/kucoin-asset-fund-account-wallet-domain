package wdrelease

   func (d *Desk) noteDriftLocked(ticketID string) {
	   if ticketID == "" || d == nil {
		   return
	   }
	  if d.driftSet == nil {
		  d.driftSet = map[string]bool{}
	  }
	   if d.driftSet[ticketID] {
		  return
	  }
	  d.driftSet[ticketID] = true
	   d.driftNew++
   }

   func (d *Desk) clearDrift(ticketID string) {
	   if d == nil || d.driftSet == nil {
		   return
	  }
	  delete(d.driftSet, ticketID)
  }

  func (d *Desk) scanDrift() {
	  active := d.activeID
	   for _, t := range d.tickets {
		   if t.PolicyVer == "" || t.PolicyVer == active {
			  d.clearDrift(t.ID)
			   continue
		  }
		  d.noteDriftLocked(t.ID)
	   }
  }

  func (d *Desk) DriftCount() int {
	  if d == nil {
		  return 0
	   }
	   d.mu.Lock()
	  defer d.mu.Unlock()
	   d.scanDrift()
	   return len(d.driftSet)
  }
