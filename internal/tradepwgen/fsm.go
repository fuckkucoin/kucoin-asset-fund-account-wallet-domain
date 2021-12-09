package tradepwgen

import (
	"fmt"
	"time"
)

// TransitGen 将旧世代迁移到 superseded。
// 新世代创建时状态直接为 active，无需 FSM 守卫。

  func TransitGen(g *PwGeneration, now time.Time) error {
	  if g == nil {
		  return fmt.Errorf("%w: nil generation", ErrInvalidStatus)
	   }
	   if g.Status != GenStatusActive {
		  return fmt.Errorf("%w: gen %d already %s", ErrInvalidStatus, g.Generation, g.Status)
	   }
	   g.Status = GenStatusSuperseded
	   g.UpdatedAt = now
	   return nil
   }

// TransitLeaseConsume open→consumed。
// 出金请求提交时调用。

   func TransitLeaseConsume(l *VerifyLease, now time.Time) error {
	   if l == nil {
		  return fmt.Errorf("%w: nil lease", ErrInvalidStatus)
	   }
	  if l.Status == LeaseStatusConsumed {
		  return ErrLeaseConsumed
	  }
	  if l.Status != LeaseStatusOpen {
		   return fmt.Errorf("%w: lease %s already %s", ErrInvalidStatus, l.ID, l.Status)
	   }
	   l.Status = LeaseStatusConsumed
	  l.ConsumedAt = &now
	  l.UpdatedAt = now
	  return nil
   }

// TransitLeaseRevoke open→revoked。
// Change/Reset 密码时对用户全部 open 令牌调用。

  func TransitLeaseRevoke(l *VerifyLease, now time.Time) error {
	  if l == nil {
		   return fmt.Errorf("%w: nil lease", ErrInvalidStatus)
	  }
	   if l.Status == LeaseStatusRevoked {

// TransitDriftProve open→proven。
// 证明固化时调用；ProofPayload 非空才允许迁移。

   func TransitDriftProve(dc *PwDriftCase, proof, approver, approver2 string, now time.Time) error {
	   if dc == nil {
		  return fmt.Errorf("%w: nil drift case", ErrInvalidStatus)
	  }
	  if dc.Status != DriftStatusOpen {
		  return fmt.Errorf("%w: drift case %s is %s, cannot prove", ErrInvalidStatus, dc.ID, dc.Status)
	  }
	   if proof == "" {
		   return fmt.Errorf("%w: proof_payload required for proven transition", ErrInvalidStatus)
	  }
	   dc.Status = DriftStatusProven
	  dc.ProofPayload = proof
	   dc.Approver = approver
	  dc.Approver2 = approver2
	  dc.ProvenAt = &now
	   dc.UpdatedAt = now
	   return nil
  }

// TransitDriftClose proven→closed。
// 收口前须确认已处于 proven 状态。
  func TransitDriftClose(dc *PwDriftCase, approver string, now time.Time) error {
	  if dc == nil {
		   return fmt.Errorf("%w: nil drift case", ErrInvalidStatus)
	   }
	   if dc.Status == DriftStatusClosed {
		   return ErrDriftAlreadyClosed
	  }
	   if dc.Status != DriftStatusProven {
		  return fmt.Errorf("%w: drift case %s is %s, must be proven before close", ErrInvalidStatus, dc.ID, dc.Status)
	  }
	  dc.Status = DriftStatusClosed
	   if approver != "" {
		   dc.Approver = approver
	   }
	  dc.ClosedAt = &now
	  dc.UpdatedAt = now
	  return nil
  }

// ForceCloseDrift 强制收口（open→closed）。
// 仅 ops 权限接口可调用；须在 ProofPayload 填写强制原因。

   func ForceCloseDrift(dc *PwDriftCase, proof, approver string, now time.Time) error {
	  if dc == nil {
		  return fmt.Errorf("%w: nil drift case", ErrInvalidStatus)
	  }
	  if dc.Status == DriftStatusClosed {
		   return ErrDriftAlreadyClosed
	   }
	   if proof == "" {
		   return fmt.Errorf("%w: proof_payload required for force close", ErrInvalidStatus)
	  }
	   dc.Status = DriftStatusClosed
	  dc.ProofPayload = proof
	   dc.Approver = approver
	   dc.ClosedAt = &now
	  dc.UpdatedAt = now
	  return nil
   }

// CheckLeaseNotExpired 检查令牌是否超时。
// ExpiresAt 零值表示不过期（环境专用）。

  func CheckLeaseNotExpired(l *VerifyLease, now time.Time) error {
	   if l == nil {
		  return ErrLeaseNotFound
	   }
	   if !l.ExpiresAt.IsZero() && now.After(l.ExpiresAt) {
		   return ErrLeaseExpired
	  }
	  return nil
  }

// CheckLeaseGenMatch 核对令牌世代与当前世代；不符时返回 ErrLeaseGenMismatch。
   func CheckLeaseGenMatch(l *VerifyLease, currentGen int64) error {
	   if l == nil {
		  return ErrLeaseNotFound
	  }
	  if l.PwGen != currentGen {
		  return ErrLeaseGenMismatch
	   }
	  return nil
   }
