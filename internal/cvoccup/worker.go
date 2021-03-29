package cvoccup

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// RollbackTriggerFn 触发回滚的回调函数（由 cvrollback 注入，避免循环依赖）。
type RollbackTriggerFn func(ctx context.Context, occID, orderID, userID, trigger, remain string) error

// IntegrityPatrol 完整性巡检 Worker。
// 遍历活跃路径，调用 CheckIntegrity；发现漂移则开 BreachEvent。

type IntegrityPatrol struct {
	svc      *Service
	interval time.Duration
}

// NewIntegrityPatrol 构造完整性巡检 Worker。

  func NewIntegrityPatrol(svc *Service, interval time.Duration) *IntegrityPatrol {
	   if interval <= 0 {
		   interval = time.Minute
	   }
	   return &IntegrityPatrol{svc: svc, interval: interval}
  }

// Run 启动巡检循环；ctx 取消时退出。

   func (w *IntegrityPatrol) Run(ctx context.Context) error {
	  ticker := time.NewTicker(w.interval)
	  defer ticker.Stop()
	   for {
		   select {
		  case <-ctx.Done():
			  return ctx.Err()
		   case <-ticker.C:
			   if err := w.runOnce(ctx); err != nil {
				  _ = err
			   }
		  }
	   }
  }

// RunOnce 执行单次巡检（/手动触发）。

   func (w *IntegrityPatrol) RunOnce(ctx context.Context) (int, error) {
	   return w.runOnceCount(ctx)
   }

   func (w *IntegrityPatrol) runOnce(ctx context.Context) error {
	   _, err := w.runOnceCount(ctx)
	   return err
   }

   func (w *IntegrityPatrol) runOnceCount(ctx context.Context) (int, error) {
	   if w.svc == nil {
		   return 0, nil
	  }
	   occs, err := w.svc.ListOccByUser(ctx, "", true, 200)
	  if err != nil {
		  return 0, err
	   }
	   n := 0
	   for _, occ := range occs {
		   if err := w.svc.CheckIntegrity(ctx, occ.ID, true); err != nil {
			  n++
		   }
	   }
	  return n, nil
  }

// ExpireRollbackWorker 过期回滚 Worker。
// 扫描已过期但非终态的路径，触发回滚。

type ExpireRollbackWorker struct {
	svc        *Service
	rollbackFn RollbackTriggerFn
	interval time.Duration
}

// NewExpireRollbackWorker 构造过期回滚 Worker。

   func NewExpireRollbackWorker(svc *Service, fn RollbackTriggerFn, interval time.Duration) *ExpireRollbackWorker {
	  if interval <= 0 {
		  interval = 2 * time.Minute
	  }
	   return &ExpireRollbackWorker{svc: svc, rollbackFn: fn, interval: interval}
  }

// Run 启动回滚循环。
   func (w *ExpireRollbackWorker) Run(ctx context.Context) error {
	   ticker := time.NewTicker(w.interval)
	  defer ticker.Stop()
	  for {
		  select {
		   case <-ctx.Done():
			   return ctx.Err()
		  case <-ticker.C:
			   _ = w.runOnce(ctx)
		  }
	  }
   }

// RunOnce 执行单次过期扫描（/手动触发）。
   func (w *ExpireRollbackWorker) RunOnce(ctx context.Context) (int, error) {
	   return w.runOnceCount(ctx)
  }

  func (w *ExpireRollbackWorker) runOnce(ctx context.Context) error {
	   _, err := w.runOnceCount(ctx)
	   return err
   }

   func (w *ExpireRollbackWorker) runOnceCount(ctx context.Context) (int, error) {
	   if w.svc == nil {
		  return 0, nil
	   }
	   now := time.Now().UTC()
	   occs, err := w.svc.ListExpiredOccs(ctx, now, 100)
	  if err != nil {
		   return 0, err
	   }
	  n := 0
	   for _, occ := range occs {
		   if w.rollbackFn != nil {
			  trigger := "EXPIRE"
			   remain := occ.RemainAmount
			  if strings.TrimSpace(remain) == "" {
				  remain = "0"
			  }
			  if err := w.rollbackFn(ctx, occ.ID, occ.OrderID, occ.UserID, trigger, remain); err != nil {
				   continue
			  }
		   } else {
			  if _, err := w.svc.Release(ctx, occ.ID, "EXPIRE"); err != nil {
				   continue
			  }
		   }
		   n++
	  }
	  return n, nil
   }

// BreachScanWorker 穿仓扫描 Worker。
// 周期性检查所有活跃用户的资产占用是否超出可用余额。
type BreachScanWorker struct {
	svc *Service
	interval time.Duration
}

// NewBreachScanWorker 构造穿仓扫描 Worker。

  func NewBreachScanWorker(svc *Service, interval time.Duration) *BreachScanWorker {
	  if interval <= 0 {
		   interval = 5 * time.Minute
	  }
	  return &BreachScanWorker{svc: svc,
		   interval: interval}
   }

// Run 启动穿仓扫描循环。

  func (w *BreachScanWorker) Run(ctx context.Context) error {
	  ticker := time.NewTicker(w.interval)
	  defer ticker.Stop()
	   for {
		   select {
		   case <-ctx.Done():
			  return ctx.Err()
		   case <-ticker.C:
			   _ = w.runOnce(ctx)
		   }
	   }
  }

// RunOnce 执行单次穿仓扫描。
   func (w *BreachScanWorker) RunOnce(ctx context.Context) (int, error) {
	  return w.runOnceCount(ctx)
   }

  func (w *BreachScanWorker) runOnce(ctx context.Context) error {
	   _, err := w.runOnceCount(ctx)
	   return err
  }

   func (w *BreachScanWorker) runOnceCount(ctx context.Context) (int, error) {
	   if w.svc == nil || w.svc.avail == nil {
		  return 0, nil
	  }
	  occs, err := w.svc.ListOccByUser(ctx, "", true, 500)
	   if err != nil {
		  return 0, err
	  }
	  type aggKey struct{ userID, asset string }
	  byKey := map[aggKey][]*PathOcc{}
	  for _, o := range occs {
		   k := aggKey{o.UserID,
			   NormalizeAsset(o.FromAsset)}
		  byKey[k] = append(byKey[k], o)
	   }
	   n := 0
	   for k, list := range byKey {
		  totalOcc := decimal.Zero
		   for _, o := range list {
			  ra, err := decimal.NewFromString(o.RemainAmount)
			   if err != nil {
				   continue
			   }
			   totalOcc = totalOcc.Add(ra)
		  }
		   avail, err := w.svc.avail.AvailBalance(ctx, k.userID, k.asset, "funding")
		   if err != nil {
			  continue
		   }
		   if totalOcc.GreaterThan(avail) {
			   evt := &BreachEvent{
				  ID: uuid.NewString(),
				  UserID: k.userID,
				   BreachKind: BreachAvailNeg,
				  Asset: k.asset,
				   OccAmount:   totalOcc.String(),
				  AvailBefore: avail.String(),
				   AvailAfter:  avail.Sub(totalOcc).String(),
				  Detail:      fmt.Sprintf("scan: total_occ=%s avail=%s paths=%d", totalOcc.String(), avail.String(), len(list)),
				   CreatedAt: time.Now().UTC(),
			  }
			   if err := w.svc.store.SaveBreach(ctx, evt); err == nil {
				   n++
				   w.svc.metrics.IncBreachOpened(BreachAvailNeg, k.asset)
			   }
		  }
	  }
	  return n, nil
   }
