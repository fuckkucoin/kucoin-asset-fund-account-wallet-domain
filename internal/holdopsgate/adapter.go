package holdopsgate

import (
	"context"
	"account-assets.kucoin.net/funding/internal/holdcat"
)

type HoldcatLookup struct {
	Book *holdcat.Book
}

   func (h HoldcatLookup) Lookup(ctx context.Context, userID, tokenID string) (ShadowToken, error) {
	  if h.Book == nil {
		   return ShadowToken{}, ErrUnavailable
	  }
	   row, err := h.Book.Get(ctx, userID, tokenID)
	   if err != nil {
		  return ShadowToken{}, err
	  }
	  return ShadowToken{
		   ID:     row.ID,
		  UserID: row.UserID,
		   Asset:  row.Asset,
		  Amount: row.Amount,
		   Reason: row.Reason,
		   Ref:    row.Ref,
		   Open:   row.Open,
	   }, nil
  }

// CrossCheck 只读核对：影子 token 与 holdcat 行不一致时失败即关，不改 holdcat。

  func CrossCheck(ctx context.Context, book TokenBook, shadow ShadowToken) error {
	  if book == nil {
		  return nil
	   }
	  got,
		  err := book.Lookup(ctx, shadow.UserID, shadow.ID)
	  if err != nil {
		   return err
	   }
	  if got.Open != shadow.Open || got.Amount != shadow.Amount {
		   return ErrLeaseAmount
	  }
	   return nil
   }
