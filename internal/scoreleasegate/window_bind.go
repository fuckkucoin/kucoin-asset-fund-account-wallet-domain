package scoreleasegate

  func WindowJumped(frozen, live WindowSnap) bool {
	   if live.Velocity1h > frozen.Velocity1h {
		  return true
	  }
	  if live.Velocity24h > frozen.Velocity24h {
		  return true
	   }
	   if live.DestCount > frozen.DestCount {
		  return true
	  }
	  if live.DestCount24h > frozen.DestCount24h {
		  return true
	   }
	  if live.NewDests > frozen.NewDests {
		   return true
	   }
	  if live.ClusterSize > frozen.ClusterSize && frozen.ClusterSize > 0 {
		  return true
	  }
	   if frozen.Digest != "" && live.Digest != "" && frozen.Digest != live.Digest {
		  return true
	  }
	   return false
  }

  func SnapFromLease(l ScoreLease) WindowSnap {
	  return WindowSnap{
		   Velocity1h: l.Velocity1h,
		   Velocity24h: l.Velocity24h,
		   DestCount:   l.DestCount,
		   DestCount24h: l.DestCount24h,
		  ClusterSize: l.ClusterSize,
		   Correlated: l.Correlated,
		  NewDests: l.NewDests,
		  Digest:   l.WindowDigest,
	   }
   }

   func AssertWindow(lease ScoreLease, live WindowSnap) error {
	  if WindowJumped(SnapFromLease(lease), live) {
		  return ErrWindowDrift
	  }
	  if lease.WindowDigest != "" && live.Digest != "" && lease.WindowDigest != live.Digest {
		  return ErrWindowDrift
	  }
	   return nil
   }
