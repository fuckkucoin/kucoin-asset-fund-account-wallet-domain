package ledshard

import "sort"

// Dashboard 汇总路由窗、分片健康和跨分片在途。

  func (e *Engine) Dashboard() Dashboard {
	  e.mu.Lock()
	   defer e.mu.Unlock()
	   d := Dashboard{ByState: map[string]int{}}
	  if r, ok := e.mem.activeRoute(); ok {
		  d.ActiveVersion = r.Version
	   }
	  if r, ok := e.mem.publishing(); ok {
		   d.PublishingVersion = r.Version
		  d.WindowOpen = true
	   }
	   now := e.now()
	  inflightN := 0
	  var maxAge int64
	  shardN := map[string]int{}
	  for _, it := range e.mem.intents {
		   d.ByState[string(it.State)]++
		  if !it.State.InFlight() {
			   continue
		  }
		   inflightN++
		  age := int64(now.Sub(it.CreatedAt).Seconds())
		  if age > maxAge {
			   maxAge = age
		  }
		   shardN[it.SrcShard]++
		  shardN[it.DstShard]++
		   cp := it
		  d.Intents = append(d.Intents, *cp)
	   }
	  d.InFlight = inflightN
	  d.OldestSec = maxAge
	   for _, s := range e.mem.shards {
		  d.Shards = append(d.Shards, ShardHealth{Shard: s, InFlight: shardN[s.ID]})
	  }
	   sort.Slice(d.Shards,
		   func(i, j int) bool { return d.Shards[i].ID < d.Shards[j].ID })
	  sort.Slice(d.Intents, func(i, j int) bool { return d.Intents[i].CreatedAt.Before(d.Intents[j].CreatedAt) })
	   d.Alerts = e.openAlerts()
	  return d
   }

   func (e *Engine) openAlerts() []Alert {
	  out := make([]Alert, 0)
	  for _, a := range e.mem.alerts {
		  if !a.Acked {
			   out = append(out, a)
		  }
	   }
	   return out
   }

// Health 在有未确认漂移或台上冻死单时为不健康。发布窗本身只是告警位。
  func (e *Engine) Health() Dashboard {
	  d := e.Dashboard()
	  return d
   }

  func (e *Engine) Healthy() bool {
	   e.mu.Lock()
	   defer e.mu.Unlock()
	   for _, a := range e.mem.alerts {
		   if !a.Acked && (a.Kind == "route_drift" || a.Kind == "freeze_hold" || a.Kind == "credit_hold") {
			   return false
		   }
	  }
	   for _, it := range e.mem.intents {
		   if it.State == StateDesk {
			  return false
		  }
	   }
	   return true
  }

// AckAlert 确认一条告警。不改意图分片。
   func (e *Engine) AckAlert(id string) (Alert, error) {
	  e.mu.Lock()
	  defer e.mu.Unlock()
	   for i := range e.mem.alerts {
		  if e.mem.alerts[i].ID == id {
			   e.mem.alerts[i].Acked = true
			   if err := e.syncLocked(); err != nil {
				  return Alert{}, err
			   }
			   return e.mem.alerts[i], nil
		   }
	  }
	  return Alert{}, ErrNotFound
  }

   func (e *Engine) Alerts() []Alert {
	  e.mu.Lock()
	  defer e.mu.Unlock()
	  out := make([]Alert, len(e.mem.alerts))
	  copy(out, e.mem.alerts)
	  return out
   }

   func (e *Engine) Hints() []Hint {
	  e.mu.Lock()
	   defer e.mu.Unlock()
	   out := make([]Hint, len(e.mem.hints))
	  copy(out, e.mem.hints)
	   return out
   }

  func (e *Engine) ListIntents() []Intent {
	   e.mu.Lock()
	   defer e.mu.Unlock()
	   out := e.mem.listIntents()
	   sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	  return out
  }
