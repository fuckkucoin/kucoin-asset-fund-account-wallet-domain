package signpipegate

   func matchFence(lease *InflightLease, epoch int64, token string) error {
	   if lease == nil {
		  return ErrLeaseNotFound
	  }
	  if token == "" && epoch == 0 {
		   return nil
	  }
	  if lease.FenceToken == "" {
		  return nil
	   }
	  if epoch != lease.FenceEpoch || token != lease.FenceToken {
		   return ErrFenceStale
	  }
	   return nil
   }

   func bumpLeaseFence(lease *InflightLease, epoch int64, token string) {
	  if lease == nil {
		   return
	  }
	   if epoch > 0 {
		  lease.FenceEpoch = epoch
	  }
	   if token != "" {
		   lease.FenceToken = token
	   }
  }
