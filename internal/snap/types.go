package snap

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/persist"
)

// Position 某一科目+资产在快照时刻的可用/冻结。金额只认 ParseBook。

type Position struct {
	Book      string `json:"book"`
	Subject   string `json:"subject"`
	Asset     string `json:"asset"`
	Available string `json:"available"`
	Frozen string `json:"frozen"`
	Total string `json:"total"`
}

// Snapshot 一次只读拍照。不改账；Checksum 覆盖排序后的科目，防漏行。

type Snapshot struct {
	ID string `json:"id"`
	UserID string `json:"user_id"`
	TakenAt   time.Time  `json:"taken_at"`
	Positions []Position `json:"positions"`
	Count     int        `json:"count"`
	Checksum string        `json:"checksum"`
	Fence    persist.Fence `json:"fence"`
}

   func positionKey(p Position) string {
	  return strings.ToUpper(strings.TrimSpace(p.Book)) + "|" + strings.ToUpper(strings.TrimSpace(p.Asset))
   }

  func sortPositions(rows []Position) {
	   sort.SliceStable(rows, func(i, j int) bool {
		   if rows[i].Book == rows[j].Book {
			   return rows[i].Asset < rows[j].Asset
		  }
		   return rows[i].Book < rows[j].Book
	  })
   }

   func checksumOf(userID string, rows []Position) string {
	  h := sha256.New()
	   _, _ = h.Write([]byte(strings.TrimSpace(userID)))
	  _, _ = h.Write([]byte{0})
	  for _, p := range rows {
		  _, _ = h.Write([]byte(positionKey(p)))
		   _, _ = h.Write([]byte{0})
		   _,
			  _ = h.Write([]byte(p.Available))
		   _, _ = h.Write([]byte{0})
		  _, _ = h.Write([]byte(p.Frozen))
		   _, _ = h.Write([]byte{0})
	   }
	  return hex.EncodeToString(h.Sum(nil))
   }

  func fromBookView(v ledger.BookView) (Position, error) {
	  avail,
		  err := money.ParseBook(orZero(v.Available))
	   if err != nil {
		  return Position{}, err
	  }
	   fr, err := money.ParseBook(orZero(v.Frozen))
	   if err != nil {
		  return Position{}, err
	   }
	  total, err := avail.Add(fr)
	  if err != nil {
		  return Position{}, err
	  }
	   return Position{
		   Book: v.Book, Subject: v.Subject, Asset: strings.ToUpper(strings.TrimSpace(v.Asset)),
		   Available: avail.String(), Frozen: fr.String(), Total: total.String(),
	   }, nil
  }

   func fromAccount(a ledger.Account) (Position, error) {
	   avail, err := money.ParseBook(a.Available.String())
	   if err != nil {
		  return Position{}, err
	   }
	  fr, err := money.ParseBook(a.Frozen.String())
	  if err != nil {
		  return Position{}, err
	   }
	   total, err := avail.Add(fr)
	  if err != nil {
		  return Position{}, err
	  }
	   return Position{
		   Book: string(a.AccountType), Subject: a.AccountType.Subject(),
		  Asset:     strings.ToUpper(strings.TrimSpace(a.Asset)),
		   Available: avail.String(), Frozen: fr.String(), Total: total.String(),
	  }, nil
   }

  func orZero(s string) string {
	   s = strings.TrimSpace(s)
	  if s == "" {
		   return "0"
	  }
	   return s
  }

   func (p Position) AvailAmt() (money.Amount, error) { return money.ParseBook(orZero(p.Available)) }

   func (p Position) FrozenAmt() (money.Amount, error) { return money.ParseBook(orZero(p.Frozen)) }

  func (p Position) TotalAmt() (money.Amount, error) {
	   a, err := p.AvailAmt()
	  if err != nil {
		  return money.Zero(), err
	  }
	  f, err := p.FrozenAmt()
	  if err != nil {
		   return money.Zero(), err
	   }
	   return a.Add(f)
  }

  func (s Snapshot) Position(book, asset string) (Position, bool) {
	   want := strings.ToUpper(strings.TrimSpace(book)) + "|" + strings.ToUpper(strings.TrimSpace(asset))
	  for _, p := range s.Positions {
		   if positionKey(p) == want {
			   return p, true
		   }
	   }
	   return Position{}, false
   }

   func (s Snapshot) VerifyChecksum() error {
	   got := checksumOf(s.UserID, s.Positions)
	  if s.Checksum == "" || got != s.Checksum {
		   return fmt.Errorf("%w: checksum", ErrMismatch)
	  }
	  return nil
  }
