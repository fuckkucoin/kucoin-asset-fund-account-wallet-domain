package ha

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"account-assets.kucoin.net/funding/internal/metrics"
)

var (
	ErrNotLeader = fmt.Errorf("not the region leader")
	ErrLostLease = fmt.Errorf("ha lease lost")
)

type Record struct {
	Service string `json:"service"`
	Region string `json:"region"`
	Leader bool `json:"leader"`
	Epoch int64 `json:"fence_epoch"`
	Token string `json:"fence_token,omitempty"`
	Peer  string `json:"holder,omitempty"`
	Label string `json:"label"`
	Key string `json:"key"`
}

type backend interface {
	campaign(ctx context.Context, key, token string, ttl time.Duration) (won bool, holder string, err error)
	touch(ctx context.Context,
		key, token string, ttl time.Duration) error
	release(ctx context.Context, key, token string)
	bump(ctx context.Context, epochKey string) (int64, error)
}

type Elector struct {
	service string
	region string
	key    string
	epochK string
	ttl time.Duration
	be backend
	token string
	epoch  atomic.Int64
	leader atomic.Bool
}

   func NewElector(service, region string, be backend, ttl time.Duration) *Elector {
	   service = strings.TrimSpace(service)
	  if service == "" {
		  service = "ledger"
	   }
	  region = strings.TrimSpace(region)
	  if region == "" {
		  region = "sg"
	   }
	  if ttl <= 0 {
		   ttl = 8 * time.Second
	   }
	  if be == nil {
		   be = NewMemBackend()
	  }
	  return &Elector{
		  service: service,
		   region: region,
		   key:    "ha:leader:" + service,
		   epochK: "ha:epoch:" + service,
		   ttl:    ttl,
		  be:     be,
		   token: uuid.NewString(),
	  }
   }

  func FromEnv(rdb *redis.Client) *Elector {
	  svc := os.Getenv("HA_SERVICE")
	   if svc == "" {
		   svc = "ledger"
	  }
	   region := os.Getenv("REGION")
	  if region == "" {
		   region = "sg"
	  }
	   var be backend
	  if rdb != nil {
		  be = RedisBackend{R: rdb}
	   } else {
		   be = NewMemBackend()
	   }
	  return NewElector(svc, region, be, 8*time.Second)
  }

// Campaign 争选主。
// 入参：ctx 取消则停；本进程 token 固定，续租不换。
// 出参：Record.Leader / Fence epoch+token。别人持锁返回 ErrNotLeader，调用方不得写账。
// 第一次赢才 bump epoch；同一 token 续租不涨 epoch，避免自己把自己围出去。
  func (e *Elector) Campaign(ctx context.Context) (Record, error) {
	   if e == nil || e.be == nil {
		  return Record{}, ErrLostLease
	   }
	  won, holder, err := e.be.campaign(ctx, e.key, e.token, e.ttl)
	  if err != nil {
		  return e.Status(), err
	  }
	  if !won {
		  e.leader.Store(false)
		  rec := e.Status()
		  rec.Peer = holder
		   return rec, ErrNotLeader
	  }
	   if !e.leader.Load() {
		   ep, err := e.be.bump(ctx, e.epochK)
		  if err != nil {
			  _ = e.be.touch(ctx, e.key, e.token, 0)
			  e.be.release(ctx, e.key, e.token)
			  return e.Status(), err
		   }
		   e.epoch.Store(ep)
		   e.leader.Store(true)
		   metrics.HAElections.WithLabelValues(e.service, e.region).Inc()
	   }
	   return e.Status(), nil
  }

// Heartbeat 续租。失败立刻卸主，后续 RequireLeader 必须挡住写。
   func (e *Elector) Heartbeat(ctx context.Context) error {
	   if e == nil || !e.leader.Load() {
		  return ErrNotLeader
	  }
	   if err := e.be.touch(ctx, e.key, e.token, e.ttl); err != nil {
		  e.leader.Store(false)
		   return err
	   }
	   return nil
   }

   func (e *Elector) Relinquish(ctx context.Context) {
	   if e == nil {
		  return
	   }
	  e.leader.Store(false)
	  e.be.release(ctx, e.key, e.token)
   }

   func (e *Elector) IsLeader() bool { return e != nil && e.leader.Load() }

  func (e *Elector) Fence() (int64, string) {
	   if e == nil {
		  return 0, ""
	  }
	  return e.epoch.Load(), e.token
   }

   func (e *Elector) RequireLeader() error {
	   if e == nil || !e.leader.Load() {
		  return ErrNotLeader
	  }
	   return nil
  }

  func (e *Elector) Status() Record {
	  if e == nil {
		   return Record{Label: "dual-region write fence"}
	   }
	  return Record{
		  Service: e.service, Region: e.region, Leader: e.leader.Load(),
		   Epoch: e.epoch.Load(), Token: e.token, Key: e.key,
		  Label: "dual-region write fence",
	   }
   }

type memBackend struct {
	mu     sync.Mutex
	holder map[string]string
	epoch map[string]int64
}

   func NewMemBackend() *memBackend {
	   return &memBackend{holder: map[string]string{}, epoch: map[string]int64{}}
   }

  func (m *memBackend) campaign(_ context.Context, key, token string, _ time.Duration) (bool, string, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  cur, ok := m.holder[key]
	   if !ok || cur == "" || cur == token {
		   m.holder[key] = token
		   return true, token, nil
	   }
	  return false, cur, nil
  }

   func (m *memBackend) touch(_ context.Context, key, token string, _ time.Duration) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if m.holder[key] != token {
		   return ErrLostLease
	   }
	  return nil
  }

   func (m *memBackend) release(_ context.Context, key, token string) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   if m.holder[key] == token {
		   delete(m.holder, key)
	   }
   }

  func (m *memBackend) bump(_ context.Context, epochKey string) (int64, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  m.epoch[epochKey]++
	  return m.epoch[epochKey], nil
   }

type RedisBackend struct{ R *redis.Client }

  func (b RedisBackend) campaign(ctx context.Context, key, token string, ttl time.Duration) (bool, string, error) {
	  ok, err := b.R.SetNX(ctx, key, token, ttl).Result()
	  if err != nil {
		   return false, "", err
	  }
	  if ok {
		   return true, token, nil
	   }
	   cur, err := b.R.Get(ctx, key).Result()
	  if err == redis.Nil {
		  return false, "", nil
	  }
	   if err != nil {
		  return false, "", err
	   }
	  if cur == token {
		   _ = b.R.Expire(ctx,
			  key, ttl).Err()
		  return true, token, nil
	  }
	  return false, cur, nil
  }

  func (b RedisBackend) touch(ctx context.Context, key, token string, ttl time.Duration) error {
	  n, err := touchLua.Run(ctx, b.R, []string{key}, token, ttl.Milliseconds()).Int64()
	   if err != nil {
		  return err
	  }
	  if n == 0 {
		  return ErrLostLease
	   }
	   return nil
  }

   func (b RedisBackend) release(ctx context.Context, key, token string) {
	   _ = unlockLua.Run(ctx, b.R, []string{key}, token).Err()
  }

  func (b RedisBackend) bump(ctx context.Context, epochKey string) (int64, error) {
	   return b.R.Incr(ctx, epochKey).Result()
   }

var unlockLua = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

var touchLua = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return 0
`)
