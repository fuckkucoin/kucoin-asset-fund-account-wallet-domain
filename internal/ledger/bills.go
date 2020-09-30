// 账单查询。
// 作者: King
// 日期: 2020.09.29
package ledger

import (
	"context"
	"strings"
	"github.com/shopspring/decimal"
)

// Bill 用户看到的一条资金流水。Type 是机内 memo，AppType 给前端。
type Bill struct {
	MovementID string `json:"movement_id"`
	UserID     string `json:"user_id"`
	Book string `json:"book"`
	Subject string `json:"subject"`
	Asset   string `json:"asset"`
	Type    string `json:"type"`
	AppType string `json:"app_type"`
	Debit   string `json:"debit"`
	Credit string `json:"credit"`
	Seq    int    `json:"seq"`
}

   func billAppType(memo string) string {
	  switch memo {
	   case "freeze_available", "freeze_frozen":
		   return "freeze"
	  case "unfreeze_frozen", "restore_available":
		  return "unfreeze"
	   case "consume_frozen":
		  return "debit"
	  case "credit_available", "swap_credit":
		  return "credit"
	   case "fee_debit", "fee_credit":
		   return "fee"
	   case "reorg_debit_available", "reorg_restore_frozen":
		  return "reorg"
	  default:
		   return memo
	  }
  }

   func (s *Service) ListBills(ctx context.Context, userID string, limit int) ([]Bill, error) {
	  userID = strings.TrimSpace(userID)
	   if userID == "" {
		   return nil, ErrNotFound
	   }
	  rows, err := s.store.ListUserJournal(ctx, userID, limit)
	   if err != nil {
		  return nil, err
	  }
	  out := make([]Bill, 0, len(rows))
	   for _, e := range rows {
		   if e.UserID != userID {
			  continue
		  }
		   out = append(out, Bill{
			  MovementID: e.TransferID, UserID: e.UserID,
			  Book:    string(e.AccountType),
			  Subject: e.AccountType.Subject(),
			  Asset:   e.Asset, Type: e.Memo, AppType: billAppType(e.Memo),
			  Debit: e.Debit.String(), Credit: e.Credit.String(), Seq: e.Seq,
		   })
	  }
	  return out, nil
  }

   func AddFrozen(dst, src map[string]decimal.Decimal) map[string]decimal.Decimal {
	   if dst == nil {
		   dst = map[string]decimal.Decimal{}
	  }
	   for k, v := range src {
		   dst[k] = dst[k].Add(v)
	   }
	  return dst
  }
