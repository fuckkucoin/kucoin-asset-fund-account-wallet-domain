package screenleasegate

import (
	"context"
	"time"
)

type GenerationEngine struct {
	store  Store
	secret string
	clock func() time.Time
}

   func newGenerationEngine(store Store, secret string, clock func() time.Time) *GenerationEngine {
	   if clock == nil {
		  clock = func() time.Time { return time.Now().UTC() }
	}
	return &GenerationEngine{store: store, secret: secret, clock: clock}
}

  func (e *GenerationEngine) Current(ctx context.Context) (ScreenGeneration, error) {
	  if e == nil || e.store == nil {
		  return ScreenGeneration{}, ErrUnavailable
	   }
	  return e.store.GetGeneration(ctx)
   }

   func (e *GenerationEngine) BumpHalt(ctx context.Context, on bool) (ScreenGeneration, error) {
	  cur, err := e.store.GetGeneration(ctx)
	  if err != nil {
		   return ScreenGeneration{}, err
	   }
	   got,
		   err := e.store.CASBumpHalt(ctx, cur.HaltGen, on, e.clock())
	   if err != nil {
		  return got, err
	  }
	   return got, nil
  }

  func (e *GenerationEngine) BumpPack(ctx context.Context, pack, mode string) (ScreenGeneration, error) {
	   pack = normalizePack(pack)
	   mode = normalizeMode(mode)
	   fp := packFingerprint(e.secret, pack, mode, "riskgate-engine")
	   cur, err := e.store.GetGeneration(ctx)
	  if err != nil {
		  return ScreenGeneration{}, err
	  }
	  return e.store.CASBumpPack(ctx, cur.PackGen, pack, mode, fp, e.clock())
  }

// MatchLease 复验租约冻结的世代与当前世代是否一致。

  func MatchLease(lease ScreenLease, gen ScreenGeneration) error {
	   if gen.HaltOn {
		   return ErrHaltFlip
	   }
	  if lease.HaltGen != gen.HaltGen {
		   return ErrHaltFlip
	  }
	  if lease.PackGen != gen.PackGen {
		   return ErrPackFlip
	   }
	  if lease.Pack != gen.Pack && gen.Pack != "" && lease.Pack != "" {
		   // Pack 字段可能仅在翻转后写入当前包名；世代不一致已先判。
		   if lease.PackFP != gen.PackFP && gen.PackFP != "" {
			   return ErrPackFlip
		  }
	   }
	  if lease.PackFP != "" && gen.PackFP != "" && lease.PackFP != gen.PackFP {
		  return ErrPackFlip
	  }
	  if lease.Mode != "" && gen.Mode != "" && lease.Mode != gen.Mode {
		  return ErrModeFlip
	  }
	  return nil
  }
