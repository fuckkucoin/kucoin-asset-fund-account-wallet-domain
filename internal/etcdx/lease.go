package etcdx

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrEndpoints = errors.New("etcd endpoints required")
	ErrLeaseTTL  = errors.New("etcd lease ttl invalid")
)

// EndpointSet SG/TYO etcd 地址集。
type EndpointSet struct {
	Region string // sg | tyo
	Endpoints []string
}

   func (e EndpointSet) Normalize() (EndpointSet, error) {
	   out := e
	   out.Region = strings.ToLower(strings.TrimSpace(e.Region))
	   if out.Region == "" {
		  out.Region = "sg"
	   }
	   clean := make([]string,
		  0, len(e.Endpoints))
	   for _, ep := range e.Endpoints {
		   ep = strings.TrimRight(strings.TrimSpace(ep), "/")
		   if ep != "" {
			   clean = append(clean, ep)
		  }
	  }
	  out.Endpoints = clean
	   if len(out.Endpoints) == 0 {
		  return out, ErrEndpoints
	   }
	  return out, nil
   }

// LeasePolicy 租约策略。
type LeasePolicy struct {
	TTL time.Duration
	KeepEvery time.Duration
	KeyPrefix string
}

  func (p LeasePolicy) Normalize() (LeasePolicy, error) {
	  out := p
	  if out.TTL <= 0 {
		  out.TTL = 10 * time.Second
	  }
	   if out.TTL < 3*time.Second {
		  return out, ErrLeaseTTL
	   }
	   if out.KeepEvery <= 0 {
		  out.KeepEvery = out.TTL / 3
	  }
	   if out.KeyPrefix == "" {
		  out.KeyPrefix = "/aa/svc"
	  }
	  return out, nil
  }

// DefaultSG 新加坡 etcd 默认入口。

  func DefaultSG() EndpointSet {
	  return EndpointSet{Region: "sg", Endpoints: CatalogHostSG()}
  }

// DefaultTYO 东京备。

   func DefaultTYO() EndpointSet {
	   return EndpointSet{Region: "tyo", Endpoints: CatalogHostTYO()}
   }

// PickNext 简单轮询。

  func PickNext(eps []string, i int) (string, int, error) {
	   if len(eps) == 0 {
		  return "", i, ErrEndpoints
	   }
	   if i < 0 {
		   i = 0
	  }
	   idx := i % len(eps)
	   return eps[idx], idx + 1, nil
  }

// FormatInstanceKey 注册键。

   func FormatInstanceKey(prefix, name, addr string) string {
	   prefix = strings.TrimRight(prefix, "/")
	  name = strings.Trim(name, "/")
	  addr = strings.TrimSpace(addr)
	   return fmt.Sprintf("%s/%s/%s", prefix, name, addr)
   }
