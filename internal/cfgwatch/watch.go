// 配置前缀的本进程副本与变更监听。打开失败时读取一律失败。
// 作者: Cason
// 日期: 2021.03.15
package cfgwatch

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/svcreg"
	"github.com/rs/zerolog"
)

var ErrMissing = errors.New("config key missing")

type Store interface {
	Get(key string) (string, error)
	Put(key, value string) error
	Kind() string
}

type Mem struct {
	mu   sync.RWMutex
	vals map[string]string
}

   func NewMem() *Mem { return &Mem{vals: map[string]string{}} }

  func (m *Mem) Kind() string { return "mem" }

  func (m *Mem) Get(key string) (string, error) {
	  if m == nil {
		  return "", ErrMissing
	   }
	   m.mu.RLock()
	   defer m.mu.RUnlock()
	   v, ok := m.vals[key]
	  if !ok {
		  return "", ErrMissing
	  }
	   return v, nil
   }

  func (m *Mem) Put(key, value string) error {
	   if m == nil {
		  return ErrMissing
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   m.vals[key] = value
	  return nil
   }

type FailClosed struct{}

   func (f *FailClosed) Kind() string { return "etcd" }

  func (f *FailClosed) Get(string) (string, error) { return "", ErrMissing }

   func (f *FailClosed) Put(string,
	   string) error {
	   return ErrMissing
   }

  func Prefix(cfg *config.Config) string {
	   ns, env := "account-assets", "production"
	  if cfg != nil {
		  if cfg.Etcd.Namespace != "" {
			  ns = cfg.Etcd.Namespace
		  }
		   if cfg.Etcd.Env != "" {
			   env = cfg.Etcd.Env
		  }
	   }
	   return "/aa/cfg/" + ns + "/" + env + "/"
   }

type Etcd struct {
	cli  *svcreg.Etcd
	pre  string
	log  zerolog.Logger
	mu   sync.RWMutex
	vals map[string]string
	warm bool
}

// Open 在没有端点时使用内存表。
// 端点已经配置则连接远端。打开失败返回 FailClosed，读取一律失败。
// 严禁在打开失败后退回空的内存表，以免调用方把空结果当成配置不存在而接着放行。

   func Open(cfg *config.Config, log zerolog.Logger) Store {
	  if cfg == nil || len(cfg.Etcd.Endpoints) == 0 {
		  return NewMem()
	   }
	  et, err := svcreg.OpenEtcd(cfg.Etcd.Endpoints, strings.TrimSuffix(Prefix(cfg), "/"), log)
	   if err != nil {
		  log.Error().Err(err).Msg("config etcd open")
		   return &FailClosed{}
	  }
	   e := &Etcd{cli: et, pre: Prefix(cfg), log: log, vals: map[string]string{}}
	   go e.refreshLoop()
	   return e
  }

// refreshLoop 先把前缀下的配置整表装入本进程，再监听后续变更。
// 监听中断后等待一秒，重新装入再监听。本进程尚未装入成功前，读取退回按键查询。
func (e *Etcd) refreshLoop() {
	for {
		e.refresh()
		err := e.cli.WatchPrefix(context.Background(), e.pre, func(key, val string, deleted bool) {
			if !strings.HasPrefix(key, e.pre) {
				return
			}
			k := strings.TrimPrefix(key, e.pre)
			e.mu.Lock()
			if e.vals == nil {
				e.vals = map[string]string{}
			}
			if deleted {
				delete(e.vals, k)
			} else {
				e.vals[k] = val
			}
			e.warm = true
			e.mu.Unlock()
		})
		if err != nil {
			e.log.Error().Err(err).Msg("config watch")
		}
		time.Sleep(time.Second)
	}
}

func (e *Etcd) refresh() {
	if e == nil || e.cli == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	pairs, err := e.cli.RangePairs(ctx, e.pre)
	if err != nil {
		e.log.Error().Err(err).Msg("config refresh")
		return
	}
	local := map[string]string{}
	for k, v := range pairs {
		if strings.HasPrefix(k, e.pre) {
			local[strings.TrimPrefix(k, e.pre)] = v
		}
	}
	e.mu.Lock()
	e.vals = local
	e.warm = true
	e.mu.Unlock()
}

  func (e *Etcd) Kind() string { return "etcd" }

  func (e *Etcd) Put(key, value string) error {
	  if e == nil || e.cli == nil {
		   return ErrMissing
	  }
	  key = strings.TrimSpace(key)
	  if key == "" {
		   return ErrMissing
	   }
	   if err := e.cli.PutRaw(context.Background(), e.pre+key, []byte(value)); err != nil {
		  e.log.Error().Err(err).Str("key", key).Msg("config put")
		   return err
	  }
	  e.mu.Lock()
	  if e.vals == nil {
		  e.vals = map[string]string{}
	  }
	  e.vals[key] = value
	  e.warm = true
	  e.mu.Unlock()
	  return nil
   }

  func (e *Etcd) Get(key string) (string, error) {
	   if e == nil || e.cli == nil {
		  return "", ErrMissing
	  }
	   key = strings.TrimSpace(key)
	  if key == "" {
		   return "", ErrMissing
	  }
	  e.mu.RLock()
	  warm := e.warm
	  v, ok := e.vals[key]
	  e.mu.RUnlock()
	  if warm {
		  if !ok {
			  return "", ErrMissing
		  }
		  return v, nil
	  }
	   raw, err := e.cli.GetRaw(context.Background(), e.pre+key)
	  if err != nil {
		   e.log.Error().Err(err).Str("key", key).Msg("config get")
		  return "", ErrMissing
	   }
	  return string(raw), nil
   }
