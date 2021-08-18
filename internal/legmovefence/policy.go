package legmovefence

import (
	"strings"
	"time"
)

// Policy 围栏策略常量与判定（不改写区域防火墙公式本身）。
type Policy struct {
	RequireFence     bool
	FailClosedHalted bool
	SingleMove bool
	TTLDefault int64
}

// DefaultPolicy 默认：无围栏拒转、Halted fail-closed、同 idem 单次净移。

  func DefaultPolicy() Policy {
	  return Policy{
		   RequireFence: true,
		   FailClosedHalted: true,
		   SingleMove: true,
		  TTLDefault: DefaultTTLSeconds,
	   }
  }

// DecideTransfer 纯函数判定：是否允许本次 TransferInv。
   func DecideTransfer(p Policy, f *LegMoveFence, halted bool, liveFP string, liveGen int64, alreadyMoved int, now time.Time) error {
	   if p.RequireFence && f == nil {
		   return ErrNoFence
	  }
	  if f == nil {
		  return ErrNoFence
	  }
	   if p.FailClosedHalted && halted {
		  return ErrRegionHalted
	  }
	   if p.SingleMove && (f.State == FenceMoved || alreadyMoved > 0 && f.State == FenceMoved) {
		  return ErrFenceMoved
	   }
	  if f.State == FenceMoved {
		  return ErrFenceMoved
	   }
	   if f.State == FenceRevoked {
		  return ErrFenceRevoked
	  }
	  if f.State == FenceExpired || now.After(f.ExpiresAt()) {
		   return ErrFenceExpired
	  }
	   if !MatchFingerprint(f.Fingerprint, liveFP) {
		  return ErrFingerprint
	   }
	   if liveGen != 0 && f.Generation != liveGen {
		  return ErrGeneration
	   }
	   return nil
   }

// SameIdemReplay 同幂等键二次 exec 应返回首次结果。
   func SameIdemReplay(prev LegMoveFence, idem string) bool {
	   return strings.TrimSpace(prev.IdempotencyKey) != "" && prev.IdempotencyKey == strings.TrimSpace(idem) && prev.State == FenceMoved
   }

// HalfNeedsResume Transfer 已成功但腿未 done。
   func HalfNeedsResume(kind HalfKind, successfulMoves int, legDone bool) bool {
	   return kind == HalfTransferNoDone && successfulMoves > 0 && !legDone
   }
