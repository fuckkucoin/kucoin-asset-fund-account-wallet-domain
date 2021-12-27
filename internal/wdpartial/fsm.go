package wdpartial

import (
	"fmt"
	"time"
)

// ──────────────────────────────────────────────
// RemReleaseLease 状态机
// ──────────────────────────────────────────────
// TransitLeaseReleasing open→releasing。
// 调用前须已确认无活跃 FillMutexToken（由 Service 层保证）。
// 过期的租约不得迁移到 releasing。

   func TransitLeaseReleasing(l *RemReleaseLease, now time.Time) error {
	   if l == nil {
		  return fmt.Errorf("%w: nil lease", ErrInvalidStatus)
	   }
	   if l.Status != LeaseStatusOpen {
		  return fmt.Errorf("%w: lease %s is %s, expected open", ErrInvalidStatus, l.ID, l.Status)
	   }
	  if !l.ExpiresAt.IsZero() && now.After(l.ExpiresAt) {
		  return ErrLeaseExpired
	   }
	  l.Status = LeaseStatusReleasing
	   t := now
	  l.ReleasingAt = &t
	   l.UpdatedAt = now
	   return nil
  }

// TransitLeaseBlocked open 或 releasing→blocked。
// FillMutexToken 颁发后调用，记录阻塞来源 fillID。
   func TransitLeaseBlocked(l *RemReleaseLease, fillID string, now time.Time) error {
	  if l == nil {
		   return fmt.Errorf("%w: nil lease", ErrInvalidStatus)
	   }
	   switch l.Status {
	   case LeaseStatusOpen, LeaseStatusReleasing:
	  default:
		  return fmt.Errorf("%w: lease %s is %s, cannot block", ErrInvalidStatus, l.ID, l.Status)
	  }
	   l.Status = LeaseStatusBlocked
	   l.BlockedByFillID = fillID
	  l.UpdatedAt = now
	  return nil
   }

// TransitLeaseUnblock blocked→open（Fill 路径完成后释放阻塞）。
  func TransitLeaseUnblock(l *RemReleaseLease, now time.Time) error {
	   if l == nil {
		  return fmt.Errorf("%w: nil lease", ErrInvalidStatus)
	   }
	  if l.Status != LeaseStatusBlocked {
		   return fmt.Errorf("%w: lease %s is %s, expected blocked", ErrInvalidStatus, l.ID, l.Status)
	  }
	   l.Status = LeaseStatusOpen
	  l.BlockedByFillID = ""
	  l.UpdatedAt = now
	   return nil
   }

// TransitLeaseDone releasing→done。
// 释放流程完成后调用。
  func TransitLeaseDone(l *RemReleaseLease, now time.Time) error {
	   if l == nil {
		   return fmt.Errorf("%w: nil lease", ErrInvalidStatus)
	  }
	  if l.Status != LeaseStatusReleasing {
		  return fmt.Errorf("%w: lease %s is %s, expected releasing", ErrInvalidStatus, l.ID, l.Status)
	   }
	   l.Status = LeaseStatusDone
	  t := now
	   l.DoneAt = &t
	  l.UpdatedAt = now
	   return nil
  }

// TransitLeaseRevoked *（非终态）→revoked。
// 租约过期或管理员主动吊销时调用。
   func TransitLeaseRevoked(l *RemReleaseLease, now time.Time) error {
	   if l == nil {
		  return fmt.Errorf("%w: nil lease", ErrInvalidStatus)
	   }
	  if l.Status.IsTerminal() {
		   return fmt.Errorf("%w: lease %s already terminal (%s)", ErrInvalidStatus, l.ID, l.Status)
	   }
	   l.Status = LeaseStatusRevoked
	   t := now
	  l.RevokedAt = &t
	   l.UpdatedAt = now
	   return nil
   }

// CheckLeaseActive 检查租约是否活跃（open 且未过期）。

  func CheckLeaseActive(l *RemReleaseLease, now time.Time) error {
	   if l == nil {
		   return ErrLeaseNotFound
	   }
	   if l.Status != LeaseStatusOpen {
		   return fmt.Errorf("%w: status=%s", ErrLeaseNotOpen, l.Status)
	   }
	  if !l.ExpiresAt.IsZero() && now.After(l.ExpiresAt) {
		  return ErrLeaseExpired
	  }
	  return nil
  }

// DenyLeaseInventory 返回所有禁止边（回归用）。
   func DenyLeaseInventory() [][2]LeaseStatus {
	  return [][2]LeaseStatus{
		  {LeaseStatusDone, LeaseStatusOpen},
		   {LeaseStatusDone, LeaseStatusReleasing},
		   {LeaseStatusDone,
			  LeaseStatusBlocked},
		  {LeaseStatusDone, LeaseStatusRevoked},
		  {LeaseStatusRevoked, LeaseStatusOpen},
		   {LeaseStatusRevoked, LeaseStatusReleasing},
		   {LeaseStatusRevoked, LeaseStatusBlocked},
		   {LeaseStatusRevoked, LeaseStatusDone},
		   {LeaseStatusBlocked, LeaseStatusReleasing},
		  {LeaseStatusBlocked, LeaseStatusDone},
	   }
  }

// ──────────────────────────────────────────────
// ConserveCase 状态机
// ──────────────────────────────────────────────
// TransitConserveProven open→proven。
// 须提供非空 proof_payload 和双人 approver。

   func TransitConserveProven(c *ConserveCase, proof, approver, approver2 string, now time.Time) error {
	   if c == nil {
		   return fmt.Errorf("%w: nil conserve case", ErrInvalidStatus)
	  }
	   if c.Status != ConserveCaseOpen {
		   return fmt.Errorf("%w: conserve case %s is %s, expected open", ErrInvalidStatus, c.ID, c.Status)
	   }
	  if proof == "" {
		   return ErrConserveCaseProofRequired
	  }
	  c.Status = ConserveCaseProven
	   c.ProofPayload = proof
	  c.Approver = approver
	  c.Approver2 = approver2
	   t := now
	   c.ProvenAt = &t
	  c.UpdatedAt = now
	   return nil
   }

// TransitConserveClose proven→closed。
  func TransitConserveClose(c *ConserveCase, approver string, now time.Time) error {
	  if c == nil {
		   return fmt.Errorf("%w: nil conserve case", ErrInvalidStatus)
	   }
	   if c.Status == ConserveCaseClosed {
		  return ErrConserveCaseAlreadyClosed
	   }
	  if c.Status != ConserveCaseProven {
		  return fmt.Errorf("%w: conserve case %s is %s", ErrConserveCaseNotProven, c.ID, c.Status)
	  }
	   c.Status = ConserveCaseClosed
	  if approver != "" {
		   c.Approver = approver
	  }
	   t := now
	  c.ClosedAt = &t
	   c.UpdatedAt = now
	   return nil
   }

// ForceCloseConserveCase open→closed（双人强制收口，须提供 proof_payload）。

   func ForceCloseConserveCase(c *ConserveCase, proof, approver, approver2 string, now time.Time) error {
	   if c == nil {
		   return fmt.Errorf("%w: nil conserve case", ErrInvalidStatus)
	  }
	  if c.Status == ConserveCaseClosed {
		  return ErrConserveCaseAlreadyClosed
	  }
	   if proof == "" {
		   return ErrConserveCaseProofRequired
	   }
	   if approver == "" || approver2 == "" {
		   return ErrForceReleaseDenied
	  }
	  c.Status = ConserveCaseClosed
	  c.ProofPayload = proof
	   c.Approver = approver
	  c.Approver2 = approver2
	  t := now
	  c.ClosedAt = &t
	   c.UpdatedAt = now
	   return nil
  }

// DenyConserveInventory 禁止边列表（回归用）。

  func DenyConserveInventory() [][2]ConserveCaseStatus {
	  return [][2]ConserveCaseStatus{
		   {ConserveCaseClosed,
			   ConserveCaseOpen},
		   {ConserveCaseClosed, ConserveCaseProven},
		  {ConserveCaseOpen, ConserveCaseClosed},
		  {ConserveCaseProven, ConserveCaseOpen},
	   }
  }

// ──────────────────────────────────────────────
// BroadcastGuardGen 状态机
// ──────────────────────────────────────────────
// TransitGuardInvalidate active→invalidated。
// live-hash 变更或广播 FSM 迁移时调用；须提供失效原因。

  func TransitGuardInvalidate(g *BroadcastGuardGen, reason string, now time.Time) error {
	  if g == nil {
		  return fmt.Errorf("%w: nil guard gen", ErrInvalidStatus)
	  }
	   if g.Status != GuardGenActive {
		  return fmt.Errorf("%w: guard_gen %s is %s, expected active", ErrInvalidStatus, g.ID, g.Status)
	   }
	   if reason == "" {
		  reason = "live-hash invalidated"
	  }
	  g.Status = GuardGenInvalidated
	  g.InvalidatedReason = reason
	   t := now
	  g.InvalidatedAt = &t
	  g.UpdatedAt = now
	  return nil
   }

// TransitGuardConsume active→consumed。
// skip 腿成功完成释放时调用。
   func TransitGuardConsume(g *BroadcastGuardGen, now time.Time) error {
	   if g == nil {
		   return fmt.Errorf("%w: nil guard gen", ErrInvalidStatus)
	  }
	  if g.Status == GuardGenConsumed {
		  return ErrGuardGenConsumed
	   }
	  if g.Status != GuardGenActive {
		  return fmt.Errorf("%w: guard_gen %s is %s, cannot consume", ErrInvalidStatus, g.ID, g.Status)
	   }
	   g.Status = GuardGenConsumed
	   t := now
	   g.ConsumedAt = &t
	   g.UpdatedAt = now
	   return nil
   }

// CheckGuardActive 检查守卫世代是否活跃。
// 失效（invalidated）时返回 ErrGuardGenInvalid，禁止释放 skip 腿。
   func CheckGuardActive(g *BroadcastGuardGen) error {
	  if g == nil {
		   return ErrGuardGenNotFound
	   }
	   switch g.Status {
	   case GuardGenActive:
		  return nil
	  case GuardGenInvalidated:
		   return ErrGuardGenInvalid
	   case GuardGenConsumed:
		  return ErrGuardGenConsumed
	  default:
		   return fmt.Errorf("%w: unknown status %s", ErrInvalidStatus, g.Status)
	  }
  }

// DenyGuardInventory 禁止边列表。

   func DenyGuardInventory() [][2]GuardGenStatus {
	   return [][2]GuardGenStatus{
		  {GuardGenInvalidated, GuardGenActive},
		   {GuardGenInvalidated, GuardGenConsumed},
		   {GuardGenConsumed,
			   GuardGenActive},
		  {GuardGenConsumed, GuardGenInvalidated},
	  }
  }

// ──────────────────────────────────────────────
// HoldRecord / FillRecord 迁移
// ──────────────────────────────────────────────
// TransitHoldRelease open→released。
// 释放前须已通过 RemReleaseLease 互斥检查。

   func TransitHoldRelease(h *HoldRecord, now time.Time) error {
	  if h == nil {
		   return fmt.Errorf("%w: nil hold", ErrInvalidStatus)
	  }
	  if h.Status == HoldStatusReleased {
		   return ErrHoldAlreadyReleased
	  }
	   if h.Status != HoldStatusOpen {
		   return fmt.Errorf("%w: hold %s is %s, cannot release", ErrInvalidStatus, h.ID, h.Status)
	   }
	   h.Status = HoldStatusReleased
	   t := now
	   h.ReleasedAt = &t
	   h.UpdatedAt = now
	   return nil
  }

// TransitHoldConsume open→consumed。
// 消费前须确认无 open ConserveCase（由 Service 层前置检查）。
  func TransitHoldConsume(h *HoldRecord, now time.Time) error {
	   if h == nil {
		   return fmt.Errorf("%w: nil hold", ErrInvalidStatus)
	   }
	   if h.Status == HoldStatusConsumed {
		   return fmt.Errorf("%w: hold %s already consumed", ErrHoldNotOpen, h.ID)
	   }
	  if h.Status != HoldStatusOpen {
		   return fmt.Errorf("%w: hold %s is %s, cannot consume", ErrInvalidStatus, h.ID, h.Status)
	   }
	   h.Status = HoldStatusConsumed
	   t := now
	  h.ConsumedAt = &t
	  h.UpdatedAt = now
	  return nil
  }

// TransitFillConfirm pending→confirmed。
   func TransitFillConfirm(f *FillRecord, tokenID string, now time.Time) error {
	  if f == nil {
		   return fmt.Errorf("%w: nil fill", ErrInvalidStatus)
	  }
	  if f.Status == FillStatusConfirmed {
		  return ErrFillAlreadyConfirmed
	  }
	   if f.Status != FillStatusPending {
		   return fmt.Errorf("%w: fill %s is %s, cannot confirm", ErrInvalidStatus, f.ID, f.Status)
	   }
	  f.Status = FillStatusConfirmed
	   f.MutexTokenID = tokenID
	  t := now
	   f.ConfirmedAt = &t
	   f.UpdatedAt = now
	  return nil
   }

// TransitFillReject pending→rejected。

   func TransitFillReject(f *FillRecord, now time.Time) error {
	  if f == nil {
		  return fmt.Errorf("%w: nil fill", ErrInvalidStatus)
	  }
	  if f.Status.IsTerminal() {
		   return fmt.Errorf("%w: fill %s already terminal (%s)", ErrInvalidStatus, f.ID, f.Status)
	   }
	   f.Status = FillStatusRejected
	  t := now
	  f.RejectedAt = &t
	  f.UpdatedAt = now
	   return nil
  }
