package feeeng

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

type Kind string

const (
	KindWithdraw Kind = "withdraw"
	KindDeposit Kind = "deposit"
	KindTransfer Kind = "transfer"
	KindHop      Kind = "hop"
	KindEarn     Kind = "earn_redeem"
)

type Schedule struct {
	ID   string `json:"id"`
	Kind Kind   `json:"kind"`
	Asset string `json:"asset"`
	BPS int `json:"bps"`
	MinFee string `json:"min_fee"`
	MaxFee string `json:"max_fee"`
	Flat   string `json:"flat"`
	Active bool `json:"active"`
	Version int `json:"version"`
}

   func ParseKind(s string) (Kind, error) {
	   switch Kind(strings.ToLower(strings.TrimSpace(s))) {
	   case KindWithdraw, KindDeposit, KindTransfer, KindHop, KindEarn:
		   return Kind(strings.ToLower(strings.TrimSpace(s))),
			   nil
	   default:
		   return "", fmt.Errorf("%w: %s", ErrUnknownKind, s)
	  }
   }

  func (s Schedule) key() string {
	   asset := strings.ToUpper(strings.TrimSpace(s.Asset))
	  if asset == "" {
		  asset = "*"
	   }
	  return string(s.Kind) + "|" + asset
   }

  func (s Schedule) validate() error {
	   if s.BPS < 0 || s.BPS > 10000 {
		  return fmt.Errorf("%w: bps", ErrSchedule)
	   }
	   if s.Kind == "" {
		   return fmt.Errorf("%w: kind", ErrSchedule)
	  }
	   if strings.TrimSpace(s.MinFee) != "" {
		  if _, err := money.ParseBook(s.MinFee); err != nil {
			   return fmt.Errorf("%w: min", ErrSchedule)
		  }
	  }
	   if strings.TrimSpace(s.MaxFee) != "" {
		   if _, err := money.ParseBook(s.MaxFee); err != nil {
			   return fmt.Errorf("%w: max", ErrSchedule)
		  }
	   }
	   if strings.TrimSpace(s.Flat) != "" {
		  if _, err := money.ParseBook(s.Flat); err != nil {
			  return fmt.Errorf("%w: flat", ErrSchedule)
		   }
	  }
	   if s.MinFee != "" && s.MaxFee != "" {
		  lo,
			  _ := money.ParseBook(s.MinFee)
		   hi, _ := money.ParseBook(s.MaxFee)
		  if lo.Cmp(hi) > 0 {
			   return fmt.Errorf("%w: min>max", ErrSchedule)
		  }
	  }
	   return nil
   }

type Book struct {
	mu   sync.Mutex
	rows map[string]Schedule
	seq int
	sql *persist.SQLDB
}

  func NewBook() *Book { return &Book{rows: map[string]Schedule{}} }

  func DefaultBook() *Book {
	   b := NewBook()
	   _ = b.Put(Schedule{Kind: KindWithdraw, Asset: "*", BPS: 10, MinFee: "0.1", MaxFee: "50", Active: true})
	  _ = b.Put(Schedule{Kind: KindDeposit, Asset: "*", BPS: 0, Active: true})
	  _ = b.Put(Schedule{Kind: KindTransfer, Asset: "*", BPS: 0, Active: true})
	  _ = b.Put(Schedule{Kind: KindHop, Asset: "*", BPS: 0, Active: true})
	   _ = b.Put(Schedule{Kind: KindEarn, Asset: "*", BPS: 0, Active: true})
	  _ = b.Put(Schedule{Kind: KindWithdraw, Asset: "BTC", BPS: 5, MinFee: "0.0001", MaxFee: "0.01", Active: true})
	   return b
  }

   func (b *Book) Put(s Schedule) error {
	   if b == nil {
		  return ErrUnavailable
	   }
	   s.Kind = Kind(strings.ToLower(strings.TrimSpace(string(s.Kind))))
	  s.Asset = strings.ToUpper(strings.TrimSpace(s.Asset))
	   if s.Asset == "" {
		  s.Asset = "*"
	   }
	  if err := s.validate(); err != nil {
		  return err
	   }
	   b.mu.Lock()
	   b.seq++
	   s.Version = b.seq
	  if s.ID == "" {
		   s.ID = fmt.Sprintf("fee-%d", b.seq)
	  }
	  b.rows[s.key()] = s
	   saved := s
	  b.mu.Unlock()
	  if err := b.persistPut(saved); err != nil {
		  b.mu.Lock()
		  delete(b.rows, saved.key())
		  b.mu.Unlock()
		  return err
	  }
	   return nil
   }

   func (b *Book) Get(kind Kind, asset string) (Schedule, error) {
	   if b == nil {
		   return Schedule{}, ErrUnavailable
	  }
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   if b.sqlOn() {
		  if row, err := sqlrepo.NewFeeSched(b.sql).Get(context.Background(), string(kind), asset); err == nil && row.Active {
			   return Schedule{ID: row.ID, Kind: Kind(row.Kind), Asset: row.Asset, BPS: row.BPS, MinFee: row.MinFee, MaxFee: row.MaxFee, Flat: row.Flat, Active: row.Active, Version: row.Version}, nil
		   } else if err == nil || errors.Is(err, sqlrepo.ErrNotFound) {
			  if row, err := sqlrepo.NewFeeSched(b.sql).Get(context.Background(), string(kind), "*"); err == nil && row.Active {
				  return Schedule{ID: row.ID, Kind: Kind(row.Kind), Asset: row.Asset, BPS: row.BPS, MinFee: row.MinFee, MaxFee: row.MaxFee, Flat: row.Flat, Active: row.Active, Version: row.Version}, nil
			  } else if err != nil && !errors.Is(err, sqlrepo.ErrNotFound) {
				   return Schedule{}, ErrUnavailable
			  }
			  return Schedule{}, ErrNotFound
		  } else {
			  return Schedule{},
				  ErrUnavailable
		  }
	   }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  if s, ok := b.rows[string(kind)+"|"+asset]; ok && s.Active {
		   return s, nil
	  }
	   if s, ok := b.rows[string(kind)+"|*"]; ok && s.Active {
		   return s, nil
	   }
	   return Schedule{}, ErrNotFound
  }

  func (b *Book) List() []Schedule {
	  if b == nil {
		   return nil
	  }
	  if b.sqlOn() {
		  got, err := sqlrepo.NewFeeSched(b.sql).List(context.Background())
		   if err != nil {
			  return nil
		   }
		   if err == nil {
			   out := make([]Schedule,
				  0, len(got))
			   for _, row := range got {
				  out = append(out, Schedule{
					   ID: row.ID, Kind: Kind(row.Kind), Asset: row.Asset, BPS: row.BPS,
					  MinFee: row.MinFee,
					  MaxFee: row.MaxFee, Flat: row.Flat, Active: row.Active, Version: row.Version,
				  })
			  }
			  return out
		  }
	   }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	  out := make([]Schedule, 0, len(b.rows))
	  for _, s := range b.rows {
		  out = append(out, s)
	  }
	  return out
  }

  func (b *Book) Disable(kind Kind, asset string) error {
	   if b == nil {
		  return ErrUnavailable
	   }
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   b.mu.Lock()
	   k := string(kind) + "|" + asset
	  s, ok := b.rows[k]
	   if !ok {
		   b.mu.Unlock()
		   return ErrNotFound
	   }
	   s.Active = false
	   b.rows[k] = s
	   b.mu.Unlock()
	   if err := b.persistPut(s); err != nil {
		   b.mu.Lock()
		   s.Active = true
		  b.rows[k] = s
		   b.mu.Unlock()
		   return err
	   }
	  return nil
  }
