package swapthaw

import (
	"fmt"
	"time"
)

// leaseTransitions 合法的 Lease 状态转换表。

var leaseTransitions = map[LeaseStatus][]LeaseStatus{
	LeaseStatusOpen: {
		LeaseStatusSettled,
		LeaseStatusThawed,
		LeaseStatusBlocked,
	},
	// 终态不可再转。
	LeaseStatusSettled: {},
	LeaseStatusThawed:  {},
	LeaseStatusBlocked: {},
}

// TransitLease 执行 FreezeLease 状态转换，并记录转换时间戳。
// 调用方必须在持库事务（InTx）内调用，并随后持久化 Lease。

   func TransitLease(lease *FreezeLease, to LeaseStatus, now time.Time) error {
	   if lease == nil {
		  return fmt.Errorf("swapthaw: TransitLease: nil lease")
	  }
	  allowed, ok := leaseTransitions[lease.Status]
	  if !ok {
		   return fmt.Errorf("%w: unknown source status %q", ErrLeaseConflict, lease.Status)
	  }
	   for _, a := range allowed {
		   if a == to {
			   lease.Status = to
			  lease.UpdatedAt = now
			   if !to.IsTerminal() {
				  // 只有终态才记录 WinnerLeg；此分支理论上不触发，留作防御。
				  return nil
			   }
			   return nil
		  }
	   }
	   return fmt.Errorf("%w: cannot transit Lease from %q to %q", ErrLeaseConflict, lease.Status, to)
   }

// TransitLeaseWithLeg 执行 Lease 转换并同时记录胜出终态腿。
// settle 调用时 leg=BindLegSettle；failUnfreeze 调用时 leg=BindLegFailUnfreeze。

  func TransitLeaseWithLeg(lease *FreezeLease, to LeaseStatus, leg BindLeg, now time.Time) error {
	  if err := TransitLease(lease, to, now); err != nil {
		   return err
	  }
	   if to.IsTerminal() {
		   lease.WinnerLeg = leg
	   }
	   return nil
  }

// thawCaseTransitions 合法的 ThawCase 状态转换表。

var thawCaseTransitions = map[ThawCaseStatus][]ThawCaseStatus{
	ThawCaseStatusOpen: {ThawCaseStatusProven},
	ThawCaseStatusProven: {ThawCaseStatusClosed},
	ThawCaseStatusClosed: {},
}

// TransitThawCase 执行 ThawCase 状态转换，校验证明完整性。
// open→proven：要求 ProofPayload 非空。
// proven→closed：无额外前置条件（调用方须在事务内先 GetCase 后再 TransitThawCase）。

  func TransitThawCase(tc *ThawCase, to ThawCaseStatus, now time.Time) error {
	  if tc == nil {
		  return fmt.Errorf("swapthaw: TransitThawCase: nil case")
	   }
	   allowed, ok := thawCaseTransitions[tc.Status]
	  if !ok {
		  return fmt.Errorf("%w: unknown ThawCase source status %q", ErrThawCaseProofMissing, tc.Status)
	  }
	  for _, a := range allowed {
		  if a == to {
			  if to == ThawCaseStatusProven && tc.ProofPayload == "" {
				   return ErrThawCaseProofMissing
			   }
			   tc.Status = to
			   tc.UpdatedAt = now
			   switch to {
			   case ThawCaseStatusProven:
				   t := now
				   tc.ProvenAt = &t
			  case ThawCaseStatusClosed:
				   t := now
				  tc.ClosedAt = &t
			   }
			   return nil
		  }
	  }
	  return fmt.Errorf("%w: cannot transit ThawCase from %q to %q", ErrThawCaseProofMissing, tc.Status, to)
   }

// ValidateLeaseForClaim 校验 FreezeLease 是否允许本次 failUnfreeze/settle 调用。
// 调用方须在调用前读取最新 Lease，确保读写在同一事务内。

   func ValidateLeaseForClaim(lease *FreezeLease, req ClaimTerminalReq, now time.Time) error {
	  if lease == nil {
		  return ErrLeaseNotFound
	   }
	  if lease.ID != req.LeaseID {
		  return ErrLeaseConflict
	   }
	  if lease.Token != req.LeaseToken {
		   return fmt.Errorf("%w: token mismatch for order %s", ErrLeaseConflict, req.OrderID)
	  }
	  if lease.Status.IsTerminal() {
		  // 终态 Lease 再次被声明——视为双释事故。
		   return ErrDoubleRelease
	   }
	  if !lease.ExpiresAt.IsZero() && now.After(lease.ExpiresAt) {
		  return ErrLeaseExpired
	   }
	   return nil
   }
