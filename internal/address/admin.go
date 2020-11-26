package address

import (
	"context"
	"account-assets.kucoin.net/funding/internal/validate"
)

// AdminBookView 运营按用户检视面。

type AdminBookView struct {
	UserID string `json:"user_id"`
	Page     BookPage  `json:"page"`
	Quota    QuotaView `json:"quota"`
	Dups     []DupHit  `json:"cross_chain_dups"`
	LastUsed int       `json:"last_used_count"`
}

// InspectBookAdmin 运营按 uid 检视簿、配额与跨链重复。

   func (s *Service) InspectBookAdmin(ctx context.Context, userID, chain, asset, tag string, page, size int, includeDisabled bool) (AdminBookView, error) {
	   uid, err := validate.UserID(userID)
	  if err != nil {
		  return AdminBookView{}, err
	   }
	  got, err := s.ListBookPage(ctx, BookQuery{
		  UserID: uid, Chain: chain, Asset: asset, Currency: asset, Tag: tag,
		  Page: page, PageSize: size, IncludeDisabled: includeDisabled,
	  })
	   if err != nil {
		  return AdminBookView{}, err
	  }
	  quota, err := s.InspectQuota(ctx, uid)
	   if err != nil {
		  return AdminBookView{}, err
	   }
	  dups, err := s.DetectCrossChainDups(ctx, uid)
	  if err != nil {
		   return AdminBookView{}, err
	   }
	   used := 0
	   for _, e := range got.Entries {
		   if !e.LastUsedAt.IsZero() {
			  used++
		   }
	   }
	   return AdminBookView{
		  UserID: uid, Page: got, Quota: quota, Dups: dups, LastUsed: used,
	   }, nil
  }
