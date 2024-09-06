package closelatecredit

import "fmt"

// TransitError 非法转换。

type TransitError struct {
	Kind string
	From string
	To   string
}

   func (e TransitError) Error() string {
	  return fmt.Sprintf("closelatecredit: invalid %s transit %s → %s", e.Kind, e.From, e.To)
  }

   func (e TransitError) Unwrap() error { return ErrInvalidTransit }

// AssertEpochTransit 世代转换。

   func AssertEpochTransit(from, to EpochState) error {
	  if CanTransitEpoch(from, to) {
		   return nil
	  }
	   return TransitError{Kind: "epoch", From: string(from), To: string(to)}
  }

// AssertLateTransit 迟贷证明转换。
  func AssertLateTransit(from, to LateState) error {
	  if CanTransitLate(from, to) {
		   return nil
	  }
	   return TransitError{Kind: "late", From: string(from), To: string(to)}
   }

// AssertHoldTransit 争议仓转换。

  func AssertHoldTransit(from, to HoldState) error {
	   if CanTransitHold(from, to) {
		  return nil
	  }
	   return TransitError{Kind: "hold", From: string(from), To: string(to)}
   }

// CloseRaisesEpoch 关户成功必须升世代。
   func CloseRaisesEpoch(beforeN uint64, afterN uint64) bool {
	  return afterN == beforeN+1 && afterN > 0
  }

// ReopenRaisesEpoch 重开必须升世代。

   func ReopenRaisesEpoch(beforeN uint64, afterN uint64) bool {
	   return afterN == beforeN+1 && afterN > 0
  }

// CreditRequiresEpoch 一切贷记路径必须携带世代指针。
   func CreditRequiresEpoch(claimed *uint64) bool {
	  return claimed != nil
   }

// AllowedWithoutEpoch 必须 false。

   func AllowedWithoutEpoch(_ CreditSource) bool {
	   return false
  }

// ForceLiveDenied 禁止把迟贷强制写入活账。

   func ForceLiveDenied(force bool) bool {
	   return force
  }

// DecisionForClosed 已关户：拒入或改挂争议仓。默认争议仓（可观测、可人工）。

  func DecisionForClosed(source CreditSource, preferReject bool) CreditDecision {
	   if preferReject {
		   return DecisionReject
	  }
	  if source.Valid() {
		   return DecisionDispute
	   }
	   return DecisionReject
  }

// DecisionForStale 重开后旧世代：拒入新活户并证明化。
   func DecisionForStale() CreditDecision {
	   return DecisionReject
  }

// DecisionForMissing 无世代：fail-closed 拒绝。

   func DecisionForMissing() CreditDecision {
	  return DecisionReject
   }

// LateStateForDecision 裁决对应证明初态。

  func LateStateForDecision(d CreditDecision) LateState {
	  switch d {
	  case DecisionDispute:
		  return LateDisputed
	  case DecisionAdmit:
		   return LateClosed
	  default:
		   return LateRejected
	   }
   }

// ReasonForClosed 关户后迟贷原因。
  func ReasonForClosed(d CreditDecision) LateReason {
	   if d == DecisionDispute {
		   return ReasonClosedDispute
	  }
	   return ReasonClosedRefuse
   }

// SameSourceConstraint 观察 / 出箱 / 法币回调必须走同一套 epoch 断言。
   func SameSourceConstraint(a, b CreditSource) bool {
	   return a.Valid() && b.Valid()
   }

// NextEpochStateOnClose 关户提交后的目标相位。
   func NextEpochStateOnClose(from EpochState) (EpochState, error) {
	   switch from {
	  case EpochOpen, EpochReopened:
		  return EpochClosing, nil
	   case EpochClosing:
		  return EpochClosed, nil
	   default:
		  return "", AssertEpochTransit(from, EpochClosed)
	   }
   }

// NextEpochStateOnReopen 重开目标相位。

   func NextEpochStateOnReopen(from EpochState) (EpochState, error) {
	  if from == EpochClosed {
		  return EpochReopened, nil
	  }
	   return "", AssertEpochTransit(from, EpochReopened)
   }

// HoldAfterPark 入仓后相位。
  func HoldAfterPark() HoldState { return HoldParked }

// ResolveTarget 人工处置目标。
  func ResolveTarget(reject, exportOnly bool) HoldState {
	  if exportOnly {
		   return HoldExported
	   }
	  if reject {
		   return HoldRejected
	  }
	   return HoldResolved
  }
