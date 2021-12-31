package workerloop

import (
	"context"
	"fmt"
	"time"
	"github.com/rs/zerolog"
)

// Job 一次工作单元。
type Job func(ctx context.Context) error

// Config 循环参数。

type Config struct {
	Name       string
	Interval   time.Duration
	MaxBackoff time.Duration
	FailThresh int
	CircuitPause time.Duration
}

// Run 阻塞运行直到 ctx 取消。
  func Run(ctx context.Context, cfg Config, log zerolog.Logger, job Job) error {
	  if job == nil {
		  return fmt.Errorf("worker job required")
	   }
	   if cfg.Name == "" {
		   cfg.Name = "worker"
	  }
	  if cfg.Interval <= 0 {
		   cfg.Interval = time.Second
	  }
	   if cfg.MaxBackoff <= 0 {
		  cfg.MaxBackoff = 30 * time.Second
	  }
	   if cfg.FailThresh <= 0 {
		   cfg.FailThresh = 8
	  }
	   if cfg.CircuitPause <= 0 {
		   cfg.CircuitPause = 15 * time.Second
	   }
	  fails := 0
	  backoff := cfg.Interval
	  t := time.NewTimer(cfg.Interval)
	   defer t.Stop()
	   for {
		  select {
		  case <-ctx.Done():
			  return ctx.Err()
		  case <-t.C:
			  err := safe(job, ctx)
			   if err != nil {
				  fails++
				  log.Error().Err(err).Str("worker", cfg.Name).Int("fails", fails).Msg("tick")
				  if fails >= cfg.FailThresh {
					   log.Error().Str("worker", cfg.Name).Dur("pause", cfg.CircuitPause).Msg("circuit open")
					   if !sleep(ctx, cfg.CircuitPause) {
						  return ctx.Err()
					   }
					   fails = 0
					  backoff = cfg.Interval
				  } else {
					   backoff *= 2
					   if backoff > cfg.MaxBackoff {
						   backoff = cfg.MaxBackoff
					  }
					  if !sleep(ctx, backoff) {
						   return ctx.Err()
					  }
				   }
			   } else {
				   fails = 0
				   backoff = cfg.Interval
			   }
			   t.Reset(cfg.Interval)
		   }
	  }
  }

  func safe(job Job, ctx context.Context) (err error) {
	   defer func() {
		   if rec := recover(); rec != nil {
			  err = fmt.Errorf("worker panic: %v", rec)
		   }
	  }()
	   return job(ctx)
   }

  func sleep(ctx context.Context, d time.Duration) bool {
	   t := time.NewTimer(d)
	  defer t.Stop()
	  select {
	  case <-ctx.Done():
		  return false
	  case <-t.C:
		   return true
	   }
  }
