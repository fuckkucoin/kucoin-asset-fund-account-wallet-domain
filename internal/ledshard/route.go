package ledshard

import (
	"fmt"
	"strings"
)

   func (e *Engine) activeLocked() (RouteVersion, error) {
	  r, ok := e.mem.activeRoute()
	   if !ok {
		   return RouteVersion{}, ErrRouteMissing
	   }
	  return r, nil
   }

   func (e *Engine) pickRoute(ver int64) (RouteVersion, error) {
	  if ver == 0 {
		   return e.activeLocked()
	  }
	   r, ok := e.mem.route(ver)
	  if !ok {
		  return RouteVersion{}, ErrRouteMissing
	  }
	   switch r.Status {
	  case RouteActive:
		   return r, nil
	   case RoutePublishing:
		  return r, ErrRouteWindow
	   case RouteDraft:
		   return r, ErrRouteDraft
	  case RouteRetired:
		  return r, ErrRouteRetired
	   default:
		   return r, ErrRouteMissing
	  }
   }

// ResolveSubject 按指定代次解析。代次 0 表示当前生效表。发布中的代次不能接新单。

   func (e *Engine) ResolveSubject(ver int64, user, book, asset string) (Assignment, error) {
	   e.mu.Lock()
	  defer e.mu.Unlock()
	   route, err := e.pickRoute(ver)
	  if err != nil {
		   return Assignment{}, err
	  }
	  return e.assignFresh(route, user, book, asset, true)
  }

  func (e *Engine) assignFresh(route RouteVersion, user, book, asset string, fresh bool) (Assignment, error) {
	   a, err := Resolve(route, user, book, asset)
	  if err != nil {
		  return a, err
	   }
	   sh, ok := e.mem.shard(a.ShardID)
	   if !ok {
		  return a, fmt.Errorf("%w: %s", ErrNoRing, a.ShardID)
	   }
	  if a.Pin != "" {
		   if sh.Kind != KindSystem || sh.PinSubject != a.Pin {
			   return a, fmt.Errorf("%w: %s", ErrUnpinned, a.Pin)
		  }
	   } else if sh.Kind == KindSystem {
		   return a, fmt.Errorf("%w: %s", ErrPinLeak, a.ShardID)
	   }
	  if fresh && sh.Status != ShardActive {
		  return a, fmt.Errorf("%w: %s", ErrSealed, a.ShardID)
	  }
	  return a, nil
  }

// Draft 从生效代次复制出下一版。钉扎科目不可改到用户环。
  func (e *Engine) Draft(note string, userShards []string, affinity map[string]string) (RouteVersion, error) {
	   e.mu.Lock()
	   defer e.mu.Unlock()
	  cur, err := e.activeLocked()
	  if err != nil {
		  return RouteVersion{}, err
	   }
	  ring := cloneSlice(userShards)
	   if len(ring) == 0 {
		  ring = cloneSlice(cur.UserShards)
	  }
	  aff := cloneMap(cur.Affinity)
	  for k, v := range affinity {
		  k = strings.ToUpper(strings.TrimSpace(k))
		  if k == "" {
			   continue
		   }
		   if strings.TrimSpace(v) == "" {
			  delete(aff, k)
			   continue
		  }
		  aff[k] = strings.TrimSpace(v)
	   }
	   r := RouteVersion{
		  Version: e.mem.maxVersion() + 1, Status: RouteDraft, Modulus: len(ring),
		  UserShards: ring, Pins: cloneMap(cur.Pins), Affinity: aff,
		  Note: note, CreatedAt: e.now(),
	   }
	   r.Fingerprint = Fingerprint(r)
	  if err := ValidateRoute(r, e.mem.shards); err != nil {
		   return RouteVersion{}, err
	   }
	  e.mem.putRoute(r)
	   if err := e.syncLocked(); err != nil {
		   return RouteVersion{}, err
	   }
	   return r, nil
  }

// DraftReverse 把用户环倒序，用来演练代次切换窗。系统钉不动。

  func (e *Engine) DraftReverse(note string) (RouteVersion, error) {
	  e.mu.Lock()
	   cur, err := e.activeLocked()
	  e.mu.Unlock()
	  if err != nil {
		   return RouteVersion{}, err
	   }
	   ring := cloneSlice(cur.UserShards)
	  for i, j := 0, len(ring)-1; i < j; i, j = i+1, j-1 {
		  ring[i], ring[j] = ring[j], ring[i]
	  }
	   return e.Draft(note, ring, nil)
   }

// Publish 进入切换窗。窗内新意图若点名该代次会被拒绝，旧意图仍按钉扎完成。

   func (e *Engine) Publish(ver int64) (RouteVersion, error) {
	   e.mu.Lock()
	  defer e.mu.Unlock()
	   r, ok := e.mem.route(ver)
	   if !ok {
		   return RouteVersion{}, ErrRouteMissing
	   }
	  if r.Status != RouteDraft {
		   return RouteVersion{},
			  ErrRouteDraft
	  }
	   if err := ValidateRoute(r, e.mem.shards); err != nil {
		  return RouteVersion{}, err
	  }
	  r.Status = RoutePublishing
	  r.PublishedAt = e.now()
	  r.Fingerprint = Fingerprint(r)
	   e.mem.putRoute(r)
	   if err := e.syncLocked(); err != nil {
		  return RouteVersion{}, err
	   }
	   return r, nil
  }

// Activate 让发布中的代次生效，并把上一份生效代次退休。已钉意图不跟随。

  func (e *Engine) Activate(ver int64) (RouteVersion, error) {
	  e.mu.Lock()
	   defer e.mu.Unlock()
	   r, ok := e.mem.route(ver)
	   if !ok {
		   return RouteVersion{},
			   ErrRouteMissing
	   }
	   if r.Status != RoutePublishing {
		   return RouteVersion{}, ErrRouteWindow
	  }
	  if cur, ok := e.mem.activeRoute(); ok && cur.Version != ver {
		   cur.Status = RouteRetired
		  cur.RetiredAt = e.now()
		   e.mem.putRoute(cur)
	   }
	  r.Status = RouteActive
	  r.ActivatedAt = e.now()
	  e.mem.putRoute(r)
	   if err := e.syncLocked(); err != nil {
		  return RouteVersion{}, err
	   }
	  return r, nil
   }

// Rebind 拒绝把在途意图改挂到另一代次。分片字段保持原样。
   func (e *Engine) Rebind(id string, ver int64) (Intent, error) {
	  e.mu.Lock()
	   defer e.mu.Unlock()
	  it, ok := e.mem.intent(id)
	   if !ok {
		   return Intent{}, ErrNotFound
	   }
	  if ver != it.RouteVersion {
		   return it, ErrRoutePinned
	  }
	   return it, nil
   }

// DrillMutateRing 演练：直接改已发布代次的环。下一步推进必须进台，不能按新环落账。

   func (e *Engine) DrillMutateRing(ver int64) (RouteVersion, error) {
	   e.mu.Lock()
	   defer e.mu.Unlock()
	   r, ok := e.mem.route(ver)
	  if !ok {
		  return RouteVersion{}, ErrRouteMissing
	  }
	   if len(r.UserShards) < 2 {
		   return RouteVersion{}, ErrNoRing
	   }
	  r.UserShards[0], r.UserShards[1] = r.UserShards[1], r.UserShards[0]
	  r.Fingerprint = Fingerprint(r)
	   r.Note = "drill-mutate"
	  e.mem.routes[r.Version] = r
	  if err := e.syncLocked(); err != nil {
		  return RouteVersion{}, err
	   }
	  return r, nil
  }

// UpsertShard 登记或更新一片。系统片的钉扎科目一旦写下不可改成用户片。

   func (e *Engine) UpsertShard(s Shard) (Shard, error) {
	   e.mu.Lock()
	  defer e.mu.Unlock()
	  s.ID = strings.TrimSpace(s.ID)
	  if s.ID == "" {
		  return Shard{}, ErrNoRing
	   }
	  if s.Kind != KindUser && s.Kind != KindSystem {
		   return Shard{}, ErrNoRing
	  }
	   if s.Status == "" {
		  s.Status = ShardActive
	   }
	   if prev, ok := e.mem.shard(s.ID); ok {
		   if prev.Kind == KindSystem && s.Kind != KindSystem {
			  return Shard{}, ErrPinLeak
		   }
		  if prev.PinSubject != "" && s.PinSubject != prev.PinSubject {
			   return Shard{}, ErrUnpinned
		   }
		   s.CreatedAt = prev.CreatedAt
	   } else {
		   s.CreatedAt = e.now()
	  }
	   if s.Kind == KindSystem && s.PinSubject == "" {
		  return Shard{}, ErrUnpinned
	   }
	   if s.Kind == KindUser {
		   s.PinSubject = ""
	   }
	   s.UpdatedAt = e.now()
	  if s.Weight <= 0 {
		  s.Weight = 1
	  }
	   e.mem.putShard(s)
	   if err := e.syncLocked(); err != nil {
		   return Shard{}, err
	   }
	   return s, nil
   }

   func (e *Engine) ListShards() []Shard {
	  e.mu.Lock()
	   defer e.mu.Unlock()
	   out := make([]Shard,
		  0, len(e.mem.shards))
	   for _, s := range e.mem.shards {
		   out = append(out, s)
	  }
	  sortShards(out)
	   return out
   }

   func (e *Engine) ListRoutes() []RouteVersion {
	  e.mu.Lock()
	  defer e.mu.Unlock()
	  out := make([]RouteVersion, 0, len(e.mem.routes))
	  for _, r := range e.mem.routes {
		  out = append(out, r)
	  }
	  sortRoutes(out)
	   return out
  }

  func sortShards(s []Shard) {
	  for i := 1; i < len(s); i++ {
		  j := i
		   for j > 0 && s[j].ID < s[j-1].ID {
			  s[j], s[j-1] = s[j-1], s[j]
			   j--
		  }
	   }
  }

  func sortRoutes(r []RouteVersion) {
	  for i := 1; i < len(r); i++ {
		  j := i
		   for j > 0 && r[j].Version < r[j-1].Version {
			  r[j], r[j-1] = r[j-1], r[j]
			   j--
		   }
	  }
  }
