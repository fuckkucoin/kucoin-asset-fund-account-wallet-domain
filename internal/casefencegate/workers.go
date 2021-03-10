package casefencegate

import (
	"context"
	"time"
	"github.com/rs/zerolog"
	"account-assets.kucoin.net/funding/internal/workerloop"
)

// ExpireWorker 过期案件扫描 + 围栏 bump worker。

  func ExpireWorker(ctx context.Context, eng *Engine, log zerolog.Logger) error {
	  if eng == nil {
		   return ErrEngineNil
	  }
	   interval := eng.cfg.ExpireInterval
	  if interval <= 0 {
		   interval = 5 * time.Second
	   }
	  return workerloop.Run(ctx, workerloop.Config{
		   Name: "case-fence-expire",
		  Interval: interval,
	  },
		  log, func(ctx context.Context) error {
			   result,
				   err := eng.ExpireStaleTickets(ctx)
			   if err != nil {
				  return err
			  }
			  if len(result.Expired) > 0 || len(result.Bumped) > 0 {
				  log.Info().
					  Int("expired_tickets", len(result.Expired)).
					   Int("bumped_cases", len(result.Bumped)).
					  Msg("case fence expire patrol")
			  }
			   return nil
		   })
   }

// BareDecidePatrolWorker 裸 Decide 检测 worker。

   func BareDecidePatrolWorker(ctx context.Context, eng *Engine, log zerolog.Logger) error {
	   if eng == nil {
		   return ErrEngineNil
	   }
	   interval := eng.cfg.PatrolInterval
	  if interval <= 0 {
		   interval = 10 * time.Second
	  }
	  return workerloop.Run(ctx, workerloop.Config{
		   Name:     "case-fence-bare-patrol",
		   Interval: interval,
	  }, log, func(ctx context.Context) error {
		  alerts, err := eng.ExportBareAlerts(ctx, "", 50)
		  if err != nil {
			   return err
		  }
		   if len(alerts) > 0 {
			   log.Warn().
				  Int("count", len(alerts)).
				  Msg("bare decide alerts detected in patrol cycle")
		  }
		   return nil
	   })
   }
