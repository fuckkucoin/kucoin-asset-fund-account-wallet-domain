package address

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/persist"
)

const (
	kindBookQuota = "address.book.quota"
	defaultQuota  = 200
)

var ErrBookQuota = errors.New("address book quota exceeded")

   func init() {
	   apierr.Register(ErrBookQuota,
		  http.StatusConflict)
   }

// QuotaView 用户簿配额面。
type QuotaView struct {
	UserID string `json:"user_id"`
	Used   int `json:"used"`
	Limit  int `json:"limit"`
	Remain int `json:"remain"`
	Reached bool `json:"reached"`
}

  func (s *Service) bookQuotaLimit(ctx context.Context, userID string) int {
	   if s == nil || s.kv == nil {
		  return defaultQuota
	  }
	  if ctx == nil {
		   ctx = context.Background()
	  }
	  key := strings.TrimSpace(userID)
	  if key == "" {
		  key = "default"
	   }
	   row, err := s.kv.Get(ctx, kindBookQuota, key)
	   if err != nil {
		   if errors.Is(err, persist.ErrNotFound) {
			  if row, err = s.kv.Get(ctx, kindBookQuota, "default"); err != nil {
				  return defaultQuota
			   }
		   } else {
			   return defaultQuota
		  }
	  }
	   n, perr := strconv.Atoi(strings.TrimSpace(string(row.Body)))
	   if perr != nil {
		   var wrap struct {
			  Limit int `json:"limit"`
		   }
		  if json.Unmarshal(row.Body, &wrap) == nil && wrap.Limit > 0 {
			  return wrap.Limit
		  }
		   return defaultQuota
	  }
	   if n <= 0 {
		   return defaultQuota
	   }
	   return n
   }

// InspectQuota 读用户簿配额。used 仅计未停用条。

  func (s *Service) InspectQuota(ctx context.Context, userID string) (QuotaView, error) {
	   n, err := s.BookCount(ctx, userID, "")
	   if err != nil {
		   return QuotaView{}, err
	  }
	  lim := s.bookQuotaLimit(ctx, userID)
	  remain := lim - n
	   if remain < 0 {
		   remain = 0
	  }
	  return QuotaView{UserID: strings.TrimSpace(userID), Used: n, Limit: lim, Remain: remain, Reached: n >= lim}, nil
  }

   func (s *Service) checkBookQuota(ctx context.Context, userID string) error {
	   view, err := s.InspectQuota(ctx, userID)
	  if err != nil {
		   return err
	  }
	  if view.Reached {
		   return ErrBookQuota
	  }
	   return nil
  }

// PersistQuota 运营热写每用户或默认配额。
  func (s *Service) PersistQuota(ctx context.Context, userID string, limit int) error {
	   if s == nil || s.kv == nil {
		   return nil
	   }
	   if limit <= 0 {
		   limit = defaultQuota
	   }
	   key := strings.TrimSpace(userID)
	  if key == "" {
		   key = "default"
	   }
	   body, err := json.Marshal(struct {
		   Limit int `json:"limit"`
	   }{Limit: limit})
	  if err != nil {
		  return err
	   }
	   if ctx == nil {
		  ctx = context.Background()
	   }
	   _, existed, err := persist.GetOrInit(ctx, s.kv, kindBookQuota, key, body)
	   if err != nil {
		  return err
	  }
	   if existed {
		  return persist.Replace(ctx, s.kv, kindBookQuota, key, body)
	  }
	  return nil
   }
