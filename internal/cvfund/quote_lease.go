package cvfund

import (
	"context"
	"strings"
)

  func (b *Book) QuoteWithLease(ctx context.Context, userID, from, to, amount, book string) (Quote, error) {
	  if b == nil || b.lease == nil {
		   return b.Quote(ctx, userID, from, to, amount, book)
	   }
	   return b.lease.QuoteIssued(ctx,
		  userID, from, to, amount, book)
   }

  func (b *Book) ExecuteWithLease(ctx context.Context, userID, quoteID, leaseID string) (Order, error) {
	  if b == nil || b.lease == nil {
		  return b.Execute(ctx, userID, quoteID)
	  }
	  return b.lease.ExecuteBound(ctx, userID, quoteID, leaseID)
   }

  func (b *Book) leaseIDOf(q Quote) string {
	  return strings.TrimSpace(q.LeaseID)
  }
