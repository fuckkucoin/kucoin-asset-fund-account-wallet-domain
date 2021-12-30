package webhook

import (
	"context"
	"strings"
)

   func (q *DLQ) At(i int) (Dead, bool) {
	  if q == nil {
		  return Dead{}, false
	  }
	  q.mu.Lock()
	   defer q.mu.Unlock()
	  if i < 0 || i >= len(q.rows) {
		  return Dead{}, false
	   }
	  return q.rows[i], true
  }

  func (q *DLQ) Drop(i int) {
	  if q == nil {
		   return
	  }
	   q.mu.Lock()
	  defer q.mu.Unlock()
	  if i < 0 || i >= len(q.rows) {
		   return
	   }
	  q.rows = append(q.rows[:i], q.rows[i+1:]...)
  }

  func (q *DLQ) replace(i int, d Dead) {
	  q.mu.Lock()
	   if i >= 0 && i < len(q.rows) {
		   q.rows[i] = d
	   }
	  q.mu.Unlock()
  }

// Replay 把 DLQ 里一条再投一次。成功才摘掉，失败更新次数，不得当已通知。

  func (d *Dispatcher) Replay(ctx context.Context, idx int) (Delivery, error) {
	  if d == nil {
		  return Delivery{}, ErrUnavailable
	   }
	   dead, ok := d.dlq.At(idx)
	   if !ok {
		  return Delivery{}, ErrDead
	  }
	  ep, ok := d.reg.Get(dead.Endpoint)
	  if !ok || strings.TrimSpace(ep.Secret) == "" {
		   return Delivery{}, ErrEndpoint
	  }
	   env := dead.Env
	   if env.ID == "" {
		   shaped, err := Shape(ep.Secret, dead.Kind, dead.Endpoint+"-replay", dead.Body, dead.Kind+"-replay", d.clock())
		  if err != nil {
			  return Delivery{}, err
		  }
		   env = shaped
	   }
	   del := Delivery{Endpoint: ep.ID, Kind: dead.Kind, ID: env.ID, At: d.clock(), Attempts: dead.Attempts}
	   err := d.sink(ctx, ep, env)
	  del.Attempts++
	   if err != nil {
		  del.Err = err.Error()
		  dead.Attempts = del.Attempts
		   dead.LastErr = err.Error()
		  dead.At = del.At
		   d.dlq.replace(idx, dead)
		   return del, err
	   }
	  del.OK = true
	   d.dlq.Drop(idx)
	  return del, nil
  }
