package dephash

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Claim struct {
	Chain  string `json:"chain"`
	TxHash string `json:"tx_hash"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
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

  func claimKey(chain, hash string) string {
	   return strings.ToUpper(strings.TrimSpace(chain)) + "|" + strings.TrimSpace(hash)
  }

var ErrTaken = errors.New("deposit hash already claimed")

// Claim 首次认领。已存在且用户/资产一致则回放；用户不同返回 ErrTaken。
  func (s *Store) Claim(ctx context.Context, chain, hash, userID, asset, amount, transferID string) (Claim, error) {
	   if _, err := money.Parse(amount); err != nil {
		   return Claim{}, err
	   }
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	   hash = strings.TrimSpace(hash)
	  if chain == "" || hash == "" || userID == "" {
		   return Claim{}, persist.ErrConflict
	  }
	  var out Claim
	   err := s.kv.InTx(ctx, func(ctx context.Context) error {
		  row, err := s.kv.Get(ctx, "deposit", claimKey(chain, hash))
		   if err == nil {
			  var cur Claim
			  if err := json.Unmarshal(row.Body, &cur); err != nil {
				  return err
			   }
			  if cur.UserID != userID || cur.Asset != asset {
				   return ErrTaken
			   }
			  cur.Fence = row.Fence
			   out = cur
			  return nil
		  }
		   if !errors.Is(err, persist.ErrNotFound) {
			  return err
		   }
		  c := Claim{
			   Chain: chain, TxHash: hash, UserID: userID, Asset: asset,
			  Amount: amount, TransferID: transferID, CreatedAt: time.Now().UTC(),
		  }
		  body, err := json.Marshal(c)
		   if err != nil {
			   return err
		  }
		   if err := s.kv.Put(ctx, persist.Row{Kind: "deposit", Key: claimKey(chain, hash), Body: body}, persist.Fence{}); err != nil {
			  return err
		  }
		   got, err := s.kv.Get(ctx, "deposit", claimKey(chain, hash))
		  if err != nil {
			  return err
		   }
		   if err := json.Unmarshal(got.Body, &c); err != nil {
			   return err
		   }
		   c.Fence = got.Fence
		   out = c
		   return nil
	   })
	   return out, err
  }

   func (s *Store) Get(ctx context.Context, chain, hash string) (Claim, error) {
	  row, err := s.kv.Get(ctx, "deposit", claimKey(chain, hash))
	   if err != nil {
		   return Claim{}, err
	  }
	  var c Claim
	   if err := json.Unmarshal(row.Body, &c); err != nil {
		   return Claim{}, err
	   }
	  c.Fence = row.Fence
	  return c, nil
  }
