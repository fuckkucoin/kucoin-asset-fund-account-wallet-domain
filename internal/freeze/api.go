package freeze

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"account-assets.kucoin.net/funding/internal/account"
)

const ReasonMaintenanceBreach = "maintenance_breach"

var (
	ErrAccount = fmt.Errorf("freeze account required")
	ErrReason  = fmt.Errorf("freeze reason required")
)

// API 冻结入口。reason 须由调用方传入（门控使用 maintenance_breach）。

type API interface {
	Freeze(accountID string, reason string) error
	Frozen(accountID string) bool
	LastReason(accountID string) string
	FreezeCalls(accountID string) int
}

// Adapter 接到 account.Service.Freeze（margin 科目）。已冻结再次调用视为成功，不重复计数冲突。
type Adapter struct {
	mu sync.Mutex
	acct   *account.Service
	book   account.Book
	frozen map[string]bool
	reason map[string]string
	calls  map[string]int
}

  func NewAdapter(acct *account.Service) *Adapter {
	  book := account.BookMargin
	  return &Adapter{
		  acct: acct,
		  book: book,
		   frozen: map[string]bool{},
		   reason: map[string]string{},
		  calls:  map[string]int{},
	   }
   }

  func (a *Adapter) Freeze(accountID string, reason string) error {
	   accountID = strings.TrimSpace(accountID)
	   reason = strings.TrimSpace(reason)
	   if accountID == "" {
		  return ErrAccount
	   }
	   if reason == "" {
		  return ErrReason
	   }
	  a.mu.Lock()
	  defer a.mu.Unlock()
	  if a.frozen[accountID] {
		   return nil
	   }
	   if a.acct != nil {
		   ctx := context.Background()
		   if cur := a.acct.StatusOf(accountID, a.book); cur == account.StatusClosed {
			   if _, err := a.acct.Open(ctx, accountID, a.book); err != nil {
				  return err
			  }
		  }
		   st, err := a.acct.Freeze(ctx, accountID, a.book, true)
		  if err != nil {
			   return err
		  }
		  if st.Status == account.StatusFrozen {
			  a.frozen[accountID] = true
		   }
	   } else {
		  a.frozen[accountID] = true
	   }
	   a.reason[accountID] = reason
	   a.calls[accountID]++
	  return nil
   }

  func (a *Adapter) Frozen(accountID string) bool {
	   a.mu.Lock()
	   defer a.mu.Unlock()
	   return a.frozen[strings.TrimSpace(accountID)]
   }

  func (a *Adapter) LastReason(accountID string) string {
	   a.mu.Lock()
	  defer a.mu.Unlock()
	  return a.reason[strings.TrimSpace(accountID)]
   }

   func (a *Adapter) FreezeCalls(accountID string) int {
	  a.mu.Lock()
	  defer a.mu.Unlock()
	  return a.calls[strings.TrimSpace(accountID)]
  }
