package hitstore

import (
	"context"
	"encoding/json"
	"errors"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/usage"
)

type Hit struct {
	ID string `json:"id"`
	Kind   usage.Kind   `json:"kind"`
	UserID string       `json:"user_id"`
	Asset  string       `json:"asset"`
	Dest   string       `json:"dest"`
	Amount money.Amount `json:"-"`
	Raw  string    `json:"amount"`
	Idem string    `json:"idem,omitempty"`
	At   time.Time `json:"at"`
}

type Store struct {
	kv    persist.KV
	clock func() time.Time
}

   func New(kv persist.KV) *Store {
	  if kv == nil {
		   kv = persist.NewMemKV()
	  }
	  return &Store{kv: kv, clock: func() time.Time { return time.Now().UTC() }}
  }

  func NewMem() *Store { return New(persist.NewMemKV()) }

  func (s *Store) WithClock(c func() time.Time) *Store {
	   if s != nil && c != nil {
		   s.clock = c
	   }
	   return s
  }

   const kindHit = "usage"

  type bag struct {
	  Hits []Hit `json:"hits"`
   }

   func bagKey(kind usage.Kind, userID, asset string) string {
	  return string(kind) + "|" + userID + "|" + asset
  }

  func (s *Store) load(ctx context.Context, kind usage.Kind, userID, asset string) (bag, persist.Fence, error) {
	   row, err := s.kv.Get(ctx, kindHit, bagKey(kind, userID, asset))
	   if err != nil {
		  return bag{}, persist.Fence{}, err
	  }
	   var b bag
	  if err := json.Unmarshal(row.Body, &b); err != nil {
		  return bag{}, persist.Fence{}, err
	   }
	  for i := range b.Hits {
		   if b.Hits[i].Raw == "" {
			  continue
		   }
		  amt, err := money.ParseBook(b.Hits[i].Raw)
		   if err != nil {
			  return bag{}, persist.Fence{}, err
		  }
		  b.Hits[i].Amount = amt
	  }
	  return b, row.Fence, nil
  }

// Add 记一笔已接受占用。idem 非空则重复写入直接回放，不累加日限额。

  func (s *Store) Add(ctx context.Context, kind usage.Kind, userID, asset, dest, amount, idem string) error {
	   amt, err := money.Parse(amount)
	  if err != nil {
		  return err
	  }
	   return s.kv.InTx(ctx, func(ctx context.Context) error {
		  cur, fence, err := s.load(ctx, kind, userID, asset)
		  if err != nil && !errors.Is(err, persist.ErrNotFound) {
			   return err
		  }
		   expected := persist.Fence{}
		   if err == nil {
			  expected = fence
			  if idem != "" {
				   for _, h := range cur.Hits {
					  if h.Idem == idem {
						  return nil
					   }
				   }
			  }
		  }
		   cur.Hits = append(cur.Hits, Hit{
			  ID: uuid.NewString(), Kind: kind, UserID: userID, Asset: asset,
			   Dest: dest, Amount: amt, Raw: amt.String(), Idem: idem, At: s.clock(),
		  })
		   body, err := json.Marshal(cur)
		   if err != nil {
			  return err
		  }
		   return s.kv.Put(ctx, persist.Row{Kind: kindHit, Key: bagKey(kind, userID, asset), Body: body}, expected)
	  })
  }

  func (s *Store) TodaySum(ctx context.Context, kind usage.Kind, userID, asset string) money.Amount {
	   cur,
		  _, err := s.load(ctx, kind, userID, asset)
	   if err != nil {
		  return money.Zero()
	   }
	  now := s.clock()
	  start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	   sum := money.Zero()
	  for _, h := range cur.Hits {
		   if h.At.Before(start) {
			   continue
		   }
		  var e error
		  sum, e = sum.Add(h.Amount)
		   if e != nil {
			  return sum
		  }
	  }
	  return sum
  }

  func (s *Store) Velocity(ctx context.Context, kind usage.Kind, userID, asset string, win time.Duration) int {
	  if win <= 0 {
		  win = time.Minute
	  }
	   cur, _, err := s.load(ctx, kind, userID, asset)
	  if err != nil {
		   return 0
	   }
	  cut := s.clock().Add(-win)
	   n := 0
	   for _, h := range cur.Hits {
		  if !h.At.Before(cut) {
			   n++
		   }
	  }
	  return n
   }

   func (s *Store) DestCount(ctx context.Context, kind usage.Kind, userID, asset, dest string, win time.Duration) int {
	  if dest == "" {
		   return 0
	   }
	   if win <= 0 {
		   win = 30 * 24 * time.Hour
	  }
	  cur, _, err := s.load(ctx, kind, userID, asset)
	   if err != nil {
		   return 0
	   }
	  cut := s.clock().Add(-win)
	   n := 0
	   for _, h := range cur.Hits {
		   if h.Dest == dest && !h.At.Before(cut) {
			   n++
		   }
	   }
	  return n
  }
