package screenleasegate

import (
	"context"
	"sync"
	"time"
)

type Worker struct {
	svc      *Service
	interval time.Duration
	stop chan struct{}
	once sync.Once
	ticks int64
	lastN int
}

   func NewWorker(svc *Service, interval time.Duration) *Worker {
	  if interval <= 0 {
		   interval = 15 * time.Second
	  }
	   return &Worker{svc: svc, interval: interval, stop: make(chan struct{})}
   }

   func (w *Worker) Start() {
	  if w == nil || w.svc == nil {
		  return
	  }
	   go w.loop()
  }

   func (w *Worker) Stop() {
	  if w == nil {
		  return
	  }
	   w.once.Do(func() { close(w.stop) })
   }

  func (w *Worker) loop() {
	   t := time.NewTicker(w.interval)
	  defer t.Stop()
	  for {
		  select {
		   case <-w.stop:
			  return
		  case <-t.C:
			   _, _ = w.Tick(context.Background())
		   }
	   }
  }

  func (w *Worker) Tick(ctx context.Context) (int, error) {
	   if w == nil || w.svc == nil {
		   return 0, ErrUnavailable
	  }
	   n, err := w.svc.ExpireSweep(ctx)
	  w.ticks++
	  w.lastN = n
	   if n > 0 {
		   list, _ := w.svc.ListOpenLeases(ctx)
		  _ = list
	  }
	   return n, err
   }

  func (w *Worker) Stats() (ticks int64, last int) {
	  if w == nil {
		  return 0, 0
	   }
	   return w.ticks, w.lastN
  }

// RevokeIfGenerationDrift 供外部在观察到 riskgate Halt/Engine 变化时调用。
// 本函数不回写 riskgate，只按本包世代吊销残留租约。
   func (w *Worker) RevokeIfGenerationDrift(ctx context.Context, haltOn bool, pack, mode string) error {
	  if w == nil || w.svc == nil {
		  return ErrUnavailable
	  }
	  cur, err := w.svc.Generation(ctx)
	  if err != nil {
		  return err
	  }
	   if haltOn != cur.HaltOn {
		  _, _, err = w.svc.SetHalt(ctx, SetHaltReq{On: haltOn, Operator: "worker-scan"})
		  return err
	  }
	  if pack != "" && (pack != cur.Pack || (mode != "" && mode != cur.Mode)) {
		  _, _, err = w.svc.FlipPack(ctx, FlipPackReq{Pack: pack, Mode: mode, Operator: "worker-scan"})
		   return err
	   }
	  return nil
  }
