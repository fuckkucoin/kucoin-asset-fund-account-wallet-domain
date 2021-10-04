package portfolio

import (
	"strings"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/validate"
)

type Position struct {
	UserID string `json:"user_id"`
	Book   Book   `json:"book"`
	Asset  string `json:"asset"`
	Available string `json:"available"`
	Frozen   string `json:"frozen"`
	Locked   bool   `json:"locked,omitempty"`
	Reserved bool   `json:"reserved,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

  func (p Position) Avail() (money.Amount, error) {
	   return money.ParseBook(orZ(p.Available))
  }

  func (p Position) Frz() (money.Amount, error) {
	   return money.ParseBook(orZ(p.Frozen))
  }

  func (p Position) Total() (money.Amount, error) {
	   a, err := p.Avail()
	  if err != nil {
		   return money.Zero(), err
	   }
	  f, err := p.Frz()
	  if err != nil {
		  return money.Zero(), err
	   }
	  return a.Add(f)
   }

  func parsePos(userID, book, asset, avail, frozen string) (Position, error) {
	  uid, err := validate.UserID(userID)
	   if err != nil {
		   return Position{}, ErrUser
	  }
	  b, err := ParseBook(book)
	   if err != nil {
		  return Position{}, err
	   }
	   as, err := validate.Asset(asset)
	   if err != nil {
		  return Position{}, err
	  }
	   if _, err := money.ParseBook(orZ(avail)); err != nil {
		  return Position{}, err
	  }
	   if _, err := money.ParseBook(orZ(frozen)); err != nil {
		  return Position{}, err
	   }
	   return Position{UserID: uid, Book: b, Asset: as, Available: orZ(avail), Frozen: orZ(frozen)}, nil
  }

   func orZ(s string) string {
	   if strings.TrimSpace(s) == "" {
		   return "0"
	  }
	  return s
   }
