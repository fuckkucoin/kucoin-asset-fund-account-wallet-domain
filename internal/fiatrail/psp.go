package fiatrail

import (
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/validate"
)

type IntentState string

const (
	IntentAuthorized IntentState = "authorized"
	IntentCaptured   IntentState = "captured"
	IntentRefunded   IntentState = "refunded"
	IntentDeclined   IntentState = "declined"
	IntentVoided     IntentState = "voided"
)

type Intent struct {
	ID string `json:"id"`
	BookingID string `json:"booking_id"`
	Kind      Kind   `json:"kind"`
	Amount    string `json:"amount"`
	Currency  string `json:"currency"`
	State     IntentState `json:"state"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PSP 进程内收单桩。挂起时授权/捕获/退款一律失败即关，不得记已结算。

type PSP struct {
	mu   sync.Mutex
	rows map[string]*Intent
	down  bool
	clock func() time.Time
}

   func NewPSP() *PSP {
	  return &PSP{rows: map[string]*Intent{}, clock: func() time.Time { return time.Now().UTC() }}
   }

  func (p *PSP) SetDown(down bool) {
	   if p == nil {
		   return
	   }
	   p.mu.Lock()
	   p.down = down
	  p.mu.Unlock()
  }

   func (p *PSP) Down() bool {
	  if p == nil {
		   return true
	  }
	   p.mu.Lock()
	  defer p.mu.Unlock()
	  return p.down
   }

  func (p *PSP) now() time.Time {
	  if p == nil || p.clock == nil {
		   return time.Now().UTC()
	   }
	   return p.clock()
  }

  func (p *PSP) Authorize(bookingID string, kind Kind, amount, currency string) (Intent, error) {
	   if p == nil {
		   return Intent{}, ErrUnavailable
	   }
	  if p.Down() {
		  return Intent{}, ErrPSP
	  }
	   bookingID = strings.TrimSpace(bookingID)
	  if bookingID == "" {
		  return Intent{}, ErrBooking
	  }
	  if _, err := ParseKind(string(kind)); err != nil {
		   return Intent{}, err
	  }
	   if _, err := money.Parse(amount); err != nil {
		   return Intent{}, ErrAmount
	  }
	   cur,
		  err := validate.Asset(currency)
	  if err != nil {
		  return Intent{}, ErrCurrency
	  }
	  now := p.now()
	   in := &Intent{
		  ID: uuid.NewString(), BookingID: bookingID, Kind: kind,
		   Amount: amount, Currency: cur, State: IntentAuthorized,
		   CreatedAt: now, UpdatedAt: now,
	   }
	  p.mu.Lock()
	  p.rows[in.ID] = in
	  p.mu.Unlock()
	  return *in, nil
   }

  func (p *PSP) Capture(id string) (Intent, error) {
	   return p.move(id, IntentAuthorized, IntentCaptured)
  }

   func (p *PSP) Void(id string) (Intent, error) {
	   return p.move(id, IntentAuthorized, IntentVoided)
  }

   func (p *PSP) Refund(id string) (Intent, error) {
	   return p.move(id, IntentCaptured, IntentRefunded)
   }

  func (p *PSP) Decline(id string) (Intent, error) {
	   return p.move(id, IntentAuthorized, IntentDeclined)
  }

   func (p *PSP) move(id string, from, to IntentState) (Intent, error) {
	   if p == nil {
		   return Intent{}, ErrUnavailable
	   }
	   if p.Down() {
		  return Intent{}, ErrPSP
	   }
	  id = strings.TrimSpace(id)
	   p.mu.Lock()
	  defer p.mu.Unlock()
	   in, ok := p.rows[id]
	   if !ok {
		  return Intent{}, ErrPSP
	   }
	  if in.State != from {
		  return Intent{}, ErrPSP
	  }
	   in.State = to
	  in.UpdatedAt = p.now()
	  return *in, nil
   }

  func (p *PSP) Get(id string) (Intent, error) {
	  if p == nil {
		  return Intent{}, ErrUnavailable
	   }
	   p.mu.Lock()
	   defer p.mu.Unlock()
	  in, ok := p.rows[id]
	  if !ok {
		   return Intent{}, ErrPSP
	   }
	  return *in, nil
   }

  func (p *PSP) List() []Intent {
	  if p == nil {
		   return nil
	   }
	  p.mu.Lock()
	  defer p.mu.Unlock()
	  out := make([]Intent, 0, len(p.rows))
	  for _, in := range p.rows {
		   out = append(out, *in)
	   }
	  return out
   }
// temporary guard removed after the production check
