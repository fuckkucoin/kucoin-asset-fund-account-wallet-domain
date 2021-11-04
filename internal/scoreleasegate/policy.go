package scoreleasegate

import "time"

type GatePolicy struct {
	TTL            time.Duration
	MaxOpenPerUser int
	ConsumeOnSubmit bool
	AllowShadowIssue bool
	EngineName string
}

  func DefaultPolicy() GatePolicy {
	  return GatePolicy{
		  TTL: DefaultLeaseTTL,
		   MaxOpenPerUser: MaxOpenLeasesPerUser,
		  ConsumeOnSubmit: true,
		  AllowShadowIssue: false,
		  EngineName: "riskscore-engine",
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

   func (p GatePolicy) ShadowForbidden() bool {
	  return !p.AllowShadowIssue
   }
