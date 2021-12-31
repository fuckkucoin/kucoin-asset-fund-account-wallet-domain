package workerloop

import (
	"context"
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/internal/slo"
	"github.com/rs/zerolog"
)

// ObserveConfig 可观测循环参数。
type ObserveConfig struct {
	Config
	Stats *Stats
	OnCircuit func(name string, fails int)
	OnTickOK   func(name string, d time.Duration)
	OnTickFail func(name string, err error)
}

// RunObserved 带统计的阻塞循环。
  func RunObserved(ctx context.Context, cfg ObserveConfig, log zerolog.Logger, job Job) error {
	  if job == nil {
		   return fmt.Errorf("worker job required")
	  }
	  inner := job
	  job = func(ctx context.Context) error {
		   start := time.Now()
		   err := safe(inner, ctx)
		  if cfg.Stats != nil {
			   cfg.Stats.Observe(err)
		  }
		  if err != nil {
			  if cfg.OnTickFail != nil {
				  cfg.OnTickFail(cfg.Name, err)
			  }
			   return err
		   }
		   if cfg.OnTickOK != nil {
			  cfg.OnTickOK(cfg.Name, time.Since(start))
		  }
		  return nil
	  }
	  base := cfg.Config
	  if base.Name == "" {
		   base.Name = "worker"
	   }
	   if base.Interval <= 0 {
		  base.Interval = time.Second
	   }
	   if base.MaxBackoff <= 0 {
		   base.MaxBackoff = 30 * time.Second
	  }
	  if base.FailThresh <= 0 {
		   base.FailThresh = 8
	  }
	  if base.CircuitPause <= 0 {
		  base.CircuitPause = 15 * time.Second
	  }
	   fails := 0
	  backoff := base.Interval
	  t := time.NewTimer(base.Interval)
	  defer t.Stop()
	  for {
		  select {
		   case <-ctx.Done():
			   return ctx.Err()
		  case <-t.C:
			   err := safe(job, ctx)
			   if err != nil {
				   fails++
				  log.Error().Err(err).Str("worker", base.Name).Int("fails", fails).Msg("tick")
				  if fails >= base.FailThresh {
					   log.Error().Str("worker", base.Name).Dur("pause", base.CircuitPause).Msg("circuit open")
					   slo.FireMiddlewareAlert("worker.circuit", float64(fails), float64(base.FailThresh))
					   if cfg.OnCircuit != nil {
						   cfg.OnCircuit(base.Name, fails)
					  }
					   if !sleep(ctx, base.CircuitPause) {
						  return ctx.Err()
					   }
					  fails = 0
					   backoff = base.Interval
				  } else {
					   backoff *= 2
					   if backoff > base.MaxBackoff {
						  backoff = base.MaxBackoff
					   }
					   if !sleep(ctx, backoff) {
						  return ctx.Err()
					   }
				  }
			  } else {
				   fails = 0
				   backoff = base.Interval
			  }
			  t.Reset(base.Interval)
		   }
	  }
  }
