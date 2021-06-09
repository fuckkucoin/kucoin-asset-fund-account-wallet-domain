package fundintgate

import "time"

var leaseTransitions = map[LeaseState][]LeaseState{
	LeaseGated: {LeaseBound,
		LeaseExpired, LeaseRevoked},
	LeaseBound: {LeaseSent,
		LeaseExpired, LeaseRevoked},
	LeaseSent:    {},
	LeaseExpired: {},
	LeaseRevoked: {},
}

// CanTransit 检查租约 from → to 是否合法。

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

// ApplyLeaseTransition 验证并应用状态迁移，填充对应时间戳。
  func ApplyLeaseTransition(lease *IntGateLease, to LeaseState, reason string, now time.Time) error {
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
	  case LeaseSent:
		  t := now.UTC()
		   lease.SentAt = &t
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

// AssertGatedForBind 确认租约可绑定 Send。

  func AssertGatedForBind(lease *IntGateLease, now time.Time) error {
	   if lease == nil {
		  return ErrLeaseNotFound
	   }
	   switch lease.State {
	   case LeaseGated:
		  if now.After(lease.ExpiresAt()) {
			  return ErrLeaseExpired
		   }
		   return nil
	   case LeaseBound:
		  if now.After(lease.ExpiresAt()) {
			  return ErrLeaseExpired
		  }
		  return nil
	  case LeaseExpired:
		  return ErrLeaseExpired
	   case LeaseRevoked:
		  return ErrLeaseRevoked
	   case LeaseSent:
		   return ErrLeaseConsumed
	  default:
		  return ErrInvalidTransition
	   }
  }

// AssertBoundForSend 确认租约可落 SpotSend。

  func AssertBoundForSend(lease *IntGateLease, now time.Time) error {
	  if lease == nil {
		   return ErrLeaseNotFound
	   }
	  if lease.State != LeaseBound {
		   if lease.State == LeaseGated {
			   return ErrLeaseNotBound
		   }
		  return AssertGatedForBind(lease, now)
	   }
	  if now.After(lease.ExpiresAt()) {
		   return ErrLeaseExpired
	   }
	  return nil
   }

// TransitionTable 返回只读迁移表副本，供文档与。

  func TransitionTable() map[LeaseState][]LeaseState {
	  out := make(map[LeaseState][]LeaseState, len(leaseTransitions))
	   for k, v := range leaseTransitions {
		   cp := make([]LeaseState, len(v))
		   copy(cp, v)
		  out[k] = cp
	   }
	  return out
   }
