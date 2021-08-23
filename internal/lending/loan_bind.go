package lending

import (
	"account-assets.kucoin.net/funding/internal/lending/accrual"
	"account-assets.kucoin.net/funding/internal/lending/repay"
	"account-assets.kucoin.net/funding/internal/marginhook"
	"account-assets.kucoin.net/funding/internal/utabook"
)

type InterestStack struct {
	Book *utabook.Book
	Hook *marginhook.Service
	Acc *accrual.Poster
	Rep *repay.Service
}

   func (s *Service) BindInterest(book *utabook.Book, hook *marginhook.Service) *InterestStack {
	   if s == nil || s.led == nil || book == nil {
		   return nil
	  }
	  if hook == nil {
		   hook = marginhook.New()
	  }
	  return &InterestStack{
		  Book: book, Hook: hook,
		   Acc: accrual.New(s.led, book, hook),
		   Rep: repay.New(s.led, book, hook),
	   }
  }

  func (st *InterestStack) Ready() error {
	  if st == nil || st.Acc == nil || st.Rep == nil {
		  return ErrUnavailable
	   }
	  return st.Acc.Ready()
   }
