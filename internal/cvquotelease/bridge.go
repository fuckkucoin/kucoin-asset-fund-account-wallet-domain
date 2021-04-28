package cvquotelease

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/cvfund"
)

type BookPort struct {
	Eng *Engine
	Book *cvfund.Book
}

  func AttachBook(b *cvfund.Book, eng *Engine) *cvfund.Book {
	  if b == nil || eng == nil {
		   return b
	   }
	  p := &BookPort{Eng: eng, Book: b}
	  eng.BindStrict(true)
	  eng.BindFill(func(ctx context.Context, snap QuoteSnapshot) (FillResult, error) {
		  o, err := b.Place(snap.UserID, snap.FromAsset, snap.ToAsset, snap.Amount, snap.Rate, snap.Book)
		  if err != nil {
			  return FillResult{}, err
		  }
		   return FillResult{OrderID: o.ID, Rate: o.Rate, Book: o.Book}, nil
	   })
	  return b.BindQuoteLeasePort(p)
  }

   func (p *BookPort) QuoteIssued(ctx context.Context, userID, from, to, amount, book string) (cvfund.Quote, error) {
	  resp, err := p.Eng.QuoteWithLease(ctx, QuoteWithLeaseReq{
		   UserID: userID, From: from, To: to, Amount: amount, Book: book,
	   })
	   if err != nil {
		   return cvfund.Quote{}, err
	  }
	  return cvfund.Quote{
		   ID: resp.QuoteID, UserID: resp.UserID, FromAsset: resp.From, ToAsset: resp.To,
		   Amount: resp.Amount, Rate: resp.Rate, Book: resp.Book,
		  ExpiresAt: resp.ExpiresAt.UTC().Format(time.RFC3339),
		  Used: resp.Used, LeaseID: resp.LeaseID, MarkGen: resp.MarkGen, Fingerprint: resp.Fingerprint,
	  }, nil
  }

   func (p *BookPort) LeaseIDOfQuote(ctx context.Context, quoteID string) (string, error) {
	   q,
		   err := p.Eng.GetQuote(ctx, quoteID)
	   if err != nil {
		  return "", err
	  }
	   return q.LeaseID, nil
   }

  func (p *BookPort) ExecuteBound(ctx context.Context, userID, quoteID, leaseID string) (cvfund.Order, error) {
	   resp, err := p.Eng.ExecuteWithLease(ctx, ExecuteWithLeaseReq{UserID: userID, QuoteID: quoteID, LeaseID: leaseID})
	  if err != nil {
		  return cvfund.Order{}, err
	  }
	   if resp.OrderID != "" && p.Book != nil {
		  if o, gerr := p.Book.GetFor(userID, resp.OrderID); gerr == nil {
			   return o, nil
		   }
	  }
	   return cvfund.Order{ID: resp.OrderID,
		  UserID: userID, Rate: resp.Rate, Book: resp.Book, Status: statusOf(resp)}, nil
   }

   func (p *BookPort) PlaceQuotedBound(ctx context.Context, userID, from, to, amount, rate, book, quoteID string, allowBare bool) (cvfund.Order, error) {
	   resp, err := p.Eng.PlaceQuoted(ctx, PlaceQuotedReq{
		  UserID: userID, From: from, To: to, Amount: amount, Rate: rate, Book: book,
		   QuoteID: quoteID, AllowBare: allowBare,
	   })
	  if err != nil {
		  return cvfund.Order{}, err
	  }
	  if resp.OrderID != "" && p.Book != nil {
		   if o, gerr := p.Book.GetFor(userID, resp.OrderID); gerr == nil {
			   return o, nil
		   }
	   }
	  return cvfund.Order{ID: resp.OrderID,
		  UserID: userID, Status: statusOf(resp)}, nil
   }

   func statusOf(resp ExecuteWithLeaseResp) string {
	  if resp.Filled {
		   return "filled"
	   }
	   return "rejected"
   }
