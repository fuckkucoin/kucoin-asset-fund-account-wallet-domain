package svcreg

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"github.com/rs/zerolog"
)

const (
	defaultLeaseTTL    = int64(15)
	defaultHTTPTimeout = 2 * time.Second
)

// Etcd 用 etcd v3 gRPC 做 Put/Range/租约。
// 多 endpoint 轮询切主；租约 grant + KeepAlive；Register/Deregister/Watch/Pick。端点已配但拨号失败时返回错误并由调用方日志重试，不伪造实例、不退回内存表。全部端点同时失败时 Pick 返回 ErrNoInstance；lease 失效需重新 Register。

type Etcd struct {
	endpoints []string
	dials     []etcdEndpoint
	keyPrefix string
	log   zerolog.Logger
	mu    sync.Mutex
	lease int64
	epIdx uint64
	closed   bool
	pickMode PickMode
	rr       map[string]*uint64
	inflight map[string]*int64
	cacheMu  sync.RWMutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	list []Instance
	at time.Time
}

// OpenEtcd 解析多端点；全部为空返回 ErrEndpoint。打开成功不代表集群可达，首次 Register/KeepAlive 才探活。
  func OpenEtcd(endpoints []string, keyPrefix string, log zerolog.Logger) (*Etcd, error) {
	  eps := normalizeEndpoints(endpoints)
	  if len(eps) == 0 {
		  return nil, ErrEndpoint
	  }
	   if keyPrefix == "" {
		  keyPrefix = "/aa/svc"
	  }
	  dials := make([]etcdEndpoint, 0, len(eps))
	   for _, ep := range eps {
		  target,
			  useTLS, err := parseEtcdTarget(ep)
		  if err != nil || target == "" {
			   continue
		  }
		  dials = append(dials, etcdEndpoint{target: target, tls: useTLS})
	  }
	  if len(dials) == 0 {
		   return nil, ErrEndpoint
	   }
	   return &Etcd{
		  endpoints: eps,
		  dials: dials,
		  keyPrefix: keyPrefix,
		  log:       log,
		  pickMode:  PickP2C,
		   rr:        map[string]*uint64{},
		   inflight:  map[string]*int64{},
		   cache: map[string]cacheEntry{},
	  }, nil
   }

   func normalizeEndpoints(in []string) []string {
	  out := make([]string, 0, len(in))
	  seen := map[string]struct{}{}
	   for _, ep := range in {
		   ep = strings.TrimRight(strings.TrimSpace(ep), "/")
		   if ep == "" {
			  continue
		  }
		  if _, ok := seen[ep]; ok {
			  continue
		   }
		   seen[ep] = struct{}{}
		  out = append(out, ep)
	   }
	  return out
   }

   func (e *Etcd) Kind() string { return "etcd" }

// SetPickMode 切换 P2C / RR。默认 P2C。

   func (e *Etcd) SetPickMode(m PickMode) {
	   if e == nil {
		   return
	  }
	   e.mu.Lock()
	  e.pickMode = m
	  e.mu.Unlock()
  }

  func (e *Etcd) Register(ctx context.Context, inst Instance) error {
	   if e == nil {
		   return ErrClosed
	   }
	   inst.Name = strings.TrimSpace(inst.Name)
	   inst.Addr = strings.TrimSpace(inst.Addr)
	   if inst.Name == "" || inst.Addr == "" {
		   return ErrEndpoint
	  }
	   if err := e.ensureLease(ctx); err != nil {
		  e.log.Error().Err(err).Str("addr", inst.Addr).Strs("endpoints", e.endpoints).Msg("registry lease")
		   observeRegistry("register", "lease_fail")
		  return err
	  }
	   body, _ := json.Marshal(inst)
	   key := e.keyPrefix + "/" + inst.Name + "/" + inst.Addr
	   if err := e.kvPut(ctx, key, body, e.currentLease()); err != nil {
		  e.log.Error().Err(err).Str("name", inst.Name).Str("addr", inst.Addr).Msg("registry put")
		   observeRegistry("register", "put_fail")
		  return err
	   }
	   observeRegistry("register", "ok")
	  e.invalidateCache(inst.Name)
	   e.log.Info().Str("name", inst.Name).Str("addr", inst.Addr).Int64("lease", e.currentLease()).Msg("registry registered")
	   return nil
   }

   func (e *Etcd) Deregister(ctx context.Context, inst Instance) error {
	  if e == nil {
		   return ErrClosed
	   }
	  inst.Name = strings.TrimSpace(inst.Name)
	  inst.Addr = strings.TrimSpace(inst.Addr)
	   if inst.Name == "" || inst.Addr == "" {
		   return ErrEndpoint
	  }
	  err := e.kvDel(ctx, e.keyPrefix+"/"+inst.Name+"/"+inst.Addr)
	   if err != nil {
		  observeRegistry("deregister", "fail")
		  e.log.Error().Err(err).Str("name", inst.Name).Str("addr", inst.Addr).Msg("registry delete")
		  return err
	   }
	   observeRegistry("deregister", "ok")
	   e.invalidateCache(inst.Name)
	  e.log.Info().Str("name", inst.Name).Str("addr", inst.Addr).Msg("registry deregistered")
	  return nil
   }

  func (e *Etcd) Watch(name string) []Instance {
	  if e == nil {
		   return nil
	  }
	  name = strings.TrimSpace(name)
	  if name == "" {
		   return nil
	  }
	  if list, ok := e.cached(name); ok {
		  return list
	  }
	   ctx, cancel := context.WithTimeout(context.Background(), defaultHTTPTimeout)
	  defer cancel()
	  list, err := e.kvRange(ctx, e.keyPrefix+"/"+name+"/")
	   if err != nil {
		   e.log.Error().Err(err).Str("name", name).Msg("registry watch")
		  observeRegistry("watch", "fail")
		  return nil
	  }
	  observeRegistry("watch", "ok")
	   e.storeCache(name, list)
	  return list
  }

  func (e *Etcd) Pick(name string) (Instance, error) {
	  var list []Instance
	  for _, n := range AliasNames(name) {
		  list = e.Watch(n)
		   if len(list) > 0 {
			  break
		  }
	  }
	   if len(list) == 0 {
		   observeRegistry("pick", "empty")
		   return Instance{}, ErrNoInstance
	   }
	   e.mu.Lock()
	   mode := e.pickMode
	  if e.rr[name] == nil {
		   n := uint64(0)
		   e.rr[name] = &n
	  }
	   counter := e.rr[name]
	  e.mu.Unlock()
	   var inst Instance
	  switch mode {
	  case PickRR:
		   inst = pickRR(list, counter)
	  default:
		   inst = pickP2CLoad(list, e.loadOf)
	   }
	   observeRegistry("pick", "ok")
	   return inst, nil
  }

// ReportDial 供 svcdial 回写实例在途计数，影响 P2C 负载偏好。

  func (e *Etcd) ReportDial(addr string, delta int64) {
	  if e == nil || addr == "" {
		   return
	   }
	  e.mu.Lock()
	   p := e.inflight[addr]
	  if p == nil {
		  var z int64
		   e.inflight[addr] = &z
		   p = e.inflight[addr]
	  }
	   e.mu.Unlock()
	   atomic.AddInt64(p, delta)
  }

  func (e *Etcd) loadOf(addr string) int64 {
	  e.mu.Lock()
	  p := e.inflight[addr]
	   e.mu.Unlock()
	  if p == nil {
		  return 0
	  }
	  return atomic.LoadInt64(p)
   }

   func (e *Etcd) Close() error {
	  if e == nil {
		  return nil
	  }
	   e.mu.Lock()
	   lease := e.lease
	   e.lease = 0
	   e.mu.Unlock()
	   if lease != 0 {
		  ctx, cancel := context.WithTimeout(context.Background(), defaultHTTPTimeout)
		   defer cancel()
		  _ = e.revokeLease(ctx, lease)
	  }
	  e.mu.Lock()
	  e.closed = true
	   e.mu.Unlock()
	   e.closeConns()
	   return nil
  }

// KeepAlive 续租。失败清零 lease，下次 Register 重新 grant；失败返回错误。

  func (e *Etcd) KeepAlive(ctx context.Context) error {
	   if e == nil {
		   return ErrClosed
	  }
	   e.mu.Lock()
	   id := e.lease
	   closed := e.closed
	  e.mu.Unlock()
	   if closed {
		   return ErrClosed
	  }
	  if id == 0 {
		   return e.ensureLease(ctx)
	   }
	  if err := e.keepAliveRPC(ctx, id); err != nil {
		  e.mu.Lock()
		   e.lease = 0
		   e.mu.Unlock()
		   e.log.Error().Err(err).Int64("lease", id).Msg("registry lease keepalive")
		   observeRegistry("keepalive", "fail")
		   return err
	  }
	   observeRegistry("keepalive", "ok")
	  return nil
   }

// PutRaw / GetRaw 给配置中心复用同一 etcd gRPC 客户端。

  func (e *Etcd) PutRaw(ctx context.Context, key string, val []byte) error {
	   if e == nil {
		   return ErrClosed
	   }
	  return e.kvPut(ctx, key, val, 0)
   }

   func (e *Etcd) GetRaw(ctx context.Context, key string) ([]byte, error) {
	  if e == nil {
		   return nil, ErrClosed
	   }
	  raw, err := e.rpc(ctx, methodKVRange, encodeRange([]byte(key), nil))
	  if err != nil {
		   return nil, err
	   }
	   val, ok := decodeFirstValue(raw)
	  if !ok {
		  return nil, ErrNoInstance
	  }
	  return val, nil
   }

  func (e *Etcd) currentLease() int64 {
	  e.mu.Lock()
	   defer e.mu.Unlock()
	   return e.lease
   }

  func (e *Etcd) ensureLease(ctx context.Context) error {
	  e.mu.Lock()
	  if e.closed {
		  e.mu.Unlock()
		   return ErrClosed
	  }
	   if e.lease != 0 {
		   e.mu.Unlock()
		   return nil
	  }
	   e.mu.Unlock()
	   id, err := e.grantLease(ctx)
	  if err != nil {
		  return err
	   }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	  if e.closed {
		  return ErrClosed
	   }
	   if e.lease == 0 {
		   e.lease = id
	  }
	   e.log.Info().Int64("lease", e.lease).Int64("ttl", defaultLeaseTTL).Msg("registry lease granted")
	   return nil
   }

   func (e *Etcd) cached(name string) ([]Instance, bool) {
	   e.cacheMu.RLock()
	  ent, ok := e.cache[name]
	   e.cacheMu.RUnlock()
	  if !ok || time.Since(ent.at) > 2*time.Second {
		   return nil, false
	   }
	   out := make([]Instance, len(ent.list))
	  copy(out, ent.list)
	   return out, true
  }

  func (e *Etcd) storeCache(name string, list []Instance) {
	   cp := make([]Instance, len(list))
	  copy(cp, list)
	   e.cacheMu.Lock()
	  e.cache[name] = cacheEntry{list: cp, at: time.Now()}
	  e.cacheMu.Unlock()
  }

  func (e *Etcd) invalidateCache(name string) {
	   e.cacheMu.Lock()
	   delete(e.cache, name)
	  e.cacheMu.Unlock()
  }

  func prefixEnd(prefix string) []byte {
	   b := []byte(prefix)
	  for i := len(b) - 1; i >= 0; i-- {
		   if b[i] < 0xff {
			  b[i]++
			  return b[:i+1]
		   }
	   }
	  return append(b, 0)
  }
