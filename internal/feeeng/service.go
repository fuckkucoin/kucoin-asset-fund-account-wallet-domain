// feeeng/service.go

// Author: Cason
// Date: 2021.04.29

package feeeng

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
)

type Service struct {
	book *Book
}

  func New() *Service { return &Service{book: DefaultBook()} }

   func NewWith(book *Book) *Service {
	   if book == nil {
		   book = DefaultBook()
	   }
	   return &Service{book: book}
   }

   func (s *Service) Book() *Book {
	   if s == nil {
		   return nil
	   }
	   return s.book
   }

type QuoteReq struct {
	Kind string
	Asset  string
	Gross  string
	Volume string
}

  func (s *Service) Quote(req QuoteReq) (Quote, error) {
	  if s == nil || s.book == nil {
		   return Quote{}, ErrUnavailable
	   }
	   kind, err := ParseKind(req.Kind)
	   if err != nil {
		  return Quote{}, err
	  }
	   asset,
		  err := validate.Asset(req.Asset)
	   if err != nil {
		   return Quote{}, err
	  }
	  gross, err := validate.Amount(req.Gross)
	   if err != nil {
		   return Quote{}, err
	   }
	   if strings.TrimSpace(req.Volume) != "" {
		  return s.book.QuoteTiered(kind, asset, gross, req.Volume, DefaultTiers())
	   }
	   return s.book.Quote(kind, asset, gross)
  }

  func (s *Service) Apply(req QuoteReq) (Applied, error) {
	  q, err := s.Quote(req)
	   if err != nil {
		  return Applied{}, err
	   }
	   return s.book.Apply(q.Kind, q.Asset, q.Gross)
  }

  func (s *Service) Put(sch Schedule) error {
	  if s == nil || s.book == nil {
		   return ErrUnavailable
	  }
	  return s.book.Put(sch)
  }

  func (s *Service) List() []Schedule {
	   if s == nil || s.book == nil {
		   return nil
	   }
	  return s.book.List()
   }

  func DefaultTiers() []Tier {
	   return []Tier{
		  {Notional: "0", BPS: 10},
		  {Notional: "10000", BPS: 8},
		  {Notional: "100000", BPS: 5},
		  {Notional: "1000000", BPS: 2},
	  }
   }
