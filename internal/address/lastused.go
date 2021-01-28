package address

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
)

const kindBookLastUsed = "address.book.lastused"

// LastUsedRec 簿项最近出金命中。

type LastUsedRec struct {
	UserID string `json:"user_id"`
	Chain string `json:"chain"`
	Address string    `json:"address"`
	UsedAt  time.Time `json:"used_at"`
	Movement string `json:"movement_id,omitempty"`
}

  func lastUsedKey(userID, chain, addr string) string {
	  return strings.TrimSpace(userID) + "|" + strings.ToUpper(strings.TrimSpace(chain)) + "|" + strings.TrimSpace(addr)
   }

// TouchLastUsed 出金命中簿项后落时间。未精确命中失败关闭。
   func (s *Service) TouchLastUsed(ctx context.Context, userID, chain, addr, movement string) (LastUsedRec, error) {
	   if s == nil || s.led == nil {
		  return LastUsedRec{}, errors.New("address ledger unavailable")
	  }
	  uid, err := validate.UserID(userID)
	  if err != nil {
		  return LastUsedRec{}, err
	   }
	   e, ok, err := s.GetBook(ctx, uid, chain, addr)
	   if err != nil {
		   return LastUsedRec{}, err
	  }
	   if !ok || e.Disabled {
		  return LastUsedRec{}, ErrBookGone
	  }
	   rec := LastUsedRec{
		   UserID: e.UserID, Chain: e.Chain, Address: e.Address,
		   UsedAt: time.Now().UTC(), Movement: strings.TrimSpace(movement),
	  }
	  if s.kv == nil {
		   return rec, nil
	   }
	   if ctx == nil {
		  ctx = context.Background()
	   }
	   body, err := json.Marshal(rec)
	  if err != nil {
		   return LastUsedRec{}, err
	   }
	   key := lastUsedKey(e.UserID, e.Chain, e.Address)
	   _, existed, err := persist.GetOrInit(ctx, s.kv, kindBookLastUsed, key, body)
	   if err != nil {
		   return LastUsedRec{}, err
	   }
	  if existed {
		  if err := persist.Replace(ctx, s.kv, kindBookLastUsed, key, body); err != nil {
			  return LastUsedRec{}, err
		   }
	   }
	   s.auditBook(ctx, "book_last_used", uid, e)
	   return rec, nil
   }

   func (s *Service) lastUsedAt(ctx context.Context, userID, chain, addr string) time.Time {
	   if s == nil || s.kv == nil {
		  return time.Time{}
	  }
	  if ctx == nil {
		  ctx = context.Background()
	  }
	  row, err := s.kv.Get(ctx, kindBookLastUsed, lastUsedKey(userID, chain, addr))
	  if err != nil {
		   return time.Time{}
	   }
	   var rec LastUsedRec
	  if json.Unmarshal(row.Body, &rec) != nil {
		   return time.Time{}
	  }
	  return rec.UsedAt
  }

// InspectLastUsed 读单条 last-used。

   func (s *Service) InspectLastUsed(ctx context.Context, userID, chain, addr string) (LastUsedRec, bool, error) {
	   uid, err := validate.UserID(userID)
	   if err != nil {
		   return LastUsedRec{}, false, err
	   }
	   if s == nil || s.kv == nil {
		   return LastUsedRec{}, false, nil
	   }
	  if ctx == nil {
		  ctx = context.Background()
	  }
	  row, err := s.kv.Get(ctx, kindBookLastUsed, lastUsedKey(uid, chain, addr))
	   if err != nil {
		   if errors.Is(err, persist.ErrNotFound) {
			   return LastUsedRec{}, false, nil
		   }
		  return LastUsedRec{}, false, err
	  }
	   var rec LastUsedRec
	  if json.Unmarshal(row.Body, &rec) != nil {
		   return LastUsedRec{}, false, nil
	   }
	  return rec, !rec.UsedAt.IsZero(), nil
   }
