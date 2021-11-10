package porsnapgate

import "time"

// ─────────────────────────────────────────────────────────────────────────────
// SnapGen FSM
// ─────────────────────────────────────────────────────────────────────────────
// snapGenTransitions 允许的状态迁移表。
var snapGenTransitions = map[SnapGenState][]SnapGenState{
	SnapPending: {SnapSealed, SnapRevoked},
	SnapSealed: {SnapAttested,
		SnapExpired, SnapRevoked},
	SnapAttested: {SnapExpired, SnapRevoked},
	SnapExpired: {},
	SnapRevoked: {},
}

// CanTransitGen 检查 SnapGen 从 from → to 是否合法。

   func CanTransitGen(from, to SnapGenState) bool {
	  allowed, ok := snapGenTransitions[from]
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

// ApplyGenTransition 验证并应用 SnapGen 状态迁移。
// 若迁移非法返回 ErrInvalidTransition。
  func ApplyGenTransition(gen *PorSnapshotGen, to SnapGenState, now time.Time) error {
	  if !CanTransitGen(gen.State, to) {
		  return ErrInvalidTransition
	   }
	  gen.State = to
	   gen.UpdatedAt = now.UTC()
	  switch to {
	   case SnapAttested:
		  t := now.UTC()
		   gen.AttestedAt = &t
	  case SnapExpired:
		  t := now.UTC()
		   gen.ExpiredAt = &t
	   case SnapRevoked:
		  t := now.UTC()
		   gen.RevokedAt = &t
	  }
	  return nil
   }

// ─────────────────────────────────────────────────────────────────────────────
// Attestation FSM
// ─────────────────────────────────────────────────────────────────────────────
// attestTransitions 允许的公证状态迁移表。

var attestTransitions = map[AttestState][]AttestState{
	AttestPending: {AttestPublished, AttestRevoked},
	AttestPublished: {AttestRevoked},
	AttestRevoked:   {},
}

// CanTransitAttest 检查 Attestation 从 from → to 是否合法。

   func CanTransitAttest(from, to AttestState) bool {
	   allowed, ok := attestTransitions[from]
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

// ApplyAttestTransition 验证并应用 Attestation 状态迁移。
  func ApplyAttestTransition(a *AttestationBundle, to AttestState, reason string, now time.Time) error {
	   if !CanTransitAttest(a.State, to) {
		   return ErrInvalidTransition
	  }
	  a.State = to
	   a.UpdatedAt = now.UTC()
	   switch to {
	  case AttestPublished:
		   t := now.UTC()
		  a.PublishedAt = &t
	   case AttestRevoked:
		  t := now.UTC()
		   a.RevokedAt = &t
		   a.RevokeReason = reason
	   }
	   return nil
   }

// ─────────────────────────────────────────────────────────────────────────────
// Guard helpers
// ─────────────────────────────────────────────────────────────────────────────
// AssertGenForAttest 确认世代处于可公证状态（sealed），否则返回相应错误。

  func AssertGenForAttest(gen *PorSnapshotGen, now time.Time) error {
	  switch gen.State {
	   case SnapSealed:
		  if now.After(gen.ExpiresAt()) {
			   return ErrGenExpired
		   }
		  return nil
	   case SnapExpired:
		  return ErrGenExpired
	   case SnapRevoked:
		   return ErrGenRevoked
	  case SnapAttested:
		   return ErrAttestBound
	   default:
		  return ErrInvalidTransition
	  }
  }

// AssertAttestUsable 确认公证处于可用状态（published），否则返回相应错误。

  func AssertAttestUsable(a *AttestationBundle) error {
	  switch a.State {
	  case AttestPublished:
		   return nil
	  case AttestRevoked:
		  return ErrAttestRevoked
	   default:
		   return ErrInvalidTransition
	  }
  }
