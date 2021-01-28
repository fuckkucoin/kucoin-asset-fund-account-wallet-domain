package addrintel

import (
	"context"
	"strings"
	"sync"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/internal/wallet"
)

// Cluster 一组地址共用标签。同一用户的多条观察可打同一 tag，不是链上图分析。
type Cluster struct {
	Tag string `json:"tag"`
	Addresses []string `json:"addresses"`
}

type Tags struct {
	mu sync.Mutex
	by map[string]string
	rev map[string][]string
	sql *persist.SQLDB
}

   func NewTags() *Tags {
	  return &Tags{by: map[string]string{}, rev: map[string][]string{}}
   }

  func (t *Tags) Tag(chain, addr, tag string) error {
	   if t == nil {
		   return ErrUnavailable
	  }
	   ch, err := validate.Chain(chain)
	   if err != nil {
		   return ErrChain
	  }
	   addr = strings.TrimSpace(addr)
	   if err := wallet.ValidChainAddress(ch, addr); err != nil {
		   return ErrAddress
	   }
	  tag, err = validate.Reason(tag)
	  if err != nil {
		   return ErrTag
	   }
	  k := addrKey(ch, addr)
	  t.mu.Lock()
	   old, had := t.by[k]
	  if had && old != tag {
		   t.dropLocked(old, k)
	   }
	  t.by[k] = tag
	  t.rev[tag] = append(t.rev[tag], k)
	  t.mu.Unlock()
	  if err := t.persistTag(ch, addr, tag); err != nil {
		  t.mu.Lock()
		   t.dropLocked(tag, k)
		  if had {
			   t.by[k] = old
			  t.rev[old] = append(t.rev[old], k)
		  } else {
			   delete(t.by, k)
		  }
		  t.mu.Unlock()
		   return err
	  }
	   return nil
   }

  func (t *Tags) dropLocked(tag, key string) {
	  rows := t.rev[tag]
	   kept := make([]string, 0, len(rows))
	   for _, r := range rows {
		  if r != key {
			   kept = append(kept, r)
		  }
	  }
	  if len(kept) == 0 {
		  delete(t.rev, tag)
		   return
	   }
	   t.rev[tag] = kept
   }

   func (t *Tags) Of(chain, addr string) (string, bool) {
	   if t == nil {
		   return "", false
	  }
	  if t.sqlOn() {
		  row,
			  err := sqlrepo.NewIntel(t.sql).GetTag(context.Background(), chain, addr)
		  if err == nil {
			  return row.Tag, true
		   }
	  }
	  t.mu.Lock()
	  defer t.mu.Unlock()
	  g, ok := t.by[addrKey(chain, addr)]
	  return g, ok
   }

   func (t *Tags) Cluster(tag string) Cluster {
	  if t == nil {
		   return Cluster{Tag: tag}
	   }
	  t.mu.Lock()
	  defer t.mu.Unlock()
	  src := t.rev[strings.TrimSpace(tag)]
	   out := Cluster{Tag: tag, Addresses: make([]string, 0, len(src))}
	   for _, k := range src {
		  parts := strings.SplitN(k, "|", 2)
		  if len(parts) == 2 {
			   out.Addresses = append(out.Addresses, parts[1])
		   }
	   }
	   return out
  }

  func (t *Tags) List() []Cluster {
	  if t == nil {
		  return nil
	   }
	  t.mu.Lock()
	  defer t.mu.Unlock()
	   out := make([]Cluster, 0, len(t.rev))
	   for tag := range t.rev {
		   out = append(out, t.ClusterUnlocked(tag))
	  }
	   return out
  }

   func (t *Tags) ClusterUnlocked(tag string) Cluster {
	  src := t.rev[tag]
	   out := Cluster{Tag: tag, Addresses: make([]string, 0, len(src))}
	   for _, k := range src {
		  parts := strings.SplitN(k, "|", 2)
		  if len(parts) == 2 {
			  out.Addresses = append(out.Addresses, parts[1])
		  }
	   }
	   return out
  }
