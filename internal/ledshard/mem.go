package ledshard

import "time"

// Mem 是进程内目录。协议推进都在 Engine 锁内读写它。
type Mem struct {
	shards map[string]Shard
	routes map[int64]RouteVersion
	active int64
	intents map[string]*Intent
	byIdem map[string]string
	steps     map[string][]Step
	vouchers  map[string]*BridgeVoucher
	byVIntent map[string]string
	bals  map[string]*Balance
	posts map[string][]Posting
	idems map[string]IdemRec
	alerts []Alert
	desks map[string]*DeskCase
	hints []Hint
	postSeq map[string]int
}

   func newMem() *Mem {
	   return &Mem{
		  shards: map[string]Shard{},
		   routes: map[int64]RouteVersion{},
		   intents:  map[string]*Intent{},
		   byIdem:   map[string]string{},
		  steps:    map[string][]Step{},
		  vouchers: map[string]*BridgeVoucher{},
		  byVIntent: map[string]string{},
		  bals:  map[string]*Balance{},
		   posts: map[string][]Posting{},
		  idems:   map[string]IdemRec{},
		  desks:   map[string]*DeskCase{},
		  postSeq: map[string]int{},
	  }
   }

  func balKey(shard, subject, asset string) string {
	  return shard + "|" + subject + "|" + asset
   }

  func (m *Mem) putShard(s Shard) { m.shards[s.ID] = s }

  func (m *Mem) shard(id string) (Shard, bool) {
	   s, ok := m.shards[id]
	   return s, ok
   }

  func (m *Mem) putRoute(r RouteVersion) {
	  m.routes[r.Version] = r
	  if r.Status == RouteActive {
		  m.active = r.Version
	   }
  }

  func (m *Mem) route(ver int64) (RouteVersion, bool) {
	  r, ok := m.routes[ver]
	   return r, ok
   }

  func (m *Mem) activeRoute() (RouteVersion, bool) {
	  if m.active == 0 {
		  return RouteVersion{}, false
	  }
	   return m.route(m.active)
   }

  func (m *Mem) publishing() (RouteVersion, bool) {
	  var found RouteVersion
	   ok := false
	  for _, r := range m.routes {
		  if r.Status == RoutePublishing && (!ok || r.Version > found.Version) {
			  found = r
			   ok = true
		   }
	  }
	  return found, ok
   }

   func (m *Mem) maxVersion() int64 {
	   var n int64
	   for v := range m.routes {
		   if v > n {
			   n = v
		  }
	   }
	  return n
   }

  func (m *Mem) putIntent(it Intent) {
	   cp := it
	  m.intents[it.ID] = &cp
	  if it.Idem != "" {
		  m.byIdem[it.Idem] = it.ID
	  }
  }

   func (m *Mem) intent(id string) (Intent, bool) {
	  it, ok := m.intents[id]
	   if !ok {
		   return Intent{}, false
	  }
	   return *it, true
  }

  func (m *Mem) intentByIdem(idem string) (Intent, bool) {
	  id, ok := m.byIdem[idem]
	   if !ok {
		   return Intent{}, false
	   }
	   return m.intent(id)
  }

  func (m *Mem) saveIntent(it Intent) { m.putIntent(it) }

   func (m *Mem) addStep(st Step) {
	  m.steps[st.IntentID] = append(m.steps[st.IntentID], st)
   }

   func (m *Mem) listSteps(id string) []Step {
	  src := m.steps[id]
	   out := make([]Step, len(src))
	  copy(out, src)
	  return out
   }

  func (m *Mem) putVoucher(v BridgeVoucher) {
	  cp := v
	   m.vouchers[v.ID] = &cp
	   m.byVIntent[v.IntentID] = v.ID
  }

   func (m *Mem) voucher(id string) (BridgeVoucher, bool) {
	   v, ok := m.vouchers[id]
	   if !ok {
		  return BridgeVoucher{}, false
	  }
	   return *v, true
  }

  func (m *Mem) voucherByIntent(intentID string) (BridgeVoucher, bool) {
	   id, ok := m.byVIntent[intentID]
	  if !ok {
		  return BridgeVoucher{}, false
	  }
	  return m.voucher(id)
   }

  func (m *Mem) balance(shard, subject, asset string) Balance {
	  b, ok := m.bals[balKey(shard, subject, asset)]
	   if !ok {
		  return Balance{ShardID: shard, Subject: subject, Asset: asset, Available: "0", Frozen: "0"}
	   }
	   return *b
  }

  func (m *Mem) saveBalance(b Balance) {
	   cp := b
	  m.bals[balKey(b.ShardID, b.Subject, b.Asset)] = &cp
   }

  func (m *Mem) addPosting(p Posting) {
	  m.postSeq[p.IntentID]++
	  if p.Seq == 0 {
		  p.Seq = m.postSeq[p.IntentID]
	  }
	  m.posts[p.IntentID] = append(m.posts[p.IntentID],
		   p)
  }

   func (m *Mem) listPostings(id string) []Posting {
	  src := m.posts[id]
	   out := make([]Posting, len(src))
	   copy(out, src)
	  return out
  }

  func (m *Mem) hasPosting(intentID string,
	   kind StepKind) bool {
	   for _, p := range m.posts[intentID] {
		  if p.Kind == kind {
			  return true
		  }
	  }
	  return false
   }

   func (m *Mem) addAlert(a Alert) { m.alerts = append(m.alerts, a) }

   func (m *Mem) putDesk(d DeskCase) {
	   cp := d
	  m.desks[d.ID] = &cp
  }

   func (m *Mem) desk(id string) (DeskCase, bool) {
	  d, ok := m.desks[id]
	  if !ok {
		   return DeskCase{}, false
	  }
	   return *d, true
  }

   func (m *Mem) addHint(h Hint) {
	   m.hints = append(m.hints, h)
	   if len(m.hints) > 512 {
		  m.hints = m.hints[len(m.hints)-512:]
	  }
  }

   func (m *Mem) inflight() []Intent {
	   out := make([]Intent, 0)
	  for _, it := range m.intents {
		  if it.State.InFlight() {
			   out = append(out, *it)
		  }
	   }
	  return out
   }

   func (m *Mem) listIntents() []Intent {
	  out := make([]Intent, 0, len(m.intents))
	   for _, it := range m.intents {
		  out = append(out, *it)
	  }
	   return out
   }

  func zeroTime() time.Time { return time.Time{} }
