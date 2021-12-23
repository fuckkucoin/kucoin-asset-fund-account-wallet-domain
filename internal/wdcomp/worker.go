package wdcomp

import (
	"context"
	"errors"
	"sort"
	"time"
)

const workerBatch = 200

// Tick 跑一轮四个工人。now 为零时用编排器时钟。

   func (o *Orchestrator) Tick(ctx context.Context, now time.Time) (TickReport, error) {
	  if err := ctx.Err(); err != nil {
		  return TickReport{}, err
	  }
	  if o == nil {
		  return TickReport{}, ErrUnavailable
	   }
	  o.mu.Lock()
	  defer o.mu.Unlock()
	  if now.IsZero() {
		   now = o.now()
	   }
	   return o.tick(ctx, now)
  }

  func (o *Orchestrator) tick(ctx context.Context, now time.Time) (TickReport, error) {
	  var rep TickReport
	  if o.shardSrc != nil {
		  evs, err := o.shardSrc.Pull(ctx, workerBatch)
		  if err != nil {
			  return rep, err
		  }
		  for _, ev := range evs {
			   o.inbox = append(o.inbox, ev.Fact())
		   }
	   }
	  n, replay, err := o.drainInbox(ctx, workerBatch)
	  if err != nil {
		  return rep, err
	   }
	   rep.Ingested = n
	   rep.Replayed = replay
	   open := o.idsWhere(func(f *WdFailure) bool { return f.Status == FailOpen && f.PlanID == "" })
	   for _, id := range open {
		   view, err := o.build(ctx, id, now)
		  if err != nil {
			   return rep, err
		   }
		  rep.Planned++
		  if view.Plan.State == PlanNeedsHuman {
			   rep.Escalated++
			  rep.Denied++
		   }
	  }
	   planning := o.planIDs(PlanPlanning)
	  for _, id := range planning {
		   p := o.plans[id]
		   if p == nil || !p.AutoOK || p.LiveHash || p.ChainBlocked {
			  continue
		  }
		  view, err := o.execute(ctx, id, now)
		   if err != nil {
			   if errors.Is(err, ErrNeedsHuman) || errors.Is(err, ErrAutoUnfreezeDenied) {
				   rep.Denied++
				  continue
			  }
			   return rep, err
		   }
		   if view.Plan.State == PlanCommitted {
			   rep.Executed++
		  }
	   }
	   for _, id := range o.planIDs(PlanExecuting) {
		  view, err := o.execute(ctx, id, now)
		   if err != nil {
			   if errors.Is(err, ErrNeedsHuman) || errors.Is(err, ErrAutoUnfreezeDenied) {
				  rep.Denied++
				   continue
			   }
			  return rep, err
		   }
		  if view.Plan.State == PlanCommitted {
			   rep.Executed++
		   }
	  }
	   esc, err := o.escalateTimeouts(ctx, now)
	  if err != nil {
		   return rep, err
	   }
	   rep.Escalated += esc
	  return rep, nil
   }

  func (o *Orchestrator) drainInbox(ctx context.Context, limit int) (int, int, error) {
	  if limit <= 0 || limit > workerBatch {
		  limit = workerBatch
	  }
	   n := len(o.inbox)
	  if n > limit {
		  n = limit
	  }
	  batch := append([]FailureFact(nil), o.inbox[:n]...)
	  o.inbox = append([]FailureFact(nil), o.inbox[n:]...)
	  ingested, replayed := 0, 0
	   for _, fact := range batch {
		  res, err := o.ingest(ctx, fact)
		  if err != nil {
			  if errors.Is(err, ErrSuccessPath) || errors.Is(err, ErrNothingToCompensate) {
				   continue
			  }
			   return ingested, replayed, err
		  }
		   if res.Replayed {
			   replayed++
			  continue
		  }
		  ingested++
	  }
	  return ingested, replayed, nil
   }

   func (o *Orchestrator) idsWhere(pred func(*WdFailure) bool) []string {
	  ids := make([]string, 0)
	  for id, f := range o.fail {
		  if pred(f) {
			   ids = append(ids, id)
		  }
	  }
	   sort.Strings(ids)
	   return ids
  }

   func (o *Orchestrator) planIDs(state PlanState) []string {
	  ids := make([]string, 0)
	   for id, p := range o.plans {
		   if p.State == state {
			  ids = append(ids, id)
		  }
	  }
	  sort.Strings(ids)
	   return ids
  }

  func (o *Orchestrator) escalateTimeouts(ctx context.Context, now time.Time) (int, error) {
	  n := 0
	   ids := make([]string, 0, len(o.plans))
	  for id := range o.plans {
		  ids = append(ids, id)
	  }
	  sort.Strings(ids)
	  for _, id := range ids {
		   p := o.plans[id]
		  if p.State != PlanPlanning && p.State != PlanExecuting {
			   continue
		   }
		   if p.TimeoutAt.IsZero() || now.Before(p.TimeoutAt) {
			  continue
		   }
		   f := o.fail[p.FailureID]
		  if f == nil {
			   continue
		   }
		   if err := o.enterHuman(ctx, p, f, "timeout", now); err != nil {
			  return n, err
		   }
		   n++
	   }
	  return n, nil
  }

// IngestWorker 只消化队列。
type IngestWorker struct{ O *Orchestrator }

  func (w IngestWorker) Run(ctx context.Context, now time.Time) (int, error) {
	   if w.O == nil {
		  return 0, ErrUnavailable
	   }
	  if err := ctx.Err(); err != nil {
		  return 0, err
	   }
	   w.O.mu.Lock()
	  defer w.O.mu.Unlock()
	  n, _, err := w.O.drainInbox(ctx, workerBatch)
	   return n, err
   }

// PlanWorker 只给未规划的失败单建计划。

type PlanWorker struct{ O *Orchestrator }

  func (w PlanWorker) Run(ctx context.Context, now time.Time) (int, error) {
	   if w.O == nil {
		   return 0, ErrUnavailable
	  }
	  if err := ctx.Err(); err != nil {
		  return 0, err
	  }
	   w.O.mu.Lock()
	  defer w.O.mu.Unlock()
	  if now.IsZero() {
		   now = w.O.now()
	   }
	  n := 0
	  for _, id := range w.O.idsWhere(func(f *WdFailure) bool { return f.Status == FailOpen && f.PlanID == "" }) {
		   if _, err := w.O.build(ctx, id, now); err != nil {
			  return n, err
		   }
		  n++
	  }
	   return n, nil
   }

// LegWorker 执行已进入 executing 或仍可自动的 planning 计划。
type LegWorker struct{ O *Orchestrator }

  func (w LegWorker) Run(ctx context.Context, now time.Time) (int, error) {
	  if w.O == nil {
		   return 0, ErrUnavailable
	   }
	   if err := ctx.Err(); err != nil {
		  return 0, err
	   }
	   w.O.mu.Lock()
	  defer w.O.mu.Unlock()
	  if now.IsZero() {
		   now = w.O.now()
	   }
	   n := 0
	  for _, id := range w.O.planIDs(PlanPlanning) {
		  p := w.O.plans[id]
		  if p == nil || !p.AutoOK || p.LiveHash || p.ChainBlocked {
			  continue
		  }
		   view,
			   err := w.O.execute(ctx, id, now)
		   if err != nil {
			  if errors.Is(err, ErrNeedsHuman) || errors.Is(err, ErrAutoUnfreezeDenied) {
				  continue
			  }
			  return n, err
		   }
		   if view.Plan.State == PlanCommitted {
			  n++
		   }
	  }
	  for _, id := range w.O.planIDs(PlanExecuting) {
		   view, err := w.O.execute(ctx, id, now)
		   if err != nil {
			  if errors.Is(err, ErrNeedsHuman) || errors.Is(err, ErrAutoUnfreezeDenied) {
				   continue
			   }
			   return n, err
		  }
		  if view.Plan.State == PlanCommitted {
			  n++
		   }
	   }
	   return n, nil
  }

// TimeoutWorker 把超时计划升到人工闸。

type TimeoutWorker struct{ O *Orchestrator }

  func (w TimeoutWorker) Run(ctx context.Context, now time.Time) (int, error) {
	   if w.O == nil {
		  return 0, ErrUnavailable
	  }
	  if err := ctx.Err(); err != nil {
		  return 0, err
	  }
	   w.O.mu.Lock()
	  defer w.O.mu.Unlock()
	   if now.IsZero() {
		   now = w.O.now()
	   }
	  return w.O.escalateTimeouts(ctx, now)
   }
