package fiatsettlegate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/fiatrail"
	"account-assets.kucoin.net/funding/internal/validate"
)

// Service 结清闸。

type Service struct {
	store Store
	rail  *fiatrail.Service
	led   LedgerHook
	clock func() time.Time
	ttl   time.Duration
	lockT time.Duration
}

// New 默认内存仓。
  func New(rail *fiatrail.Service, led LedgerHook) *Service {
	  return NewWithStore(NewMemStore(),
		   rail, led)
   }

// NewWithStore 注入仓。
  func NewWithStore(store Store, rail *fiatrail.Service, led LedgerHook) *Service {
	  if store == nil {
		  store = NewMemStore()
	  }
	  if rail == nil {
		   rail = fiatrail.New()
	  }
	  return &Service{
		  store: store,
		   rail:  rail,
		  led:   led,
		  clock: func() time.Time { return time.Now().UTC() },
		  ttl:   time.Duration(DefaultLeaseTTL) * time.Second,
		   lockT: 30 * time.Second,
	  }
  }

// WithClock 时钟。
   func (s *Service) WithClock(c func() time.Time) *Service {
	  if c != nil {
		   s.clock = c
	   }
	   return s
  }

   // WithTTL 覆盖租约 TTL。
  func (s *Service) WithTTL(d time.Duration) *Service {
	  if d > 0 {
		  s.ttl = d
	  }
	   return s
  }

   func (s *Service) now() time.Time { return s.clock() }

// Rail 只读轨道。
   func (s *Service) Rail() *fiatrail.Service { return s.rail }

// Store 仓（worker / ）。
   func (s *Service) Store() Store { return s.store }

// BookWithReserve Check 后占用 limits_reservation。

  func (s *Service) BookWithReserve(ctx context.Context, req fiatrail.BookReq) (fiatrail.Booking, *LimitsReservation, error) {
	   if s == nil || s.rail == nil {
		   return fiatrail.Booking{}, nil, ErrUnavailable
	   }
	   b, err := s.rail.Book(req)
	  if err != nil {
		   return fiatrail.Booking{}, nil, err
	   }
	   res, err := s.OccupyLimits(ctx, OccupyLimitsRequest{
		  BookingID: b.ID, UserID: b.UserID, Kind: string(b.Kind), Amount: b.Amount, CardRef: b.CardRef,
	   })
	  if err != nil {
		  return b, nil, err
	  }
	  if _, err := s.rail.BindLimitsReservation(b.ID, res.ReservationID); err != nil {
		   return b, res, err
	  }
	  b, _ = s.rail.Get(b.ID)
	   s.audit(ctx, "", b.ID, "book_reserve", "system", "", "", res.ReservationID)
	  return b, res, nil
   }

// OccupyLimits Check 通过后占用。失败不 Record。

  func (s *Service) OccupyLimits(ctx context.Context, req OccupyLimitsRequest) (*LimitsReservation, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	  }
	  kind, err := fiatrail.ParseKind(req.Kind)
	   if err != nil {
		  return nil, err
	  }
	   if err := s.rail.Limits().Check(req.UserID, req.Amount, req.CardRef, kind); err != nil {
		  return nil, fmt.Errorf("%w: %v", ErrLimit, err)
	  }
	   if existing, e := s.store.GetReservationByBooking(ctx, req.BookingID); e == nil && existing != nil {
		  if existing.State == ResReserved || existing.State == ResRecorded {
			   return existing, nil
		  }
	  }
	   now := s.now()
	   r := LimitsReservation{
		   ReservationID: uuid.NewString(),
		  BookingID: strings.TrimSpace(req.BookingID),
		  UserID: strings.TrimSpace(req.UserID),
		  Kind: string(kind),
		   Amount:            req.Amount,
		   LimitsFingerprint: s.rail.Limits().Fingerprint(),
		  State: ResReserved,
		   CreatedAt: now,
		   UpdatedAt: now,
	  }
	   if err := s.store.InsertReservation(ctx, r); err != nil {
		  return nil, err
	  }
	  return &r, nil
  }

// AuthorizeWithLease 授权成功必须签发 fiat_settle_lease_id。

  func (s *Service) AuthorizeWithLease(ctx context.Context, req AuthorizeLeaseRequest) (*FiatSettleLease, fiatrail.Booking, error) {
	   if s == nil || s.rail == nil {
		  return nil, fiatrail.Booking{}, ErrUnavailable
	   }
	  actor := strings.TrimSpace(req.Actor)
	   if actor == "" {
		   return nil,
			  fiatrail.Booking{}, ErrActor
	  }
	   if req.IdemKey != "" {
		   if id, ok, _ := s.store.GetIdem(ctx, "auth:"+req.IdemKey); ok {
			  l, err := s.store.GetLease(ctx, id)
			   if err != nil {
				  return nil, fiatrail.Booking{}, err
			   }
			   b, _ := s.rail.Get(l.BookingID)
			   return l, b, nil
		  }
	   }
	  b, err := s.rail.Get(req.BookingID)
	   if err != nil {
		  return nil, fiatrail.Booking{}, ErrBooking
	   }
	   res, rerr := s.store.GetReservationByBooking(ctx, b.ID)
	   if rerr != nil {
		   res, rerr = s.OccupyLimits(ctx, OccupyLimitsRequest{
			  BookingID: b.ID, UserID: b.UserID, Kind: string(b.Kind), Amount: b.Amount, CardRef: b.CardRef,
		  })
		  if rerr != nil {
			  return nil, b, rerr
		  }
		   _, _ = s.rail.BindLimitsReservation(b.ID, res.ReservationID)
	   }
	  if b.State != fiatrail.StateAuthorized {
		  b, err = s.rail.Authorize(b.ID)
		   if err != nil {
			   _ = s.releaseReservation(ctx, res, "authorize_failed")
			   return nil, b, err
		   }
	  }
	   ttl := req.TTLSeconds
	   if ttl <= 0 {
		   ttl = int64(s.ttl / time.Second)
	   }
	   now := s.now()
	  lease := FiatSettleLease{
		  LeaseID:   uuid.NewString(),
		  BookingID: b.ID,
		  UserID:    b.UserID,
		  Kind:      string(b.Kind),
		   Asset:     b.Asset,
		  Amount: b.Amount,
		   Currency: b.Currency,
		   PSPIntent: b.PSPIntent,
		   KYCTier: string(b.KYCTier),
		  KYCFingerprint:    s.rail.KYC().KYCFingerprint(b.UserID),
		  LimitsFingerprint: s.rail.Limits().Fingerprint(),
		   ReservationID:     res.ReservationID,
		   State:             LeaseAuthorized,
		  Generation:        1,
		  TTLSeconds:        ttl,
		  IssuedAt:          now,
		   ExpiresAt:         now.Add(time.Duration(ttl) * time.Second),
		  Actor:             actor,
	  }
	  if err := s.store.InsertLease(ctx, lease); err != nil {
		  if errors.Is(err, ErrLeaseConsumed) {
			   exist, gerr := s.store.GetLeaseByBooking(ctx, b.ID)
			   if gerr == nil {
				  bb, _ := s.rail.Get(b.ID)
				  return exist, bb, nil
			   }
		  }
		  return nil, b, err
	  }
	  res.LeaseID = lease.LeaseID
	   res.UpdatedAt = now
	   _ = s.store.CASReservation(ctx, res.ReservationID, ResReserved, *res)
	  if _, err := s.rail.BindSettleLease(b.ID, lease.LeaseID, res.ReservationID); err != nil {
		  return &lease, b, err
	   }
	  if req.IdemKey != "" {
		   _ = s.store.PutIdem(ctx, "auth:"+req.IdemKey, lease.LeaseID)
	   }
	  s.audit(ctx, lease.LeaseID, b.ID, "authorize_lease", actor, "", string(LeaseAuthorized), lease.LeaseID)
	  b, _ = s.rail.Get(b.ID)
	   cp := lease
	   return &cp, b, nil
  }

   func (s *Service) revalidateFingerprints(ctx context.Context, lease *FiatSettleLease, b fiatrail.Booking) error {
	   if strings.TrimSpace(lease.PSPIntent) != strings.TrimSpace(b.PSPIntent) {
		  s.openProof(ctx, lease, ProofPSPMismatch, true, "", "", "psp intent drifted", false, false)
		  return ErrFingerprintPSP
	  }
	   liveL := s.rail.Limits().Fingerprint()
	   if liveL != lease.LimitsFingerprint {
		  s.openProof(ctx, lease, ProofLimitsDrift, true, "", "", "limits fingerprint raised or tightened", false, false)
		  return ErrFingerprintLimits
	  }
	  liveK := s.rail.KYC().KYCFingerprint(b.UserID)
	  if liveK != lease.KYCFingerprint {
		  s.openProof(ctx, lease, ProofKYCDowngrade, true, "", "", "kyc downgrade or material invalid", false, false)
		  return ErrFingerprintKYC
	   }
	   if _, err := s.rail.KYC().Check(b.UserID, b.Kind, b.Amount); err != nil {
		   s.openProof(ctx, lease, ProofKYCDowngrade, true, "", "", err.Error(), false, false)
		   return fmt.Errorf("%w: %v", ErrKYC, err)
	   }
	   if err := s.rail.Limits().Check(b.UserID, b.Amount, b.CardRef, b.Kind); err != nil {
		   s.openProof(ctx, lease, ProofLimitsDrift, true, "", "", err.Error(), false, false)
		   return fmt.Errorf("%w: %v", ErrLimit, err)
	   }
	  return nil
  }

  func (s *Service) expireIfNeeded(ctx context.Context, lease *FiatSettleLease) error {
	  if lease.State != LeaseAuthorized {
		   return nil
	  }
	  if lease.ExpiresAt.IsZero() || s.now().Before(lease.ExpiresAt) {
		   return nil
	   }
	  next := *lease
	  next.State = LeaseExpired
	   next.Generation++
	  t := s.now()
	  next.RevokedAt = &t
	   next.Note = "ttl exceeded"
	  if err := s.store.CASLease(ctx, lease.LeaseID, lease.Generation, lease.State, next); err != nil {
		   return err
	   }
	  if res, err := s.store.GetReservation(ctx, lease.ReservationID); err == nil {
		   _ = s.releaseReservation(ctx, res, "lease_expired")
	  }
	   s.openProof(ctx, &next, ProofExpireRevoke, true, "", "", "authorized lease expired", lease.CaptureSealed, false)
	   *lease = next
	  return ErrLeaseExpired
   }

  func (s *Service) releaseReservation(ctx context.Context, res *LimitsReservation, reason string) error {
	  if res == nil || res.State != ResReserved {
		  return nil
	  }
	   if err := CanTransitReservation(res.State, ResReleased); err != nil {
		  return err
	  }
	   next := *res
	   next.State = ResReleased
	  now := s.now()
	  next.ReleasedAt = &now
	  next.UpdatedAt = now
	   next.ReleaseReason = reason
	  return s.store.CASReservation(ctx, res.ReservationID, ResReserved, next)
  }

  func (s *Service) recordReservation(ctx context.Context, res *LimitsReservation) error {
	  if res == nil {
		  return ErrReservationNotFound
	   }
	   if res.State == ResRecorded {
		   return nil
	  }
	  if err := CanTransitReservation(res.State, ResRecorded); err != nil {
		  return err
	   }
	  next := *res
	   next.State = ResRecorded
	  now := s.now()
	   next.RecordedAt = &now
	  next.UpdatedAt = now
	  return s.store.CASReservation(ctx, res.ReservationID, ResReserved, next)
   }

   func (s *Service) audit(ctx context.Context, leaseID, bookingID, action, actor, from, to, detail string) {
	   _ = s.store.InsertAudit(ctx, AuditEvent{
		   AuditID: uuid.NewString(), LeaseID: leaseID, BookingID: bookingID,
		   Action: action, Actor: actor, FromState: from, ToState: to, Detail: detail, CreatedAt: s.now(),
	   })
  }

  func (s *Service) openProof(ctx context.Context, lease *FiatSettleLease, kind ProofKind, fail bool, winner, loser, summary string, sealed, settled bool) *FiatSettleProof {
	  p := FiatSettleProof{
		   ProofID: uuid.NewString(), Kind: kind, FailClosed: fail,
		   Winner: winner, Loser: loser, Summary: summary,
		   CaptureSealed: sealed, SettledHeld: settled, OpenedAt: s.now(),
	   }
	  if lease != nil {
		  p.LeaseID = lease.LeaseID
		  p.BookingID = lease.BookingID
	   }
	  _ = s.store.InsertProof(ctx, p)
	  return &p
  }

// View 只读导出。
   func (s *Service) View(ctx context.Context, bookingID string) (*LeaseView, error) {
	  v := &LeaseView{}
	   if l, err := s.store.GetLeaseByBooking(ctx, bookingID); err == nil {
		  v.Lease = l
		   v.Proofs, _ = s.store.ListProofsByLease(ctx, l.LeaseID)
		   v.Audits, _ = s.store.ListAuditsByLease(ctx, l.LeaseID, 50)
	   }
	  if r, err := s.store.GetReservationByBooking(ctx, bookingID); err == nil {
		   v.Reservation = r
	  }
	   if rf, err := s.store.GetRefundLeaseByBooking(ctx, bookingID); err == nil {
		  v.Refund = rf
	  }
	  return v, nil
  }

// ExportProofs 证明导出。
   func (s *Service) ExportProofs(ctx context.Context, leaseID string) ([]FiatSettleProof, error) {
	  rows, err := s.store.ListProofsByLease(ctx, leaseID)
	   if err != nil {
		   return nil, err
	   }
	  now := s.now()
	   for i := range rows {
		   _ = s.store.MarkProofExported(ctx, rows[i].ProofID, now)
		   rows[i].ExportedAt = &now
	   }
	   return rows, nil
  }

// GetLease 只读。

  func (s *Service) GetLease(ctx context.Context, id string) (*FiatSettleLease, error) {
	  return s.store.GetLease(ctx, id)
  }

  func requireActor(actor string) (string, error) {
	   a := strings.TrimSpace(actor)
	   if a == "" {
		  return "", ErrActor
	  }
	  if _, err := validate.Actor(a); err != nil {
		   return "", ErrActor
	   }
	   return a, nil
   }
