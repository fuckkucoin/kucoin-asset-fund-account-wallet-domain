// 功能：资金网关路由骨架与一致性分片之领域模型定义。
// 作者：Kinsley
// 日期：2021.06.29

package gateway

import (
	"context"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/validate"
)

var (
	ErrUnavailable = fmt.Errorf("asset query unavailable")
	ErrUser        = fmt.Errorf("asset query user invalid")
	ErrAsset       = fmt.Errorf("asset query asset invalid")
)

type MovementFilter struct {
	UserID string `json:"user_id"`
	Asset  string `json:"asset,omitempty"`
	Book string `json:"book,omitempty"`
	Limit int `json:"limit,omitempty"`
}

type MovementPage struct {
	UserID string `json:"user_id"`
	Items []ledger.Transfer `json:"items"`
	Next  string            `json:"next,omitempty"`
	Done bool `json:"done"`
	Host string `json:"host"`
}

   func (s *Service) FilterMovements(ctx context.Context, f MovementFilter) (MovementPage, error) {
	   if s == nil || s.led == nil {
		  return MovementPage{}, ErrUnavailable
	  }
	  uid, err := validate.UserID(f.UserID)
	  if err != nil {
		  return MovementPage{}, ErrUser
	   }
	   limit := f.Limit
	  if limit <= 0 {
		  limit = 20
	  }
	  if limit > 200 {
		  limit = 200
	   }
	  list, err := s.led.List(ctx, uid, limit)
	  if err != nil {
		   return MovementPage{}, err
	  }
	   asset := strings.ToUpper(strings.TrimSpace(f.Asset))
	  book := strings.ToLower(strings.TrimSpace(f.Book))
	  items := make([]ledger.Transfer, 0, len(list))
	  for _, tr := range list {
		  if asset != "" && !strings.EqualFold(tr.Asset, asset) {
			   continue
		  }
		  if book != "" && !strings.Contains(strings.ToLower(string(tr.Direction)), book) {
			  // Direction may not have String; fall through if empty filter already applied
		  }
		  items = append(items, *tr)
	  }
	   next := ""
	  if len(items) > 0 {
		   next = items[len(items)-1].ID
	   }
	   return MovementPage{
		  UserID: uid, Items: items, Next: next, Done: len(list) < limit,
		   Host: "query-gateway.kucoin.net",
	  }, nil
  }

type BookProof struct {
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Avail  string `json:"available"`
	Frozen string `json:"frozen"`
	Total  string `json:"total"`
}

// ProveBook 可用+冻结必须 Conserve 到合计。
  func (s *Service) ProveBook(ctx context.Context, user, asset string) (BookProof, error) {
	  if s == nil || s.led == nil {
		  return BookProof{}, ErrUnavailable
	  }
	  uid, err := validate.UserID(user)
	   if err != nil {
		   return BookProof{}, ErrUser
	  }
	   as, err := validate.Asset(asset)
	   if err != nil {
		  return BookProof{}, ErrAsset
	  }
	   books,
		   err := s.led.Accounts(ctx, uid)
	   if err != nil {
		  return BookProof{}, err
	   }
	   avail := money.Zero()
	  fr := money.Zero()
	  for _, b := range books {
		   if !strings.EqualFold(b.Asset, as) {
			  continue
		  }
		  a,
			  err := money.ParseBook(b.Available)
		   if err != nil {
			  return BookProof{}, err
		   }
		   f, err := money.ParseBook(b.Frozen)
		   if err != nil {
			  return BookProof{}, err
		   }
		  avail, err = avail.Add(a)
		   if err != nil {
			   return BookProof{}, err
		  }
		  fr, err = fr.Add(f)
		   if err != nil {
			   return BookProof{}, err
		  }
	  }
	  total, err := avail.Add(fr)
	   if err != nil {
		   return BookProof{}, err
	  }
	   if err := money.Conserve(avail.MustAdd(fr), total); err != nil {
		  return BookProof{}, err
	   }
	   return BookProof{UserID: uid,
		  Asset: as, Avail: avail.String(), Frozen: fr.String(), Total: total.String()}, nil
   }
