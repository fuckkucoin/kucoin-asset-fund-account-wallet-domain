package cvquotelease

import (
	"context"
	"errors"
	"strings"
	"time"
)

   func (e *Engine) ExecuteWithLease(ctx context.Context, req ExecuteWithLeaseReq) (ExecuteWithLeaseResp, error) {
	  if err := e.require(); err != nil {
		  return ExecuteWithLeaseResp{}, err
	  }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	   return e.executeLocked(ctx, req)
   }

  func (e *Engine) executeLocked(ctx context.Context, req ExecuteWithLeaseReq) (ExecuteWithLeaseResp, error) {
	   user := normalizeUser(req.UserID)
	  quoteID := strings.TrimSpace(req.QuoteID)
	   leaseID := strings.TrimSpace(req.LeaseID)
	  if quoteID == "" {
		   return ExecuteWithLeaseResp{}, ErrQuoteIDRequired
	   }
	   if leaseID == "" {
		  proof := e.sealProofLocked(ctx, "", quoteID, user, "", "", DriftMissingLease, 0, 0, "", "", "", "", "", "", "strict execute missing lease")
		   return ExecuteWithLeaseResp{QuoteID: quoteID, DriftProof: &proof, Filled: false}, ErrLeaseRequired
	  }
	  lease, err := e.store.GetLease(ctx, leaseID)
	  if err != nil {
		  proof := e.sealProofLocked(ctx, leaseID, quoteID, user, "", "", DriftMissingLease, 0, 0, "", "", "", "", "", "", "lease not found")
		   return ExecuteWithLeaseResp{QuoteID: quoteID, LeaseID: leaseID, DriftProof: &proof}, ErrLeaseNotFound
	   }
	  if lease.QuoteID != quoteID {
		  proof := e.sealProofLocked(ctx, lease.ID, quoteID, user, lease.FromAsset, lease.ToAsset, DriftFingerprint, lease.MarkGen, 0, lease.Rate, "", lease.Book, "", lease.Fingerprint, "", "lease quote binding mismatch")
		   return ExecuteWithLeaseResp{QuoteID: quoteID, LeaseID: leaseID, DriftProof: &proof}, ErrLeaseMismatch
	   }
	   if user != "" && lease.UserID != user {
		   proof := e.sealProofLocked(ctx, lease.ID, quoteID, user, lease.FromAsset, lease.ToAsset, DriftOwner, lease.MarkGen, 0, lease.Rate, "", lease.Book, "", lease.Fingerprint, "", "owner mismatch")
		   return ExecuteWithLeaseResp{QuoteID: quoteID, LeaseID: leaseID, DriftProof: &proof}, ErrOwnerMismatch
	  }
	  if lease.State == LeaseConsumed && lease.Used && lease.OrderID != "" {
		   return ExecuteWithLeaseResp{
			   OrderID: lease.OrderID, QuoteID: quoteID, LeaseID: lease.ID,
			  State: LeaseConsumed, Filled: true, Reused: true, Rate: lease.Rate, Book: lease.Book, Used: true,
		  }, nil
	   }
	  if lease.State == LeaseRevoked {
		   proof := e.sealProofLocked(ctx, lease.ID, quoteID, lease.UserID, lease.FromAsset, lease.ToAsset, DriftRevoked, lease.MarkGen, 0, lease.Rate, "", lease.Book, "", lease.Fingerprint, "", "lease revoked")
		   return ExecuteWithLeaseResp{QuoteID: quoteID, LeaseID: lease.ID, State: lease.State, DriftProof: &proof}, ErrLeaseRevoked
	   }
	  now := e.now()
	   if lease.State == LeaseExpired || now.After(lease.ExpiresAt) {
		  if err := e.expireIssuedLocked(ctx, lease, now, "execute saw expiry"); err != nil && !errors.Is(err, ErrIllegalTransition) {
			   _ = err
		  }
		  // 过期 Quote 不得靠 Used=false 复活重入
		   if q, qerr := e.store.GetQuote(ctx, quoteID); qerr == nil && !q.Used {
			  q.Used = true
			   _, _ = e.store.SaveQuote(ctx, q)
		   }
		  proof := e.sealProofLocked(ctx, lease.ID, quoteID, lease.UserID, lease.FromAsset, lease.ToAsset, DriftExpired, lease.MarkGen, 0, lease.Rate, "", lease.Book, "", lease.Fingerprint, "", "lease expired; revive Used=false forbidden")
		  return ExecuteWithLeaseResp{QuoteID: quoteID, LeaseID: lease.ID, State: LeaseExpired, DriftProof: &proof, Used: true}, ErrLeaseExpired
	   }
	  if lease.State != LeaseIssued {
		  return ExecuteWithLeaseResp{QuoteID: quoteID, LeaseID: lease.ID, State: lease.State}, ErrIllegalTransition
	   }
	   if resp, rerr := e.recheckLiveFingerprint(ctx, lease); rerr != nil {
		  return resp, rerr
	   }
	  snap, err := e.store.GetQuote(ctx, quoteID)
	  if err != nil {
		  return ExecuteWithLeaseResp{QuoteID: quoteID,
			   LeaseID: lease.ID}, err
	  }
	   fill,
		  ferr := e.doFill(ctx, snap)
	  if ferr != nil {
		   return ExecuteWithLeaseResp{QuoteID: quoteID, LeaseID: lease.ID}, ferr
	  }
	   token := newID()
	  consumed, won, cerr := e.store.CASConsume(ctx, lease.ID, token, now, fill.OrderID)
	  if cerr != nil {
		  if errors.Is(cerr, ErrDualExecute) || errors.Is(cerr, ErrLeaseConsumed) {
			   proof := e.sealProofLocked(ctx, lease.ID, quoteID, lease.UserID, lease.FromAsset, lease.ToAsset, DriftDualExecute, lease.MarkGen, lease.MarkGen, lease.Rate, lease.Rate, lease.Book, lease.Book, lease.Fingerprint, lease.Fingerprint, "concurrent dual execute")
			  return ExecuteWithLeaseResp{QuoteID: quoteID, LeaseID: lease.ID, State: consumed.State, Used: consumed.Used, OrderID: consumed.OrderID, DriftProof: &proof}, ErrDualExecute
		   }
		   return ExecuteWithLeaseResp{QuoteID: quoteID, LeaseID: lease.ID}, cerr
	   }
	   if !won {
		   return ExecuteWithLeaseResp{
			   OrderID: consumed.OrderID, QuoteID: quoteID, LeaseID: lease.ID,
			  State: LeaseConsumed, Filled: true, Reused: true, Rate: consumed.Rate, Book: consumed.Book, Used: true,
		   }, nil
	  }
	  return ExecuteWithLeaseResp{
		   OrderID: fill.OrderID, QuoteID: quoteID, LeaseID: lease.ID,
		   State: LeaseConsumed, Filled: true, Rate: lease.Rate, Book: lease.Book, Used: true,
	  }, nil
  }

   func (e *Engine) recheckLiveFingerprint(ctx context.Context, lease CvQuoteLease) (ExecuteWithLeaseResp, error) {
	  if e.marks == nil {
		  proof := e.sealProofLocked(ctx, lease.ID, lease.QuoteID, lease.UserID, lease.FromAsset, lease.ToAsset, DriftRate, lease.MarkGen, 0, lease.Rate, "", lease.Book, "", lease.Fingerprint, "", "mark source gone")
		  return ExecuteWithLeaseResp{QuoteID: lease.QuoteID, LeaseID: lease.ID, DriftProof: &proof}, ErrQuoteSrc
	  }
	  liveRate, err := e.marks.Rate(ctx, lease.FromAsset, lease.ToAsset)
	   if err != nil || !liveRate.IsPositive() {
		   proof := e.sealProofLocked(ctx, lease.ID, lease.QuoteID, lease.UserID, lease.FromAsset, lease.ToAsset, DriftRate, lease.MarkGen, 0, lease.Rate, "", lease.Book, "", lease.Fingerprint, "", "live rate unavailable")
		  return ExecuteWithLeaseResp{QuoteID: lease.QuoteID,
			  LeaseID: lease.ID, DriftProof: &proof}, ErrQuoteSrc
	  }
	   mk, err := e.store.GetMark(ctx, PairKey(lease.FromAsset, lease.ToAsset))
	   if err != nil {
		  proof := e.sealProofLocked(ctx, lease.ID, lease.QuoteID, lease.UserID, lease.FromAsset, lease.ToAsset, DriftMarkGen, lease.MarkGen, 0, lease.Rate, liveRate.String(), lease.Book, "", lease.Fingerprint, "", "mark gen missing")
		  return ExecuteWithLeaseResp{QuoteID: lease.QuoteID, LeaseID: lease.ID, DriftProof: &proof}, ErrMarkGenMissing
	   }
	  livePrint := ComputeFingerprint(lease.FromAsset, lease.ToAsset, lease.Amount, liveRate.String(), lease.Book, mk.Gen)
	  liveBook := ComputeBookPrint(lease.Book, lease.Amount)
	   if mk.Gen != lease.MarkGen {
		   proof := e.sealProofLocked(ctx, lease.ID, lease.QuoteID, lease.UserID, lease.FromAsset, lease.ToAsset, DriftMarkGen, lease.MarkGen, mk.Gen, lease.Rate, liveRate.String(), lease.Book, lease.Book, lease.Fingerprint, livePrint, "mark_gen bumped after quote")
		  return ExecuteWithLeaseResp{QuoteID: lease.QuoteID, LeaseID: lease.ID, DriftProof: &proof, State: lease.State}, ErrMarkGenDrift
	  }
	   leaseRate,
		  rerr := ParsePositiveRate(lease.Rate)
	  if rerr != nil || !liveRate.Equal(leaseRate) {
		   proof := e.sealProofLocked(ctx, lease.ID, lease.QuoteID, lease.UserID, lease.FromAsset, lease.ToAsset, DriftRate, lease.MarkGen, mk.Gen, lease.Rate, liveRate.String(), lease.Book, lease.Book, lease.Fingerprint, livePrint, "board rate drifted")
		  return ExecuteWithLeaseResp{QuoteID: lease.QuoteID, LeaseID: lease.ID, DriftProof: &proof}, ErrRateDrift
	   }
	  if !FingerprintsEqual(liveBook, lease.BookPrint) {
		   proof := e.sealProofLocked(ctx, lease.ID, lease.QuoteID, lease.UserID, lease.FromAsset, lease.ToAsset, DriftBook, lease.MarkGen, mk.Gen, lease.Rate, liveRate.String(), lease.Book, lease.Book, lease.Fingerprint, livePrint, "book fingerprint drifted")
		   return ExecuteWithLeaseResp{QuoteID: lease.QuoteID,
			   LeaseID: lease.ID, DriftProof: &proof}, ErrBookDrift
	  }
	  if !FingerprintsEqual(livePrint, lease.Fingerprint) {
		   proof := e.sealProofLocked(ctx, lease.ID, lease.QuoteID, lease.UserID, lease.FromAsset, lease.ToAsset, DriftFingerprint, lease.MarkGen, mk.Gen, lease.Rate, liveRate.String(), lease.Book, lease.Book, lease.Fingerprint, livePrint, "composite fingerprint drifted")
		  return ExecuteWithLeaseResp{QuoteID: lease.QuoteID, LeaseID: lease.ID, DriftProof: &proof}, ErrFingerprintDrift
	   }
	   return ExecuteWithLeaseResp{}, nil
  }

   func (e *Engine) expireIssuedLocked(ctx context.Context, lease CvQuoteLease, now time.Time, reason string) error {
	   if lease.State != LeaseIssued {
		   return nil
	   }
	  next, err := e.fsm.Apply(string(lease.State), LeaseEvExpire)
	  if err != nil {
		  return ErrIllegalTransition
	   }
	  ts := now
	  lease.State = LeaseState(next)
	  lease.ExpiredAt = &ts
	  lease.UpdatedAt = now
	   lease.RevokeReason = reason
	  _, err = e.store.SaveLease(ctx, lease)
	  return err
  }

// PlaceQuoted 严格模式：无租约裸 Place 不得成交。
  func (e *Engine) PlaceQuoted(ctx context.Context, req PlaceQuotedReq) (ExecuteWithLeaseResp, error) {
	  if err := e.require(); err != nil {
		  return ExecuteWithLeaseResp{}, err
	  }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	  if strings.TrimSpace(req.QuoteID) != "" {
		  return e.executeLocked(ctx, ExecuteWithLeaseReq{UserID: req.UserID, QuoteID: req.QuoteID, LeaseID: req.LeaseID})
	  }
	  if e.strict && !req.AllowBare {
		  proof := e.sealProofLocked(ctx, "", "", req.UserID, req.From, req.To, DriftMissingLease, 0, 0, req.Rate, "", req.Book, "", "", "", "strict bare Place fail-closed")
		   return ExecuteWithLeaseResp{DriftProof: &proof, Filled: false}, ErrStrictBarePlace
	  }
	  if !req.AllowBare {
		  return ExecuteWithLeaseResp{}, ErrLeaseRequired
	  }
	  return ExecuteWithLeaseResp{Filled: false}, ErrStrictBarePlace
  }
