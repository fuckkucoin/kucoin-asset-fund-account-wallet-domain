package addrintel

import (
	"bufio"
	"context"
	"os"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/internal/wallet"
)

// List 制裁名单。精确匹配，大小写不敏感。缺文件或空行跳过，命中失败即关。以 SQL 为权威。

type List struct {
	mu   sync.Mutex
	rows map[string]struct{}
	hook UpdateHook
	gen int
	mtime time.Time
	sql   *persist.SQLDB
}

   func NewList() *List { return &List{rows: map[string]struct{}{}} }

   func (l *List) Add(chain, addr string) error {
	  if l == nil {
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
	   k := addrKey(ch, addr)
	  l.mu.Lock()
	  l.rows[k] = struct{}{}
	   l.mu.Unlock()
	   if err := l.persistAdd(ch, addr); err != nil {
		  l.mu.Lock()
		   delete(l.rows, k)
		   l.mu.Unlock()
		  return err
	  }
	   return nil
  }

   func (l *List) Has(chain, addr string) bool {
	  if l == nil {
		   return false
	   }
	   if l.sqlOn() {
		  ok, err := sqlrepo.NewSanctions(l.sql).Has(context.Background(), chain, addr)
		  if err != nil {
			   return false
		  }
		  return ok
	  }
	  l.mu.Lock()
	  defer l.mu.Unlock()
	   _, ok := l.rows[addrKey(chain, addr)]
	  return ok
   }

   func (l *List) Check(chain, addr string) error {
	  if l == nil {
		   return ErrUnavailable
	   }
	  if l.sqlOn() {
		  ok, err := sqlrepo.NewSanctions(l.sql).Has(context.Background(), chain, addr)
		   if err != nil {
			   return ErrUnavailable
		  }
		  if ok {
			   return ErrSanction
		  }
		  return nil
	  }
	  if l.Has(chain, addr) {
		  return ErrSanction
	   }
	   return nil
  }

   func (l *List) Len() int {
	   if l == nil {
		   return 0
	   }
	  l.mu.Lock()
	  defer l.mu.Unlock()
	   return len(l.rows)
  }

// LoadFile 读进程内名单。每行 chain address；# 开头当注释。坏行整文件拒，不得半加载。
  func (l *List) LoadFile(path string) error {
	   if l == nil {
		  return ErrUnavailable
	   }
	  f, err := os.Open(path)
	   if err != nil {
		   return ErrList
	  }
	  defer f.Close()
	  sc := bufio.NewScanner(f)
	   pending := make([][2]string, 0)
	   for sc.Scan() {
		  line := strings.TrimSpace(sc.Text())
		   if line == "" || strings.HasPrefix(line, "#") {
			   continue
		   }
		  parts := strings.Fields(line)
		  if len(parts) < 2 {
			  return ErrList
		   }
		  pending = append(pending, [2]string{parts[0], parts[1]})
	   }
	   if err := sc.Err(); err != nil {
		  return ErrList
	   }
	  for _, p := range pending {
		   if err := l.Add(p[0], p[1]); err != nil {
			  return err
		  }
	   }
	  return nil
  }

  func (l *List) Keys() []string {
	  if l == nil {
		   return nil
	  }
	   l.mu.Lock()
	   defer l.mu.Unlock()
	  out := make([]string, 0, len(l.rows))
	  for k := range l.rows {
		  out = append(out, k)
	  }
	   return out
  }
