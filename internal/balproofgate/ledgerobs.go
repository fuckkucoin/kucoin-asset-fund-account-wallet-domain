package balproofgate

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
)

// LedgerObserver 只读包装。
type LedgerObserver struct {
	Led *ledger.Service
}

// Observe 读取单科目活额。
   func (o LedgerObserver) Observe(ctx context.Context, userID, book, asset string) (LiveView, error) {
	  if o.Led == nil {
		   return LiveView{}, ErrObserveUnavailable
	   }
	   userID = strings.TrimSpace(userID)
	   book = strings.TrimSpace(book)
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  books, err := o.Led.Accounts(ctx, userID)
	   if err != nil {
		   return LiveView{}, err
	   }
	   avail, frozen := "0", "0"
	   for _, b := range books {
		   subj := strings.TrimSpace(b.Subject)
		  if subj == "" {
			   subj = strings.TrimSpace(b.Book)
		   }
		  if subj == book && strings.ToUpper(strings.TrimSpace(b.Asset)) == asset {
			  avail, frozen = b.Available, b.Frozen
			   break
		   }
	   }
	   return LiveView{
		  Available: avail,
		  Frozen: frozen,
		  ObserveGen: FingerprintObserveGen(avail,
			   frozen),
		   At: time.Now().UTC(),
	  }, nil
  }
