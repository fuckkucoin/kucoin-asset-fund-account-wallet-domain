package polystore

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/policypack"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

type Rec struct {
	Pack    string `json:"pack"`
	Version int    `json:"version"`
	Body        []byte    `json:"body"`
	PublishedAt time.Time `json:"published_at"`
}

type Store struct {
	kv  persist.KV
	sql *persist.SQLDB
}

   func New(kv persist.KV) *Store {
	  if kv == nil {
		  kv = persist.NewMemKV()
	   }
	  return &Store{kv: kv}
  }

  func NewMem() *Store { return New(nil) }

  func key(pack string, ver int) string {
	  return strings.ToLower(strings.TrimSpace(pack)) + "|" + itoa(ver)
   }

   func itoa(n int) string {
	  if n == 0 {
		  return "0"
	  }
	   neg := n < 0
	   if neg {
		   n = -n
	   }
	   var b [12]byte
	   i := len(b)
	   for n > 0 {
		  i--
		   b[i] = byte('0' + n%10)
		   n /= 10
	  }
	  if neg {
		   i--
		  b[i] = '-'
	   }
	  return string(b[i:])
   }

// Publish 同一 pack+version 已存在则冲突。正文必须能解成 Pack 且 Version 对得上。

   func (s *Store) Publish(ctx context.Context, p policypack.Pack) error {
	  if p.Name == "" || p.Doc.Version <= 0 {
		  return persist.ErrConflict
	   }
	  body, err := json.Marshal(p)
	   if err != nil {
		  return err
	   }
	   rec := Rec{Pack: p.Name,
		  Version: p.Doc.Version, Body: body, PublishedAt: time.Now().UTC()}
	  raw, err := json.Marshal(rec)
	   if err != nil {
		   return err
	   }
	   if s.sqlOn() {
		   if _, err := s.getSQL(ctx, p.Name, p.Doc.Version); err == nil {
			   return persist.ErrConflict
		   } else if !ignoreSQLMiss(err) {
			  return err
		   }
		  if err := s.publishSQL(ctx, p, body); err != nil {
			  if errors.Is(err, sqlrepo.ErrConflict) {
				  return persist.ErrConflict
			  }
			  return err
		  }
	  }
	   _, err = s.kv.Get(ctx, "policy", key(p.Name, p.Doc.Version))
	   if err == nil {
		  return persist.ErrConflict
	   }
	  if !errors.Is(err, persist.ErrNotFound) {
		   return err
	   }
	  return s.kv.Put(ctx, persist.Row{Kind: "policy", Key: key(p.Name, p.Doc.Version), Body: raw}, persist.Fence{})
   }

   func (s *Store) Get(ctx context.Context, pack string, ver int) (policypack.Pack, error) {
	  if s.sqlOn() {
		  got, err := s.getSQL(ctx, pack, ver)
		   if err == nil {
			  return got, nil
		   }
		  if !ignoreSQLMiss(err) {
			  return policypack.Pack{}, err
		  }
	   }
	  row, err := s.kv.Get(ctx, "policy", key(pack, ver))
	   if err != nil {
		  return policypack.Pack{}, err
	  }
	  var rec Rec
	   if err := json.Unmarshal(row.Body, &rec); err != nil {
		  return policypack.Pack{}, err
	  }
	   var p policypack.Pack
	   if err := json.Unmarshal(rec.Body, &p); err != nil {
		   return policypack.Pack{}, err
	   }
	  return p, nil
  }

   func (s *Store) Latest(ctx context.Context, pack string) (policypack.Pack, error) {
	   if s.sqlOn() {
		  got, err := s.latestSQL(ctx, pack)
		  if err == nil {
			  return got, nil
		   }
		   if !ignoreSQLMiss(err) {
			  return policypack.Pack{}, err
		  }
	   }
	  rows, err := s.kv.List(ctx, "policy", 200)
	  if err != nil {
		   return policypack.Pack{}, err
	   }
	   pack = strings.ToLower(strings.TrimSpace(pack))
	  bestV := 0
	   var best persist.Row
	  found := false
	  for _, r := range rows {
		   if !strings.HasPrefix(r.Key, pack+"|") {
			  continue
		   }
		   var rec Rec
		   if err := json.Unmarshal(r.Body, &rec); err != nil {
			  continue
		   }
		  if rec.Version >= bestV {
			  bestV = rec.Version
			   best = r
			  found = true
		  }
	  }
	   if !found {
		  return policypack.Pack{}, persist.ErrNotFound
	  }
	   var rec Rec
	   if err := json.Unmarshal(best.Body, &rec); err != nil {
		  return policypack.Pack{}, err
	   }
	  var p policypack.Pack
	   if err := json.Unmarshal(rec.Body, &p); err != nil {
		  return policypack.Pack{}, err
	   }
	  return p, nil
  }

  func SeedCatalog(ctx context.Context, s *Store, c *policypack.Catalog) error {
	   if s == nil || c == nil {
		   return persist.ErrTxClosed
	   }
	   for _, name := range c.Names() {
		   p, err := c.Get(name)
		  if err != nil {
			  return err
		   }
		  if err := s.Publish(ctx, p); err != nil && !errors.Is(err, persist.ErrConflict) {
			   return err
		  }
	  }
	  return nil
   }
