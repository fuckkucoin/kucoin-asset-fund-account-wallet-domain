package riskpack

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
	"account-assets.kucoin.net/funding/internal/validate"
)

type Cap struct {
	UserID string `json:"user_id,omitempty"`
	Asset  string `json:"asset"`
	Max    string `json:"max"`
}

type Occupancy struct {
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	Used string `json:"used"`
	Max  string `json:"max"`
}

// Book 用户+资产敞口。出金前占位，失败必须 Release，不得把半截占用留着挡下一笔。

type Book struct {
	mu sync.Mutex
	caps map[string]Cap
	used map[string]string
	sql  *persist.SQLDB
}

  func NewBook() *Book {
	  return &Book{caps: map[string]Cap{}, used: map[string]string{}}
  }

   func capKey(userID, asset string) string {
	   return strings.TrimSpace(userID) + "|" + strings.ToUpper(strings.TrimSpace(asset))
  }

  func (b *Book) SetCap(c Cap) error {
	   if b == nil {
		   return ErrUnavailable
	  }
	  asset, err := validate.Asset(c.Asset)
	  if err != nil {
		   return err
	   }
	   if _, err := money.Parse(c.Max); err != nil {
		  return err
	   }
	   c.Asset = asset
	  c.UserID = strings.TrimSpace(c.UserID)
	  b.mu.Lock()
	   prev, had := b.caps[capKey(c.UserID, asset)]
	  b.caps[capKey(c.UserID, asset)] = c
	   b.mu.Unlock()
	  if err := b.persistCap(c); err != nil {
		   b.mu.Lock()
		  if had {
			  b.caps[capKey(c.UserID,
				   asset)] = prev
		  } else {
			  delete(b.caps, capKey(c.UserID, asset))
		  }
		  b.mu.Unlock()
		  return err
	  }
	  return nil
   }

  func (b *Book) CapOf(userID, asset string) (Cap, bool) {
	  if b != nil && b.sqlOn() {
		   if c, err := sqlrepo.NewRiskPack(b.sql).GetCap(context.Background(), userID, asset); err == nil {
			  return Cap{UserID: c.UserID, Asset: c.Asset, Max: c.Max}, true
		   } else if errors.Is(err, sqlrepo.ErrNotFound) {
			   if c, err := sqlrepo.NewRiskPack(b.sql).GetCap(context.Background(), "", asset); err == nil {
				   return Cap{UserID: c.UserID, Asset: c.Asset, Max: c.Max}, true
			   }
		   }
	  }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   if c, ok := b.caps[capKey(userID, asset)]; ok {
		   return c, true
	  }
	  if c, ok := b.caps[capKey("", asset)]; ok {
		   return c, true
	   }
	  return Cap{}, false
   }

// Reserve 占敞口。used+amt 超 cap 返回 ErrCap，used 不变。

   func (b *Book) Reserve(userID, asset, amount string) (Occupancy, error) {
	  if b == nil {
		   return Occupancy{}, ErrUnavailable
	   }
	   uid, err := validate.UserID(userID)
	   if err != nil {
		  return Occupancy{}, err
	  }
	   asset, err = validate.Asset(asset)
	   if err != nil {
		   return Occupancy{}, err
	   }
	   amt, err := money.Parse(amount)
	   if err != nil {
		   return Occupancy{}, err
	  }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  c, ok := b.caps[capKey(uid, asset)]
	  if !ok {
		   c, ok = b.caps[capKey("", asset)]
	   }
	  if !ok {
		  return Occupancy{UserID: uid, Asset: asset, Used: orZero(b.used[capKey(uid, asset)]), Max: ""}, nil
	   }
	   max, err := money.Parse(c.Max)
	   if err != nil {
		  return Occupancy{}, err
	  }
	   cur, err := money.ParseBook(orZero(b.used[capKey(uid, asset)]))
	  if err != nil {
		  return Occupancy{}, err
	   }
	  next, err := cur.Add(amt)
	   if err != nil {
		  return Occupancy{}, err
	  }
	  if next.Cmp(max) > 0 {
		   return Occupancy{UserID: uid, Asset: asset, Used: cur.String(), Max: max.String()}, fmt.Errorf("%w: %s > %s", ErrCap, next, max)
	   }
	  b.used[capKey(uid, asset)] = next.String()
	   return Occupancy{UserID: uid, Asset: asset, Used: next.String(), Max: max.String()}, nil
   }

   func (b *Book) Release(userID, asset, amount string) error {
	   if b == nil {
		  return ErrUnavailable
	  }
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   amt, err := money.Parse(amount)
	   if err != nil {
		   if amt, err = money.ParseBook(amount); err != nil {
			   return err
		  }
	  }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   k := capKey(userID, asset)
	  cur, err := money.ParseBook(orZero(b.used[k]))
	   if err != nil {
		  return err
	   }
	   if cur.Cmp(amt) < 0 {
		   b.used[k] = "0"
		   return nil
	  }
	   next, err := cur.Sub(amt)
	   if err != nil {
		   return err
	  }
	   b.used[k] = next.String()
	  return nil
   }

  func (b *Book) Used(userID, asset string) string {
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  return orZero(b.used[capKey(userID, asset)])
  }

   func orZero(s string) string {
	  if strings.TrimSpace(s) == "" {
		   return "0"
	   }
	   return s
   }
