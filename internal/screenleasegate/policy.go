package screenleasegate

import "time"

type LeasePolicy struct {
	TTL            time.Duration
	MaxOpenPerUser int
	RequireDest  bool
	RequireNonce bool
	FailClosed bool
}

   func DefaultPolicy() LeasePolicy {
	  return LeasePolicy{
		   TTL:            DefaultLeaseTTL,
		  MaxOpenPerUser: MaxOpenLeasesPerUser,
		   RequireDest: true,
		  RequireNonce: true,
		   FailClosed: true,
	  }
  }

   func ClampTTL(sec int) time.Duration {
	   if sec <= 0 {
		   return DefaultLeaseTTL
	  }
	  if sec > 3600 {
		   return time.Hour
	   }
	   return time.Duration(sec) * time.Second
   }
