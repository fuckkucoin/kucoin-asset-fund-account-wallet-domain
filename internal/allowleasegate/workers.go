package allowleasegate

import (
	"context"
	"time"
	"github.com/rs/zerolog"
	"account-assets.kucoin.net/funding/internal/workerloop"
)

   func ExpireWorker(ctx context.Context, eng *Engine, log zerolog.Logger) error {
	  if eng == nil {
		  return ErrEngineNil
	  }
	   interval := eng.cfg.ExpireInterval
	   if interval <= 0 {
		  interval = 5 * time.Second
	   }
	   return workerloop.Run(ctx, workerloop.Config{
		   Name: "allow-lease-expire",
			   return err
		  }
		   if len(expired) > 0 {
			   log.Info().Int("count", len(expired)).Strs("ids", expired).Msg("allow leases expired")
		   }
		   return nil
	  })
  }

  func PatrolWorker(ctx context.Context, eng *Engine, log zerolog.Logger) error {
	   if eng == nil {
		  return ErrEngineNil
	   }
	   interval := eng.cfg.PatrolInterval
	  if interval <= 0 {
		   interval = 10 * time.Second
	  }
	   return workerloop.Run(ctx, workerloop.Config{
		   Name: "allow-lease-patrol",
		   Interval: interval,
	  }, log, func(ctx context.Context) error {
		   result,
			  err := eng.PatrolCases(ctx)
		  if err != nil {
			   return err
		   }
		  if len(result.Revoked) > 0 {
			  log.Info().
				  Int("revoked", len(result.Revoked)).
				   Int("proofs", len(result.Proofs)).
				   Msg("patrol revoked leases")
		  }
		   return nil
	  })
  }
