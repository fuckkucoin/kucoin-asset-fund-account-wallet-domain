package wdstore

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Rec struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Chain string `json:"chain"`
	Dest       string        `json:"dest"`
	Idem       string        `json:"idem"`
	State      string        `json:"state"`
	TransferID string        `json:"transfer_id,omitempty"`
	Fence      persist.Fence `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

type Store struct {
	kv persist.KV
}

  func New(kv persist.KV) *Store {
	   if kv == nil {
		  kv = persist.NewMemKV()
	   }
	   return &Store{kv: kv}
  }

  func NewMem() *Store { return New(nil) }

const kindWD = "withdraw"

   func (s *Store) load(ctx context.Context, idem string) (Rec, persist.Fence, error) {
	  row, err := s.kv.Get(ctx, kindWD, idem)
	   if err != nil {
		   return Rec{}, persist.Fence{}, err
	   }
	  var r Rec
	  if err := json.Unmarshal(row.Body, &r); err != nil {
		  return Rec{}, persist.Fence{}, err
	  }
	  r.Fence = row.Fence
	   return r, row.Fence, nil
   }

// Open 登记一笔出金意图。金额必须过 money.Parse。同一 idem 回放，payload 不一致冲突。
  func (s *Store) Open(ctx context.Context, userID, asset, amount, chain, dest, idem string) (Rec, error) {
	   if _, err := money.Parse(amount); err != nil {
		   return Rec{}, err
	   }
	  userID = strings.TrimSpace(userID)
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	   dest = strings.TrimSpace(dest)
	  idem = strings.TrimSpace(idem)
	   if userID == "" || dest == "" || idem == "" {
		   return Rec{}, persist.ErrConflict
	   }
	   var out Rec
	  err := s.kv.InTx(ctx, func(ctx context.Context) error {
		   cur, _, err := s.load(ctx, idem)
		   if err == nil {
			  if cur.UserID != userID || cur.Asset != asset || cur.Amount != amount || cur.Chain != chain || cur.Dest != dest {
				   return persist.ErrConflict
			   }
			   out = cur
			   return nil
		   }
		   if !errors.Is(err, persist.ErrNotFound) {
			   return err
		   }
		   r := Rec{
			   ID:     uuid.NewString(),
			   UserID: userID, Asset: asset, Amount: amount,
			   Chain: chain, Dest: dest, Idem: idem, State: "requested", CreatedAt: time.Now().UTC(),
		   }
		   body, err := json.Marshal(r)
		  if err != nil {
			   return err
		  }
		  if err := s.kv.Put(ctx, persist.Row{Kind: kindWD, Key: idem, Body: body}, persist.Fence{}); err != nil {
			   return err
		  }
		   got, _, err := s.load(ctx, idem)
		   if err != nil {
			   return err
		   }
		   out = got
		   return nil
	   })
	   return out, err
   }

// SetState 必须带当前围栏。已 credited 不得改回 requested，避免解冻已广播单。

   func (s *Store) SetState(ctx context.Context, idem, state, transferID string, epoch int64, token string) (Rec, error) {
	   var out Rec
	   err := s.kv.InTx(ctx, func(ctx context.Context) error {
		  cur, fence, err := s.load(ctx, idem)
		  if err != nil {
			  return err
		   }
		   if !fence.Match(epoch, token) {
			   return persist.ErrStaleFence
		   }
		   if cur.State == "credited" && state != "credited" {
			  return persist.ErrConflict
		  }
		  cur.State = state
		   if transferID != "" {
			  cur.TransferID = transferID
		  }
		  body, err := json.Marshal(cur)
		  if err != nil {
			  return err
		   }
		  if err := s.kv.Put(ctx, persist.Row{Kind: kindWD, Key: idem, Body: body}, fence); err != nil {
			  return err
		  }
		  got, _, err := s.load(ctx, idem)
		   if err != nil {
			   return err
		   }
		  out = got
		   return nil
	  })
	  return out, err
   }

  func (s *Store) Get(ctx context.Context, idem string) (Rec, error) {
	  r, _, err := s.load(ctx, idem)
	  return r, err
  }
