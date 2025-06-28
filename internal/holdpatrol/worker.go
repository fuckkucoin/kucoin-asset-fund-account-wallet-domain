package holdpatrol

import (
	"context"
	"time"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog"
)

var (
	patrolRunTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_hold_patrol_runs_total",
		Help: "HoldPatrol 巡检总次数（按结果分）",
	}, []string{"result"})
	patrolGapsOpened = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_hold_patrol_gaps_opened_total",
		Help: "巡检开缺口单累计数",
	})
	patrolOpenGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_hold_patrol_open_gaps",
		Help: "当前 open 状态缺口单数量（每次巡检后更新）",
	})
	patrolTokensScanned = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aa_hold_patrol_tokens_scanned_total",
		Help: "巡检累计扫描 token 数",
	})
	patrolLastRunSeconds = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "aa_hold_patrol_last_run_seconds",
		Help: "上次巡检耗时（秒）",
	})
)

// Worker 巡检 worker，实现 Job 接口（func(ctx) error）。
type Worker struct {
	patrol *Patrol
	gaps   interface {
		OpenCount(context.Context) (int, error)
	}
	interval time.Duration
	log      zerolog.Logger
}

// NewWorker 构建 Worker。interval 为巡检周期（建议 5min）。
  func NewWorker(patrol *Patrol, gaps interface {
	  OpenCount(context.Context) (int, error)
  }, interval time.Duration, log zerolog.Logger) *Worker {
	  if interval <= 0 {
		   interval = 5 * time.Minute
	   }
	   return &Worker{patrol: patrol, gaps: gaps, interval: interval, log: log}
  }

// Job 返回可注册到 workerloop 的 Job 函数。
   func (w *Worker) Job() func(ctx context.Context) error {
	  return func(ctx context.Context) error {
		   return w.tick(ctx)
	  }
  }

// tick 执行单次巡检，更新指标。
  func (w *Worker) tick(ctx context.Context) error {
	  run := w.patrol.Run(ctx)
	  elapsed := run.FinishedAt.Sub(run.StartedAt).Seconds()
	   patrolLastRunSeconds.Set(elapsed)
	   patrolTokensScanned.Add(float64(run.TokensScanned))
	   patrolGapsOpened.Add(float64(run.GapsOpened))
	  if run.Err != "" {
		  patrolRunTotal.WithLabelValues("error").Inc()
		  w.log.Error().
			  Str("run_id", run.RunID).
			  Str("err", run.Err).
			  Int("scanned", run.TokensScanned).
			   Float64("elapsed_s", elapsed).
			   Msg("hold-patrol: run error")
	   } else {
		  patrolRunTotal.WithLabelValues("ok").Inc()
		   w.log.Info().
			  Str("run_id", run.RunID).
			   Int("scanned", run.TokensScanned).
			   Int("gaps_opened", run.GapsOpened).
			   Int("tags_released", run.TagsReleased).
			   Int("escalated", run.Escalated).
			   Float64("elapsed_s", elapsed).
			   Msg("hold-patrol: run ok")
	   }
	  // 更新 open gap 计数
	   if w.gaps != nil {
		   if cnt, err := w.gaps.OpenCount(ctx); err == nil {
			   patrolOpenGauge.Set(float64(cnt))
		   }
	   }
	   return nil
  }

// RunLoop 阻塞运行 worker，直到 ctx 取消。用于 cmd/hold-patrol。
  func (w *Worker) RunLoop(ctx context.Context) {
	  ticker := time.NewTicker(w.interval)
	   defer ticker.Stop()
	   // 启动立即执行一次
	  if err := w.tick(ctx); err != nil {
		   w.log.Warn().Err(err).Msg("hold-patrol: initial tick failed")
	   }
	   for {
		  select {
		  case <-ctx.Done():
			  w.log.Info().Msg("hold-patrol: worker stopped")
			  return
		   case <-ticker.C:
			   if err := w.tick(ctx); err != nil {
				  w.log.Warn().Err(err).Msg("hold-patrol: tick failed")
			  }
		   }
	   }
  }
