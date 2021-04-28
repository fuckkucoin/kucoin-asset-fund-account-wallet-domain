package exjob

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/workerloop"
	"github.com/rs/zerolog"
)

// ExpireLoopConfig 过期回收循环参数。

type ExpireLoopConfig struct {
	Interval   time.Duration
	JobTTL     time.Duration
	BodyRetain time.Duration
}

// DefaultExpireLoopConfig 默认每分钟扫一次，TTL/保留一致 ExpireAndReclaim。
   func DefaultExpireLoopConfig() ExpireLoopConfig {
	  return ExpireLoopConfig{
		   Interval:   time.Minute,
		  JobTTL:     defaultJobTTL,
		   BodyRetain: defaultBodyRetain,
	  }
   }

// BindWorker 记录循环参数，供 RunExpireLoop / StartExpireLoop 使用。
  func (b *Book) BindWorker(cfg ExpireLoopConfig) *Book {
	  if b == nil {
		  return nil
	  }
	  b.mu.Lock()
	  b.expireCfg = cfg
	   b.expireBound = true
	   b.mu.Unlock()
	  return b
   }

// StartExpireLoop 非阻塞启动过期回收循环；ctx 取消后退出。

  func (b *Book) StartExpireLoop(ctx context.Context, log zerolog.Logger, cfg ExpireLoopConfig) {
	   if b == nil {
		  return
	  }
	   if cfg.Interval <= 0 {
		  cfg.Interval = time.Minute
	  }
	  if cfg.JobTTL <= 0 {
		  cfg.JobTTL = defaultJobTTL
	  }
	  if cfg.BodyRetain <= 0 {
		   cfg.BodyRetain = defaultBodyRetain
	   }
	  wcfg := workerloop.Config{
		  Name: "export-expire",
		  Interval:     cfg.Interval,
		   MaxBackoff:   30 * time.Second,
		   FailThresh:   8,
		   CircuitPause: 15 * time.Second,
	  }
	   go func() {
		  _ = workerloop.Run(ctx, wcfg, log, func(context.Context) error {
			  _, _, err := b.ExpireAndReclaim(time.Now().UTC(), cfg.JobTTL, cfg.BodyRetain)
			  return err
		  })
	  }()
  }

// RunExpireLoop 使用 BindWorker 已绑定参数启动；未绑定则用默认。

   func (b *Book) RunExpireLoop(ctx context.Context, log zerolog.Logger) {
	  if b == nil {
		  return
	   }
	  b.mu.Lock()
	   cfg := b.expireCfg
	   bound := b.expireBound
	  b.mu.Unlock()
	  if !bound {
		   cfg = DefaultExpireLoopConfig()
	  }
	  b.StartExpireLoop(ctx, log, cfg)
   }
