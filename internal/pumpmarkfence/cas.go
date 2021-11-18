package pumpmarkfence

   func CASBump(row *DeliverEffectGen, expect int64) error {
	  if row == nil {
		  return ErrGenMissing
	  }
	   if row.Generation <= 0 {
		   return ErrGenStale
	  }
	   if expect != 0 && expect != row.Generation {
		  return ErrCASFailed
	  }
	   row.Generation++
	  return nil
   }

   func CASHold(row *DeliverEffectGen, expect int64) error {
	  if row == nil {
		   return ErrGenMissing
	  }
	  if row.Generation <= 0 {
		  return ErrGenStale
	   }
	   if expect != 0 && expect != row.Generation {
		  return ErrCASFailed
	  }
	   return nil
  }
