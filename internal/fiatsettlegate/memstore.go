package fiatsettlegate

import (
	"context"
	"strings"
	"sync"
	"time"
)

// MemStore 内存实现。

type MemStore struct {
	mu sync.Mutex
	leases map[string]FiatSettleLease
	byBook map[string]string
	reserv map[string]LimitsReservation
	resBk  map[string]string
	refund map[string]RefundLease
	refBk  map[string]string
	proofs map[string]FiatSettleProof
	audits []AuditEvent
	idem   map[string]string
	locks  map[string]BookingLock
}

// NewMemStore 空仓。
  func NewMemStore() *MemStore {
	  return &MemStore{
		  leases: map[string]FiatSettleLease{},
		  byBook: map[string]string{},
		  reserv: map[string]LimitsReservation{},
		   resBk:  map[string]string{},
		  refund: map[string]RefundLease{},
		   refBk:  map[string]string{},
		   proofs: map[string]FiatSettleProof{},
		   idem:   map[string]string{},
		   locks:  map[string]BookingLock{},
	  }
   }

  func (m *MemStore) InsertLease(_ context.Context, l FiatSettleLease) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   if _, ok := m.leases[l.LeaseID]; ok {
		  return ErrIdem
	   }
	   if id, ok := m.byBook[l.BookingID]; ok {
		  cur := m.leases[id]
		   if !cur.State.Terminal() {
			  return ErrLeaseConsumed
		   }
	  }
	   m.leases[l.LeaseID] = l
	  m.byBook[l.BookingID] = l.LeaseID
	  return nil
   }

  func (m *MemStore) GetLease(_ context.Context, leaseID string) (*FiatSettleLease, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  l, ok := m.leases[strings.TrimSpace(leaseID)]
	   if !ok {
		   return nil, ErrLeaseNotFound
	   }
	   cp := l
	  return &cp, nil
   }

   func (m *MemStore) GetLeaseByBooking(_ context.Context, bookingID string) (*FiatSettleLease, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  id, ok := m.byBook[strings.TrimSpace(bookingID)]
	   if !ok {
		  return nil, ErrLeaseNotFound
	  }
	  l := m.leases[id]
	   cp := l
	  return &cp, nil
   }

  func (m *MemStore) CASLease(_ context.Context, leaseID string, expectedGen int64, expectedState LeaseState, next FiatSettleLease) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   cur, ok := m.leases[leaseID]
	   if !ok {
		  return ErrLeaseNotFound
	  }
	  if cur.Generation != expectedGen || cur.State != expectedState {
		   return ErrCAS
	  }
	  m.leases[leaseID] = next
	   m.byBook[next.BookingID] = next.LeaseID
	   return nil
   }

  func (m *MemStore) ListLeasesByState(_ context.Context, state LeaseState, limit int) ([]FiatSettleLease, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if limit <= 0 {
		   limit = 100
	  }
	   out := make([]FiatSettleLease, 0, limit)
	   for _, l := range m.leases {
		   if l.State == state {
			   out = append(out, l)
			  if len(out) >= limit {
				   break
			   }
		   }
	   }
	  return out, nil
   }

   func (m *MemStore) ListExpiredAuthorized(_ context.Context, now time.Time, limit int) ([]FiatSettleLease, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if limit <= 0 {
		   limit = 100
	   }
	   out := make([]FiatSettleLease, 0, limit)
	   for _, l := range m.leases {
		  if l.State == LeaseAuthorized && !l.ExpiresAt.IsZero() && !now.Before(l.ExpiresAt) {
			   out = append(out, l)
			  if len(out) >= limit {
				   break
			   }
		   }
	  }
	  return out, nil
   }

  func (m *MemStore) ListCompensate(_ context.Context, limit int) ([]FiatSettleLease, error) {
	   return m.ListLeasesByState(context.Background(), LeaseCompensate, limit)
   }

  func (m *MemStore) InsertReservation(_ context.Context, r LimitsReservation) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  if _, ok := m.reserv[r.ReservationID]; ok {
		   return ErrIdem
	  }
	  m.reserv[r.ReservationID] = r
	  m.resBk[r.BookingID] = r.ReservationID
	   return nil
   }

   func (m *MemStore) GetReservation(_ context.Context, id string) (*LimitsReservation, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   r, ok := m.reserv[strings.TrimSpace(id)]
	  if !ok {
		  return nil, ErrReservationNotFound
	   }
	  cp := r
	   return &cp, nil
   }

   func (m *MemStore) GetReservationByBooking(_ context.Context, bookingID string) (*LimitsReservation, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  id, ok := m.resBk[strings.TrimSpace(bookingID)]
	   if !ok {
		  return nil, ErrReservationNotFound
	   }
	  r := m.reserv[id]
	   cp := r
	   return &cp, nil
  }

  func (m *MemStore) CASReservation(_ context.Context, id string, expected ReservationState, next LimitsReservation) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  cur, ok := m.reserv[id]
	   if !ok {
		   return ErrReservationNotFound
	  }
	  if cur.State != expected {
		  return ErrCAS
	   }
	  m.reserv[id] = next
	  return nil
   }

   func (m *MemStore) ListReservationsByState(_ context.Context, state ReservationState, limit int) ([]LimitsReservation, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  if limit <= 0 {
		   limit = 100
	   }
	   out := make([]LimitsReservation, 0)
	   for _, r := range m.reserv {
		   if r.State == state {
			   out = append(out, r)
			  if len(out) >= limit {
				  break
			   }
		  }
	  }
	   return out, nil
   }

   func (m *MemStore) ListOpenReservationsByUser(_ context.Context, userID, kind string) ([]LimitsReservation, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   out := make([]LimitsReservation, 0)
	   for _, r := range m.reserv {
		   if r.UserID == userID && r.Kind == kind && r.State == ResReserved {
			  out = append(out, r)
		  }
	   }
	  return out, nil
  }

   func (m *MemStore) InsertRefundLease(_ context.Context, r RefundLease) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if _, ok := m.refund[r.RefundLeaseID]; ok {
		   return ErrIdem
	   }
	  m.refund[r.RefundLeaseID] = r
	   m.refBk[r.BookingID] = r.RefundLeaseID
	  return nil
  }

  func (m *MemStore) GetRefundLease(_ context.Context, id string) (*RefundLease, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  r, ok := m.refund[strings.TrimSpace(id)]
	  if !ok {
		  return nil, ErrRefundLeaseNotFound
	  }
	   cp := r
	  return &cp, nil
   }

  func (m *MemStore) GetRefundLeaseByBooking(_ context.Context, bookingID string) (*RefundLease, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  id, ok := m.refBk[strings.TrimSpace(bookingID)]
	   if !ok {
		  return nil, ErrRefundLeaseNotFound
	  }
	   r := m.refund[id]
	  cp := r
	   return &cp, nil
  }

   func (m *MemStore) CASRefundLease(_ context.Context, id string, expected RefundLeaseState, next RefundLease) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   cur, ok := m.refund[id]
	  if !ok {
		  return ErrRefundLeaseNotFound
	   }
	  if cur.State != expected {
		  return ErrCAS
	   }
	  m.refund[id] = next
	  return nil
   }

   func (m *MemStore) InsertProof(_ context.Context, p FiatSettleProof) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  m.proofs[p.ProofID] = p
	   return nil
  }

  func (m *MemStore) GetProof(_ context.Context, id string) (*FiatSettleProof, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   p, ok := m.proofs[strings.TrimSpace(id)]
	  if !ok {
		   return nil, ErrProofNotFound
	  }
	  cp := p
	  return &cp, nil
  }

  func (m *MemStore) ListProofs(_ context.Context, limit int) ([]FiatSettleProof, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   if limit <= 0 {
		  limit = 200
	   }
	  out := make([]FiatSettleProof, 0, len(m.proofs))
	  for _, p := range m.proofs {
		  out = append(out, p)
		   if len(out) >= limit {
			   break
		  }
	  }
	  return out, nil
   }

  func (m *MemStore) ListProofsByLease(_ context.Context, leaseID string) ([]FiatSettleProof, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   out := make([]FiatSettleProof, 0)
	  for _, p := range m.proofs {
		   if p.LeaseID == leaseID {
			   out = append(out, p)
		  }
	  }
	  return out, nil
  }

  func (m *MemStore) MarkProofExported(_ context.Context, id string, now time.Time) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   p, ok := m.proofs[id]
	  if !ok {
		   return ErrProofNotFound
	   }
	   p.ExportedAt = &now
	  m.proofs[id] = p
	   return nil
   }

   func (m *MemStore) InsertAudit(_ context.Context, a AuditEvent) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   m.audits = append(m.audits, a)
	  return nil
   }

   func (m *MemStore) ListAuditsByLease(_ context.Context, leaseID string, limit int) ([]AuditEvent, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   if limit <= 0 {
		   limit = 100
	   }
	   out := make([]AuditEvent, 0)
	   for _, a := range m.audits {
		   if a.LeaseID == leaseID {
			   out = append(out, a)
			  if len(out) >= limit {
				  break
			  }
		  }
	  }
	  return out, nil
   }

   func (m *MemStore) ListAuditsByBooking(_ context.Context, bookingID string, limit int) ([]AuditEvent, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   if limit <= 0 {
		   limit = 100
	   }
	   out := make([]AuditEvent, 0)
	   for _, a := range m.audits {
		   if a.BookingID == bookingID {
			  out = append(out, a)
			   if len(out) >= limit {
				  break
			  }
		   }
	  }
	  return out, nil
   }

  func (m *MemStore) PutIdem(_ context.Context, key, resultID string) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  if key == "" {
		  return nil
	   }
	  if old, ok := m.idem[key]; ok && old != resultID {
		   return ErrIdem
	   }
	  m.idem[key] = resultID
	   return nil
   }

   func (m *MemStore) GetIdem(_ context.Context, key string) (string, bool, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  id, ok := m.idem[key]
	  return id, ok, nil
  }

   func (m *MemStore) TryLockBooking(_ context.Context, bookingID, holder, purpose string, now time.Time, ttl time.Duration) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if lk, ok := m.locks[bookingID]; ok {
		  if now.Before(lk.ExpiresAt) && lk.Holder != holder {
			  return ErrMutexSettleRefund
		  }
	  }
	   m.locks[bookingID] = BookingLock{
		   BookingID: bookingID, Holder: holder, Purpose: purpose, ExpiresAt: now.Add(ttl),
	  }
	   return nil
   }

   func (m *MemStore) UnlockBooking(_ context.Context, bookingID, holder string) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  lk, ok := m.locks[bookingID]
	  if !ok {
		   return nil
	  }
	   if lk.Holder != holder {
		  return ErrMutexSettleRefund
	   }
	   delete(m.locks, bookingID)
	  return nil
  }

   func (m *MemStore) GetBookingLock(_ context.Context, bookingID string) (string, string, bool) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   lk, ok := m.locks[bookingID]
	   if !ok {
		   return "", "", false
	  }
	  return lk.Holder,
		   lk.Purpose, true
   }

   func (m *MemStore) Snapshot() StoreSnapshot {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  s := StoreSnapshot{
		  Leases: make([]FiatSettleLease, 0, len(m.leases)),
		   Reservations: make([]LimitsReservation,
			   0, len(m.reserv)),
		  Refunds: make([]RefundLease, 0, len(m.refund)),
		  Proofs:  make([]FiatSettleProof, 0, len(m.proofs)),
		  Audits:  append([]AuditEvent{}, m.audits...),
		   Idem: make(map[string]string, len(m.idem)),
		   Locks: make([]BookingLock, 0, len(m.locks)),
	  }
	   for _, l := range m.leases {
		  s.Leases = append(s.Leases, l)
	   }
	   for _, r := range m.reserv {
		  s.Reservations = append(s.Reservations,
			   r)
	  }
	  for _, r := range m.refund {
		   s.Refunds = append(s.Refunds, r)
	   }
	  for _, p := range m.proofs {
		  s.Proofs = append(s.Proofs, p)
	  }
	   for k, v := range m.idem {
		   s.Idem[k] = v
	   }
	  for _, lk := range m.locks {
		  s.Locks = append(s.Locks, lk)
	  }
	   return s
   }

  func (m *MemStore) Restore(s StoreSnapshot) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   m.leases = map[string]FiatSettleLease{}
	  m.byBook = map[string]string{}
	  m.reserv = map[string]LimitsReservation{}
	  m.resBk = map[string]string{}
	  m.refund = map[string]RefundLease{}
	  m.refBk = map[string]string{}
	  m.proofs = map[string]FiatSettleProof{}
	   m.idem = map[string]string{}
	   m.locks = map[string]BookingLock{}
	  m.audits = append([]AuditEvent{}, s.Audits...)
	  for _, l := range s.Leases {
		   m.leases[l.LeaseID] = l
		   m.byBook[l.BookingID] = l.LeaseID
	  }
	   for _, r := range s.Reservations {
		  m.reserv[r.ReservationID] = r
		  m.resBk[r.BookingID] = r.ReservationID
	  }
	   for _, r := range s.Refunds {
		  m.refund[r.RefundLeaseID] = r
		  m.refBk[r.BookingID] = r.RefundLeaseID
	  }
	  for _, p := range s.Proofs {
		  m.proofs[p.ProofID] = p
	   }
	   for k, v := range s.Idem {
		   m.idem[k] = v
	   }
	  for _, lk := range s.Locks {
		  m.locks[lk.BookingID] = lk
	  }
   }
