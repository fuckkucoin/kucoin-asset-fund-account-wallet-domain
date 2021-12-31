package xferstore

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/google/uuid"
)

type Rec struct {
	ID string `json:"id"`
	UserID   string `json:"user_id"`
	FromBook string `json:"from_book"`
	ToBook   string `json:"to_book"`
	Asset    string `json:"asset"`
	Amount     string `json:"amount"`
	Idem       string `json:"idem"`
	TransferID string `json:"transfer_id,omitempty"`
	SrcShard   string `json:"src_shard,omitempty"`
	DstShard   string `json:"dst_shard,omitempty"`
	RouteVersion int64 `json:"route_version,omitempty"`
	XShardID  string        `json:"xshard_id,omitempty"`
	SameShard bool          `json:"same_shard,omitempty"`
	Fence     persist.Fence `json:"-"`
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

const kindX = "xfer"

  func (s *Store) load(ctx context.Context, idem string) (Rec, persist.Fence, error) {
	  row,
		  err := s.kv.Get(ctx, kindX, idem)
	  if err != nil {
		  return Rec{}, persist.Fence{}, err
	  }
	  var r Rec
	   if err := json.Unmarshal(row.Body, &r); err != nil {
		   return Rec{},
			  persist.Fence{}, err
	   }
	   r.Fence = row.Fence
	  return r, row.Fence, nil
   }

// Open 同一 idem 必须同一对科目和金额。from==to 直接拒，避免空转吃幂等键。

  func (s *Store) Open(ctx context.Context, userID, from, to, asset, amount, idem string) (Rec, error) {
	   if _, err := money.Parse(amount); err != nil {
		   return Rec{}, err
	  }
	  from = strings.ToLower(strings.TrimSpace(from))
	  to = strings.ToLower(strings.TrimSpace(to))
	  idem = strings.TrimSpace(idem)
	   if from == "" || to == "" || from == to || idem == "" {
		   return Rec{}, persist.ErrConflict
	  }
	   var out Rec
	   err := s.kv.InTx(ctx, func(ctx context.Context) error {
		   cur, _, err := s.load(ctx, idem)
		   if err == nil {
			   if cur.FromBook != from || cur.ToBook != to || cur.Asset != asset || cur.Amount != amount || cur.UserID != userID {
				   return persist.ErrConflict
			   }
			   out = cur
			  return nil
		   }
		   if !errors.Is(err, persist.ErrNotFound) {
			   return err
		   }
		   r := Rec{
			  ID: uuid.NewString(), UserID: userID, FromBook: from, ToBook: to,
			   Asset: strings.ToUpper(asset), Amount: amount, Idem: idem, CreatedAt: time.Now().UTC(),
		  }
		  body, err := json.Marshal(r)
		  if err != nil {
			   return err
		   }
		  if err := s.kv.Put(ctx, persist.Row{Kind: kindX, Key: idem, Body: body}, persist.Fence{}); err != nil {
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

  func (s *Store) BindTransfer(ctx context.Context, idem, transferID string, epoch int64, token string) (Rec, error) {
	   var out Rec
	   err := s.kv.InTx(ctx, func(ctx context.Context) error {
		   cur, fence, err := s.load(ctx, idem)
		  if err != nil {
			  return err
		   }
		  if !fence.Match(epoch, token) {
			  return persist.ErrStaleFence
		  }
		  cur.TransferID = transferID
		  body, err := json.Marshal(cur)
		  if err != nil {
			  return err
		  }
		  if err := s.kv.Put(ctx, persist.Row{Kind: kindX, Key: idem, Body: body}, fence); err != nil {
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

// AnnotateRoute 记下分片路由。同一幂等键若已写下不同分片，拒绝覆盖，避免静默改片。

   func (s *Store) AnnotateRoute(ctx context.Context, idem, src, dst string, ver int64, xshard string, same bool, epoch int64, token string) (Rec, error) {
	  var out Rec
	  err := s.kv.InTx(ctx, func(ctx context.Context) error {
		   cur, fence, err := s.load(ctx, idem)
		   if err != nil {
			  return err
		  }
		   if !fence.Match(epoch, token) {
			   return persist.ErrStaleFence
		  }
		  if cur.SrcShard != "" && (cur.SrcShard != src || cur.DstShard != dst || cur.RouteVersion != ver) {
			  return persist.ErrConflict
		  }
		  cur.SrcShard = src
		  cur.DstShard = dst
		   cur.RouteVersion = ver
		  cur.XShardID = xshard
		   cur.SameShard = same
		  body, err := json.Marshal(cur)
		   if err != nil {
			   return err
		  }
		   if err := s.kv.Put(ctx, persist.Row{Kind: kindX, Key: idem, Body: body}, fence); err != nil {
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
