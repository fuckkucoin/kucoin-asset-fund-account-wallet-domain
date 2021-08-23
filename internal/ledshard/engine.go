// @FileName: engine.go
// @Summary: ledshard 模块 engine。
// @Author: Vance
// @Created: 2021.08.15

package ledshard

import (
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
)

// Engine 编排分片路由和跨分片协议。账本 Apply 只在步骤准入时只读调用。

type Engine struct {
	mu  sync.Mutex
	mem *Mem
	sql   *SQLStore
	clock func() time.Time
	prepTO    time.Duration
	freezeTO  time.Duration
	voucherTO time.Duration
	auto      bool
	workerOn  bool
}

   func New() *Engine {
	  return &Engine{
		  mem: newMem(),
		  prepTO: 15 * time.Second,
		  freezeTO: 30 * time.Second,
		   voucherTO: 30 * time.Second,
	  }
  }

  func (e *Engine) WithClock(c func() time.Time) *Engine {
	   if e != nil && c != nil {
		   e.clock = c
	  }
	  return e
  }

   func (e *Engine) EnableAuto(on bool) *Engine {
	   if e != nil {
		   e.auto = on
	  }
	  return e
   }

   func (e *Engine) SetTimeouts(prep, freeze, voucher time.Duration) {
	  if e == nil {
		   return
	   }
	  e.mu.Lock()
	  defer e.mu.Unlock()
	   if prep > 0 {
		   e.prepTO = prep
	  }
	   if freeze > 0 {
		   e.freezeTO = freeze
	  }
	  if voucher > 0 {
		  e.voucherTO = voucher
	   }
  }

   func (e *Engine) id(prefix string) string {
	   return prefix + "-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
  }

// Bootstrap 落下默认用户环和三枚系统钉。重复调用不改已生效代次。
  func (e *Engine) Bootstrap(n int) error {
	   if e == nil {
		   return ErrRouteMissing
	  }
	  e.mu.Lock()
	  defer e.mu.Unlock()
	  if _, ok := e.mem.activeRoute(); ok {
		   return nil
	  }
	   if n <= 0 {
		   n = DefaultRing
	  }
	  now := e.now()
	   for i, id := range DefaultUserShards(n) {
		   region := "sg"
		  if i%2 == 1 {
			  region = "tyo"
		   }
		  e.mem.putShard(Shard{
			  ID: id, Kind: KindUser, Status: ShardActive, Weight: 1, Region: region,
			   CreatedAt: now, UpdatedAt: now,
		  })
	   }
	  pins := DefaultPins()
	   for sub, id := range pins {
		   e.mem.putShard(Shard{
			   ID: id, Kind: KindSystem, Status: ShardActive, Weight: 1,
			   PinSubject: sub, Region: "sg", CreatedAt: now, UpdatedAt: now,
		  })
	   }
	   r := RouteVersion{
		   Version: 1, Status: RouteActive, Modulus: n,
		   UserShards: DefaultUserShards(n),
		  Pins: pins, Affinity: map[string]string{},
		  Note: "bootstrap", CreatedAt: now, ActivatedAt: now,
	   }
	  r.Fingerprint = Fingerprint(r)
	   if err := ValidateRoute(r, e.mem.shards); err != nil {
		  return err
	  }
	  e.mem.putRoute(r)
	  metricsState(StateOpen)
	  return e.syncLocked()
  }

   func (e *Engine) syncLocked() error {
	  if e == nil || e.sql == nil {
		  return nil
	  }
	  return e.sql.SaveAll(e.mem)
   }

// BindSQL 把目录镜像到 led_shard_* 表。表由迁移创建，这里不建表。
   func (e *Engine) BindSQL(db *persist.SQLDB) error {
	   if e == nil || db == nil || db.DB == nil {
		   return nil
	   }
	   e.mu.Lock()
	   defer e.mu.Unlock()
	   st := &SQLStore{db: db}
	  if err := st.LoadInto(e.mem); err != nil {
		  return err
	  }
	   e.sql = st
	  return e.sql.SaveAll(e.mem)
   }
