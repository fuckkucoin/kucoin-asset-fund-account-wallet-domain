package address

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
)

const (
	kindBookRate = "address.book.rate"
	kindBookIdem = "address.book.idem"
	bookRateMax  = 20
	bookRateWin = time.Minute
)

var (
	ErrBookRate = errors.New("address book add rate limited")
	ErrBookIdem = errors.New("address book idempotency conflict")
)

  func init() {
	  apierr.Register(ErrBookRate, http.StatusTooManyRequests)
	   apierr.Register(ErrBookIdem, http.StatusConflict)
   }

type rateWin struct {
	N int `json:"n"`
	Win time.Time `json:"win"`
}

type idemRec struct {
	UserID string `json:"user_id"`
	Chain   string `json:"chain"`
	Address string `json:"address"`
}

   func (s *Service) checkBookRate(ctx context.Context, userID string, now time.Time) error {
	   if s == nil || s.kv == nil {
		  return nil
	  }
	   key := strings.TrimSpace(userID)
	  row, err := s.kv.Get(ctx, kindBookRate, key)
	   cur := rateWin{Win: now}
	   missing := errors.Is(err, persist.ErrNotFound)
	   if err == nil {
		   _ = json.Unmarshal(row.Body, &cur)
		   if now.Sub(cur.Win) >= bookRateWin {
			  cur = rateWin{Win: now}
		  }
	  } else if !missing {
		   return fmt.Errorf("%w: %v", persist.ErrTxClosed, err)
	   }
	  if cur.N >= bookRateMax {
		  return ErrBookRate
	   }
	  cur.N++
	   body, merr := json.Marshal(cur)
	   if merr != nil {
		  return merr
	  }
	  if missing {
		  _,
			   _, err = persist.GetOrInit(ctx, s.kv, kindBookRate, key, body)
		   return err
	   }
	   return persist.Replace(ctx, s.kv, kindBookRate, key, body)
  }

   func (s *Service) loadBookIdem(ctx context.Context, userID, idem string) (ledger.AddressBookEntry, bool, error) {
	   if s == nil || s.kv == nil || strings.TrimSpace(idem) == "" {
		  return ledger.AddressBookEntry{}, false, nil
	  }
	   key, err := validate.Idem(userID + "|" + idem)
	  if err != nil {
		  return ledger.AddressBookEntry{}, false, err
	  }
	   row, err := s.kv.Get(ctx, kindBookIdem, key)
	  if err != nil {
		  if errors.Is(err, persist.ErrNotFound) {
			  return ledger.AddressBookEntry{}, false, nil
		  }
		   return ledger.AddressBookEntry{}, false, err
	   }
	  var rec idemRec
	   if json.Unmarshal(row.Body, &rec) != nil {
		   return ledger.AddressBookEntry{},
			  false, ErrBookIdem
	   }
	   return s.GetBook(ctx, rec.UserID, rec.Chain, rec.Address)
   }

   func (s *Service) saveBookIdem(ctx context.Context, userID, idem, chain, addr string) error {
	   if s == nil || s.kv == nil || strings.TrimSpace(idem) == "" {
		   return nil
	   }
	   key, err := validate.Idem(userID + "|" + idem)
	   if err != nil {
		   return err
	  }
	   body, err := json.Marshal(idemRec{UserID: userID, Chain: chain, Address: addr})
	  if err != nil {
		   return err
	   }
	  _, existed, err := persist.GetOrInit(ctx, s.kv, kindBookIdem, key, body)
	   if err != nil {
		   return err
	   }
	  if existed {
		   return persist.Replace(ctx, s.kv, kindBookIdem, key, body)
	   }
	   return nil
  }
