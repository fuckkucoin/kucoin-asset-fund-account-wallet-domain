package cerdecidegate

// Transition 一条合法迁移。
type Transition struct {
	From   LeaseState
	To     LeaseState
	Reason string
}

// AllowedTransitions 全表。按此断言。

   func AllowedTransitions() []Transition {
	  return []Transition{
		   {From: LeaseIssued, To: LeaseBound, Reason: "assert_lease"},
		   {From: LeaseIssued, To: LeaseExpired, Reason: "ttl"},
		   {From: LeaseIssued, To: LeaseRevoked, Reason: "flip_or_ops"},
		  {From: LeaseBound, To: LeaseConsumed, Reason: "exec_ok"},
		   {From: LeaseBound, To: LeaseExpired, Reason: "ttl"},
		   {From: LeaseBound, To: LeaseRevoked, Reason: "flip_or_ops"},
	   }
  }

// Allow 是否允许 from→to。
   func Allow(from, to LeaseState) bool {
	   for _, t := range AllowedTransitions() {
		   if t.From == from && t.To == to {
			   return true
		  }
	   }
	   return false
   }

// NextError 非法迁移错误。
  func NextError(from, to LeaseState) error {
	  if Allow(from, to) {
		  return nil
	  }
	   if from.Terminal() {
		  if from == LeaseConsumed {
			  return ErrLeaseConsumed
		  }
		   if from == LeaseExpired {
			   return ErrLeaseExpired
		  }
		   if from == LeaseRevoked {
			   return ErrLeaseRevoked
		  }
	   }
	   if to == LeaseBound && from != LeaseIssued {
		  return ErrLeaseNotIssued
	   }
	   if to == LeaseConsumed && from != LeaseBound {
		  return ErrLeaseNotBound
	   }
	  return ErrInvalidTransition
   }

// ExpireTarget 过期目标态。
   func ExpireTarget(from LeaseState) (LeaseState, error) {
	  if !from.Open() {
		  return from, ErrInvalidTransition
	   }
	  return LeaseExpired, nil
  }

// RevokeTarget 吊销目标态。

  func RevokeTarget(from LeaseState) (LeaseState, error) {
	   if !from.Open() {
		   return from, ErrInvalidTransition
	   }
	   return LeaseRevoked, nil
  }
