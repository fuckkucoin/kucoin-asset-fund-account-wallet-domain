package cvquotelease

import (
	"context"
	"strings"
	"time"
)

   func (e *Engine) QuoteWithLease(ctx context.Context, req QuoteWithLeaseReq) (QuoteWithLeaseResp, error) {
	  if err := e.require(); err != nil {
		  return QuoteWithLeaseResp{}, err
	  }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	   user := normalizeUser(req.UserID)
	   from := NormalizeAsset(req.From)
	  to := NormalizeAsset(req.To)
	  if user == "" {
		  return QuoteWithLeaseResp{}, ErrUserRequired
	   }
	  if from == "" || to == "" {
		  return QuoteWithLeaseResp{}, ErrAssetRequired
	   }
	  if from == to {
		   return QuoteWithLeaseResp{},
			   ErrSameAsset
	  }
	  if _, err := ParsePositiveDecimal(req.Amount); err != nil {
		  return QuoteWithLeaseResp{}, ErrAmountInvalid
	  }
	   book, err := NormalizeBook(req.Book)
	  if err != nil {
		   return QuoteWithLeaseResp{}, err
	  }
	   if e.marks == nil {
		   return QuoteWithLeaseResp{},
			  ErrQuoteSrc
	   }
	   rate,
		   err := e.marks.Rate(ctx, from, to)
	  if err != nil || !rate.IsPositive() {
		  return QuoteWithLeaseResp{}, ErrQuoteSrc
	   }
	   ttl := e.ttl
	   if strings.TrimSpace(req.TTL) != "" {
		   d, perr := time.ParseDuration(req.TTL)
		   if perr != nil || d <= 0 {
			   return QuoteWithLeaseResp{}, ErrTTLInvalid
		   }
		   ttl = d
	   }
	  pair := PairKey(from, to)
	  mk, err := e.ensureMarkLocked(ctx, pair, from, to, rate.String(), book)
	   if err != nil {
		  return QuoteWithLeaseResp{}, err
	  }
	   now := e.now()
	  fp := ComputeFingerprint(from,
		  to, req.Amount, rate.String(), book, mk.Gen)
	  bp := ComputeBookPrint(book, req.Amount)
	   quoteID := newID()
	  leaseID := newID()
	   exp := now.Add(ttl)
	   lease := CvQuoteLease{
		  ID: leaseID,
		  QuoteID: quoteID,
		  UserID:  user,
		   FromAsset: from,
		  ToAsset: to,
		   Amount:  strings.TrimSpace(req.Amount),
		   Rate:    rate.String(),
		   Book:        book,
		   MarkGen:     mk.Gen,
		  Fingerprint: fp,
		   BookPrint:   bp,
		   State:       LeaseIssued,
		  TTL:       Duration{Duration: ttl},
		  IssuedAt:  now,
		  ExpiresAt: exp,
		   Used:      false,
		  CreatedAt: now,
		   UpdatedAt: now,
	  }
	   if _, err := e.store.SaveLease(ctx, lease); err != nil {
		   return QuoteWithLeaseResp{}, err
	  }
	  snap := QuoteSnapshot{
		  ID: quoteID, LeaseID: leaseID, UserID: user,
		  FromAsset: from, ToAsset: to, Amount: lease.Amount,
		  Rate: lease.Rate, Book: book, MarkGen: mk.Gen,
		   ExpiresAt: exp, Used: false, CreatedAt: now,
	  }
	   if _, err := e.store.SaveQuote(ctx, snap); err != nil {
		   return QuoteWithLeaseResp{}, err
	   }
	   return QuoteWithLeaseResp{
		  QuoteID: quoteID, LeaseID: leaseID, UserID: user,
		  From: from, To: to, Amount: lease.Amount, Rate: lease.Rate,
		  Book:    book,
		  MarkGen: mk.Gen, Fingerprint: fp, ExpiresAt: exp, Used: false,
	   }, nil
  }

   func (e *Engine) ensureMarkLocked(ctx context.Context, pair, from, to, rate, book string) (MarkGeneration, error) {
	   mk, err := e.store.GetMark(ctx, pair)
	   if err == nil {
		  if strings.TrimSpace(mk.Rate) == "" {
			   mk.Rate = rate
		  }
		   if mk.BookPrint == "" {
			   mk.BookPrint = ComputeBookPrint(book, "1")
		  }
		   return mk, nil
	   }
	   now := e.now()
	  mk = MarkGeneration{
		  PairKey: pair, FromAsset: from, ToAsset: to,
		   Gen: 1, Rate: rate, BookPrint: ComputeBookPrint(book, "1"),
		  Source: "quote_bootstrap", UpdatedAt: now,
	   }
	   return e.store.SaveMark(ctx, mk)
   }
