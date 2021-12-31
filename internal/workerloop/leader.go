package workerloop

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/redisx"
	"account-assets.kucoin.net/funding/internal/slo"
	"github.com/rs/zerolog"
)

// Leader 领导权句柄。
type Leader struct {
	Client redisx.Client
	Key    string
	TTL    time.Duration
	Every time.Duration
	token string
}

// Claim 阻塞直到拿到锁或 ctx 取消。
  func (l *Leader) Claim(ctx context.Context, log zerolog.Logger) bool {
	   if l == nil || l.Client == nil {
		   return true // 无 Redis 时单实例直通
	  }
	   if l.Key == "" {
		   l.Key = redisx.KeyWorkerLeader
	   }
	   if l.TTL <= 0 {
		  l.TTL = slo.WorkerLeaderTTL
	  }
	  if l.Every <= 0 {
		  l.Every = slo.WorkerLeaderRefresh
	   }
	  for {
		  tok, err := l.Client.TryLock(ctx, l.Key, l.TTL)
		   if err == nil && tok != "" {
			   l.token = tok
			   go l.refresh(ctx, log)
			  return true
		   }
		  select {
		  case <-ctx.Done():
			   return false
		  case <-time.After(2 * time.Second):
		  }
	  }
   }

  func (l *Leader) refresh(ctx context.Context, log zerolog.Logger) {
	   t := time.NewTicker(l.Every)
	   defer t.Stop()
	  for {
		  select {
		  case <-ctx.Done():
			  _ = l.Client.Unlock(context.Background(), l.Key, l.token)
			  return
		  case <-t.C:
			  // 续期：重新 Set 同 token；若被抢则退出
			  if err := l.Client.Set(ctx, l.Key, []byte(l.token), l.TTL); err != nil {
				  log.Warn().Err(err).Str("key", l.Key).Msg("worker leader refresh")
			   }
		   }
	   }
   }

// RunWithLeader 先争抢领导权再进入循环。

   func RunWithLeader(ctx context.Context, lead *Leader, cfg Config, log zerolog.Logger, job Job) error {
	  if lead != nil && !lead.Claim(ctx, log) {
		  return ctx.Err()
	  }
	  return Run(ctx, cfg, log, job)
  }
