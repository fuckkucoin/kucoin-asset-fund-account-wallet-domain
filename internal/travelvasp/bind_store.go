package travelvasp

import (
	"context"
	"encoding/json"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindWithdrawBind = "aa_travel_withdraw_bind"

type WithdrawBind struct {
	WithdrawalID string `json:"withdrawal_id"`
	TravelID     string `json:"travel_id"`
	UserID       string `json:"user_id"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	Chain  string `json:"chain"`
	Dest   string `json:"dest"`
	CreatedAt string `json:"created_at"`
}

	  return &BindStore{kv: kv}
   }

  func (s *BindStore) Put(ctx context.Context, b WithdrawBind) error {
	   if s == nil || s.kv == nil {
		   return ErrTravelBind
	   }
	   b.WithdrawalID = strings.TrimSpace(b.WithdrawalID)
	  b.TravelID = strings.TrimSpace(b.TravelID)
	  b.UserID = strings.TrimSpace(b.UserID)
	  if b.WithdrawalID == "" || b.TravelID == "" || b.UserID == "" {
		   return ErrEnvelope
	  }
	   body, err := json.Marshal(b)
	  if err != nil {
		   return err
	   }
	   return s.kv.Put(ctx, persist.Row{Kind: kindWithdrawBind, Key: b.WithdrawalID, Body: body}, persist.Fence{})
   }

  func (s *BindStore) Get(ctx context.Context, withdrawalID string) (WithdrawBind, error) {
	  if s == nil || s.kv == nil {
		  return WithdrawBind{}, ErrTravelBind
	   }
	  withdrawalID = strings.TrimSpace(withdrawalID)
	   if withdrawalID == "" {
		  return WithdrawBind{}, ErrMessage
	  }
	  row,
		  err := s.kv.Get(ctx, kindWithdrawBind, withdrawalID)
	   if err != nil {
		  return WithdrawBind{}, ErrMessage
	   }
	   var b WithdrawBind
	   if json.Unmarshal(row.Body, &b) != nil || b.WithdrawalID == "" {
		  return WithdrawBind{},
			   ErrMessage
	  }
	   return b, nil
   }
