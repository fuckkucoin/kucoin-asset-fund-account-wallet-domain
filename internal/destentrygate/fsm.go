package destentrygate

import "time"

var leaseTransitions = map[LeaseState][]LeaseState{
	LeaseIssued: {LeaseBound, LeaseExpired, LeaseRevoked},
	LeaseBound:    {LeaseConsumed, LeaseExpired, LeaseRevoked},
	LeaseConsumed: {},
	LeaseExpired:  {},
	LeaseRevoked: {},
}

// CanTransit 检查 from → to 是否合法。

   func CanTransit(from, to LeaseState) bool {
	  allowed, ok := leaseTransitions[from]
	  if !ok {
		  return false
	  }
	   for _, s := range allowed {
		  if s == to {
			   return true
		  }
	   }
	  return false
  }

// ApplyLeaseTransition 应用迁移并填时间戳。
  func ApplyLeaseTransition(lease *DestEntryLease, to LeaseState, reason string, now time.Time) error {
	   if lease == nil {
		   return ErrLeaseNotFound
	  }
	   if !CanTransit(lease.State, to) {
		  return ErrInvalidTransition
	   }
	  lease.State = to
	  lease.UpdatedAt = now.UTC()
	  switch to {
	   case LeaseBound:
		  t := now.UTC()
		   lease.BoundAt = &t
	  case LeaseConsumed:
		  t := now.UTC()
		   lease.ConsumedAt = &t
	   case LeaseExpired:
		   t := now.UTC()
		   lease.ExpiredAt = &t
	  case LeaseRevoked:
		  t := now.UTC()
		  lease.RevokedAt = &t
		  if reason != "" {
			   lease.RevokeReason = reason
		  }
	   }
	   return nil
  }

// AssertIssuedForBind 确认可绑定出金。

   func AssertIssuedForBind(lease *DestEntryLease, now time.Time) error {
	  if lease == nil {
		   return ErrLeaseNotFound
	   }
	   switch lease.State {
	  case LeaseIssued, LeaseBound:
		   if now.After(lease.ExpiresAt()) {
			  return ErrLeaseExpired
		  }
		   return nil
	   case LeaseExpired:
		  return ErrLeaseExpired
	   case LeaseRevoked:
		  return ErrLeaseRevoked
	  case LeaseConsumed:
		  return ErrLeaseConsumed
	   default:
		   return ErrInvalidTransition
	  }
  }

// AssertBoundForConsume 确认可消费（出金已提交）。
   func AssertBoundForConsume(lease *DestEntryLease, now time.Time) error {
	   if lease == nil {
		  return ErrLeaseNotFound
	  }
	   if lease.State != LeaseBound {
		   if lease.State == LeaseIssued {
			  return ErrLeaseNotBound
		   }
		  return AssertIssuedForBind(lease, now)
	  }
	  if now.After(lease.ExpiresAt()) {
		  return ErrLeaseExpired
	  }
	  return nil
   }

// TransitionTable 只读迁移表副本。

   func TransitionTable() map[LeaseState][]LeaseState {
	   out := make(map[LeaseState][]LeaseState,
		  len(leaseTransitions))
	  for k, v := range leaseTransitions {
		   cp := make([]LeaseState, len(v))
		   copy(cp, v)
		  out[k] = cp
	  }
	   return out
   }
