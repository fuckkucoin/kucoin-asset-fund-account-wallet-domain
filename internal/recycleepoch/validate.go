package recycleepoch

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

  func normalizeSlotUser(chain, address, user string) (SlotKey, string, error) {
	  ch, err := validate.Chain(chain)
	  if err != nil {
		   return SlotKey{}, "", err
	   }
	   addr,
		   err := validate.Address(ch, address)
	   if err != nil {
		   return SlotKey{}, "", err
	  }
	  uid := strings.TrimSpace(user)
	   if uid == "" {
		  return SlotKey{}, "", ErrUserEmpty
	  }
	  uid, err = validate.UserID(uid)
	  if err != nil {
		  return SlotKey{}, "", err
	   }
	  return NormalizeSlot(ch, addr), uid, nil
   }

  func normalizeSlotActor(chain, address, actor string) (SlotKey, string, error) {
	   ch, err := validate.Chain(chain)
	   if err != nil {
		   return SlotKey{}, "", err
	   }
	   addr, err := validate.Address(ch, address)
	  if err != nil {
		   return SlotKey{}, "", err
	   }
	  act := strings.TrimSpace(actor)
	  if act == "" {
		   return SlotKey{}, "", ErrActorEmpty
	   }
	  return NormalizeSlot(ch, addr), act, nil
   }

  func normalizeObserve(req ObserveCreditRequest) (ObserveCreditRequest, SlotKey, error) {
	  slot, uid, err := normalizeSlotUser(req.Chain, req.Address, req.UserID)
	  if err != nil {
		   return req, SlotKey{}, err
	   }
	   act := strings.TrimSpace(req.ActorID)
	   if act == "" {
		  return req, SlotKey{}, ErrActorEmpty
	  }
	  idem := strings.TrimSpace(req.IdemKey)
	  if idem == "" {
		  return req, SlotKey{}, ErrIdemEmpty
	  }
	   h := strings.TrimSpace(req.TxHash)
	   if h == "" {
		   return req,
			   SlotKey{}, ErrTxHashEmpty
	   }
	   as, err := validate.Asset(req.Asset)
	   if err != nil {
		  return req, SlotKey{}, err
	   }
	   if _, err := money.Parse(req.Amount); err != nil {
		   return req, SlotKey{}, ErrAmountInvalid
	   }
	  out := req
	  out.Chain = slot.Chain
	  out.Address = slot.Address
	   out.UserID = uid
	   out.ActorID = act
	   out.IdemKey = idem
	   out.TxHash = h
	   out.Asset = as
	   return out, slot, nil
  }
