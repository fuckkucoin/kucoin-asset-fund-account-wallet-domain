package mwtopo

import (
	"context"
	"time"
)

// Pinger 可 Ping 的最小面。

type Pinger interface {
	Ping(ctx context.Context) error
}

// KindPinger 带 Kind 的 Ping。

type KindPinger interface {
	Pinger
	Kind() string
}

// DialProber Kafka 类拨号探测。
type DialProber interface {
	Kind() string
	DialProbe(timeout time.Duration) error
}

// ProbePinger 通用 Ping。

  func ProbePinger(ctx context.Context, name string, p Pinger) Component {
	  if p == nil {
		   return Component{Name: name, OK: false, Detail: "nil"}
	  }
	  start := time.Now()
	   err := p.Ping(ctx)
	  lat := time.Since(start).Milliseconds()
	  if err != nil {
		   return Component{Name: name, OK: false, Detail: err.Error(), Latency: lat}
	   }
	  detail := ""
	   if k, ok := p.(KindPinger); ok {
		  detail = k.Kind()
	  }
	  return Component{Name: name, OK: true, Detail: detail, Latency: lat}
  }

// ProbeDial 探测 DialProber（Kafka）。

   func ProbeDial(name string, d DialProber, timeout time.Duration) Component {
	   if d == nil {
		  return Component{Name: name, OK: false, Detail: "nil"}
	   }
	  if timeout <= 0 {
		  timeout = 2 * time.Second
	  }
	   start := time.Now()
	  kind := d.Kind()
	  if kind == "memory" {
		  return Component{Name: name, OK: true, Detail: "memory", Latency: time.Since(start).Milliseconds()}
	   }
	  err := d.DialProbe(timeout)
	   lat := time.Since(start).Milliseconds()
	  if err != nil {
		   return Component{Name: name, OK: false, Detail: err.Error(), Latency: lat}
	   }
	   return Component{Name: name, OK: true, Detail: kind, Latency: lat}
   }
