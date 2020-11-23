package address

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/validate"
)

var (
	ErrBookTransfer = errors.New("address book transfer invalid")
	ErrBookSameUID = errors.New("address book transfer same uid")
)

  func init() {
	   apierr.Register(ErrBookTransfer, http.StatusBadRequest)
	   apierr.Register(ErrBookSameUID,
		  http.StatusConflict)
  }

// TransferReq 属主转移入参。

type TransferReq struct {
	FromUser string `json:"from_user"`
	ToUser   string `json:"to_user"`
	Chain    string `json:"chain"`
	Address  string `json:"address"`
	Actor string `json:"actor"`
	Reason string `json:"reason,omitempty"`
}

// TransferRec 属主转移回执。

type TransferRec struct {
	From ledger.AddressBookEntry `json:"from"`
	To ledger.AddressBookEntry `json:"to"`
	Actor string `json:"actor"`
	Reason string `json:"reason,omitempty"`
	At time.Time `json:"at"`
}

// TransferBook 运营把簿项从 from 迁到 to。源条停用；目标新增或复用。

   func (s *Service) TransferBook(ctx context.Context, req TransferReq) (TransferRec, error) {
	   if s == nil || s.led == nil {
		  return TransferRec{}, fmt.Errorf("address ledger unavailable")
	  }
	   from, err := validate.UserID(req.FromUser)
	   if err != nil {
		  return TransferRec{}, err
	  }
	   to, err := validate.UserID(req.ToUser)
	  if err != nil {
		  return TransferRec{}, err
	   }
	  if from == to {
		  return TransferRec{}, ErrBookSameUID
	   }
	   actor := strings.TrimSpace(req.Actor)
	  if actor == "" {
		  return TransferRec{}, ErrBookTransfer
	   }
	   if _, err := validate.Actor(actor); err != nil {
		  return TransferRec{}, err
	   }
	  src, ok, err := s.GetBook(ctx, from, req.Chain, req.Address)
	   if err != nil {
		   return TransferRec{}, err
	  }
	  if !ok {
		  return TransferRec{}, ErrBookGone
	   }
	   if err := ownEntry(src, from); err != nil {
		   return TransferRec{}, err
	   }
	  dstWrite := BookWrite{
		  UserID: to, Chain: src.Chain, Asset: src.Asset, Address: src.Address,
		   Label: src.Label, Nickname: src.Label, Memo: src.Memo, Actor: actor,
	  }
	  dst, err := s.AddBookWrite(ctx, dstWrite)
	  if err != nil {
		   return TransferRec{}, err
	   }
	   disabled, err := s.DisableBook(ctx, from, src.Chain, src.Address, actor)
	   if err != nil {
		  return TransferRec{}, err
	   }
	  reason := strings.TrimSpace(req.Reason)
	   s.auditBook(ctx, "book_transfer_from", actor, disabled)
	  s.auditBook(ctx, "book_transfer_to", actor, dst)
	   if s.led != nil {
		  _ = s.led.AppendAudit(ctx, ledger.AuditEvent{
			   EntityType: "address_book",
			  EntityID:   from + "->" + to + "|" + src.Chain + "|" + src.Address,
			   Action: "book_transfer",
			  Actor: actor,
			  Detail: map[string]any{
				  "from_user": from, "to_user": to, "chain": src.Chain,
				   "address": src.Address, "reason": reason,
			   },
			   CreatedAt: time.Now().UTC(),
		  })
	   }
	  return TransferRec{From: disabled, To: dst, Actor: actor, Reason: reason, At: time.Now().UTC()}, nil
   }
