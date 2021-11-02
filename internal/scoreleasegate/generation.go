package scoreleasegate

import (
	"context"
	"time"
)

type GenerationEngine struct {
	store Store
	secret string
	clock func() time.Time
}

   func newGenerationEngine(store Store, secret string, clock func() time.Time) *GenerationEngine {
	  if clock == nil {
		   clock = func() time.Time { return time.Now().UTC() }
	}
	return &GenerationEngine{store: store, secret: secret, clock: clock}
}

   func (e *GenerationEngine) Current(ctx context.Context) (ScoreGeneration, error) {
	  if e == nil || e.store == nil {
		  return ScoreGeneration{}, ErrUnavailable
	   }
	   return e.store.GetGeneration(ctx)
  }

   func (e *GenerationEngine) BumpHalt(ctx context.Context, on bool) (ScoreGeneration, error) {
	  cur, err := e.store.GetGeneration(ctx)
	  if err != nil {
		   return ScoreGeneration{}, err
	   }
	  return e.store.CASBumpHalt(ctx, cur.HaltGen, on, e.clock())
   }

   func (e *GenerationEngine) BumpPack(ctx context.Context, pack, mode string) (ScoreGeneration, error) {
	  pack = normalizePack(pack)
	  mode = normalizeMode(mode)
	   fp := packFingerprint(e.secret, pack, mode, "riskscore-engine")
	   cur, err := e.store.GetGeneration(ctx)
	  if err != nil {
		  return ScoreGeneration{}, err
	   }
	  return e.store.CASBumpPack(ctx, cur.PackGen, pack, fp, e.clock())
  }

  func (e *GenerationEngine) BumpMode(ctx context.Context, mode string) (ScoreGeneration, error) {
	  mode = normalizeMode(mode)
	  cur,
		  err := e.store.GetGeneration(ctx)
	   if err != nil {
		   return ScoreGeneration{}, err
	  }
	  return e.store.CASBumpMode(ctx, cur.ModeGen, mode, e.clock())
  }

   func (e *GenerationEngine) BumpWindow(ctx context.Context, digest string) (ScoreGeneration, error) {
	   cur, err := e.store.GetGeneration(ctx)
	  if err != nil {
		  return ScoreGeneration{}, err
	   }
	  return e.store.CASBumpWindow(ctx, cur.WindowGen, digest, e.clock())
  }

// MatchLease 复验租约冻结的世代与当前世代是否一致。
   func MatchLease(lease ScoreLease, gen ScoreGeneration) error {
	   if gen.HaltOn {
		   return ErrHaltFlip
	  }
	   if lease.HaltGen != gen.HaltGen {
		   return ErrHaltFlip
	   }
	  if lease.PackGen != gen.PackGen {
		   return ErrPackFlip
	  }
	  if lease.ModeGen != gen.ModeGen {
		   return ErrModeFlip
	  }
	  if lease.WindowGen != gen.WindowGen {
		   return ErrWindowDrift
	   }
	   if lease.PackFP != "" && gen.PackFP != "" && lease.PackFP != gen.PackFP {
		   return ErrPackFlip
	   }
	   if lease.Pack != "" && gen.Pack != "" && lease.Pack != gen.Pack {
		   if lease.PackFP != gen.PackFP {
			  return ErrPackFlip
		  }
	  }
	  if lease.Mode != "" && gen.Mode != "" && lease.Mode != gen.Mode {
		  return ErrModeFlip
	   }
	  if lease.WindowDigest != "" && gen.WindowDigest != "" && lease.WindowDigest != gen.WindowDigest {
		   return ErrWindowDrift
	  }
	  return nil
   }

  func kindFromMatchErr(err error) ProofKind {
	  switch err {
	   case ErrHaltFlip:
		   return ProofHaltFlip
	  case ErrPackFlip:
		   return ProofPackFlip
	  case ErrModeFlip:
		   return ProofModeFlip
	   case ErrWindowDrift:
		  return ProofWindowDrift
	   case ErrFingerprintDrift:
		   return ProofFingerprint
	  default:
		   return ProofStaleGen
	  }
   }
