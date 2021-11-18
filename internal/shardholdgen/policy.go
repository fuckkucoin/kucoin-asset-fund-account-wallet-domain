package shardholdgen

import (
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/wdshard"
)

const (
	DefaultTTLSeconds = 300
	MaxTTLSeconds = 1800
	MinTTLSeconds    = 30
	ExpireBatchSize  = 200
	PatrolBatchSize  = 100
	ProofExportBatch = 100
	MutexAdvance = "advance"
	MutexRelease = "release"
	MutexResume = "resume"
)

// NormalizeTTL 夹紧 TTL。
  func NormalizeTTL(sec int64) int64 {
	  if sec <= 0 {
		   return DefaultTTLSeconds
	  }
	  if sec < MinTTLSeconds {
		   return MinTTLSeconds
	  }
	  if sec > MaxTTLSeconds {
		  return MaxTTLSeconds
	  }
	   return sec
   }

// RequireGeneration fail-closed：无世代不得 Advance/Release。
   func RequireGeneration(g *LegHoldGen) error {
	   if g == nil || strings.TrimSpace(g.GenID) == "" || g.Generation <= 0 {
		  return ErrNoGeneration
	  }
	   return nil
   }

// MayAdvance 失败腿或待处理腿才允许 Advance。
   func MayAdvance(leg wdshard.Leg, g *LegHoldGen, now time.Time) error {
	   if err := RequireGeneration(g); err != nil {
		   return err
	   }
	   if !g.IsHoldable(now) && g.State != GenIssued {
		  if g.State == GenExpired {
			  return ErrGenExpired
		  }
		   if g.State == GenReleased {
			   return ErrGenReleased
		  }
	  }
	  switch leg.Status {
	  case wdshard.HoldLegPending, wdshard.HoldLegFailed, wdshard.HoldLegAdvancing:
		   return nil
	   case wdshard.HoldLegBroadcasting:
		   return ErrBroadcastLive
	  case wdshard.HoldLegFilled:
		  return ErrAdvanceDenied
	   case wdshard.HoldLegReleased:
		  return ErrGenReleased
	   default:
		  return ErrAdvanceDenied
	   }
  }

// MayRelease 释放闸：在播禁释；额不得超 hold。
  func MayRelease(leg wdshard.Leg, g *LegHoldGen, amount int64, now time.Time) error {
	   if err := RequireGeneration(g); err != nil {
		   return err
	   }
	   if amount <= 0 {
		  return ErrInvalidReq
	  }
	   if leg.Status == wdshard.HoldLegBroadcasting {
		  return ErrBroadcastLive
	   }
	  if g.State != GenHeld && g.State != GenIssued {
		   if g.State == GenReleased {
			  return ErrGenReleased
		   }
		  if g.State == GenExpired {
			   return ErrGenExpired
		   }
		   return ErrReleaseDenied
	   }
	   if now.After(g.ExpiresAt()) && g.State == GenIssued {
		   return ErrGenExpired
	   }
	   if amount > g.ResidualHold() && g.HoldAmt > 0 {
		   return ErrOverRelease
	   }
	  if g.HoldAmt == 0 && amount > leg.HoldAmt && leg.HoldAmt > 0 {
		   return ErrOverRelease
	  }
	  if g.HoldAmt == 0 && leg.HoldAmt == 0 && amount > leg.Amount {
		  return ErrOverRelease
	   }
	  return nil
  }

// ConflictOp 互斥冲突。
   func ConflictOp(owner, want string) bool {
	   if owner == "" {
		   return false
	  }
	   return owner != want
   }

  func actorOr(actor string) string {
	   actor = strings.TrimSpace(actor)
	   if actor == "" {
		  return "system"
	  }
	   return actor
  }
