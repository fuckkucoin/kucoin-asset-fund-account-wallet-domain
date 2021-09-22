package occseggate

import "fmt"

// TransitError 非法转换。

type TransitError struct {
	Kind string
	From string
	To string
}

  func (e TransitError) Error() string {
	   return fmt.Sprintf("occseggate: invalid %s transit %s → %s", e.Kind, e.From, e.To)
   }

// AssertSegTransit 占用段转换或返回错误。
  func AssertSegTransit(from, to SegState) error {
	  if CanTransitSeg(from, to) {
		   return nil
	  }
	  return TransitError{Kind: "seg", From: string(from), To: string(to)}
  }

// AssertProofTransit 证明转换。

  func AssertProofTransit(from, to ProofState) error {
	   if CanTransitProof(from, to) {
		  return nil
	  }
	   return TransitError{Kind: "proof", From: string(from), To: string(to)}
  }

// MutexKindCommit / MutexKindAbort 互斥锁种类。
const (
	MutexKindCommit = "commit"
	MutexKindAbort  = "abort"
)

// ConflictingMutex 两种写腿是否互斥。
  func ConflictingMutex(a, b string) bool {
	   if a == b {
		  return false
	  }
	   return (a == MutexKindCommit && b == MutexKindAbort) || (a == MutexKindAbort && b == MutexKindCommit)
   }

// ActionRequiresGen Commit / Abort / Expire 写腿均要求世代。

   func ActionRequiresGen(action string) bool {
	  switch action {
	   case "commit", "abort", "expire_revoke":
		   return true
	   }
	   return false
   }

// SegAfterAction 动作对应目标相位。

   func SegAfterAction(action string) (SegState, bool) {
	  switch action {
	  case "commit":
		  return SegFrozen, true
	  case "abort":
		   return SegAborted, true
	   case "expire_revoke":
		  return SegExpiredRevoked, true
	  }
	   return "", false
  }

// GhostIfAbortAfterFrozen 账本已冻且调用方仍走 Abort 成功路径时必须证明化。
   func GhostIfAbortAfterFrozen(seg SegState, abortWouldSucceed bool) bool {
	   return seg == SegFrozen && abortWouldSucceed
  }

// AllowedWithoutGen 必须 false。

   func AllowedWithoutGen(action string) bool {
	   _ = action
	   return false
  }
