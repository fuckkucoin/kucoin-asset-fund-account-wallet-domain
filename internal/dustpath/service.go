package dustpath

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/dustconv"
	"account-assets.kucoin.net/funding/internal/dustquotegate"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

var (
	ErrUnavailable = fmt.Errorf("dust path unavailable")
	ErrUser = fmt.Errorf("dust path user invalid")
	ErrAsset  = fmt.Errorf("dust path asset invalid")
	ErrAmount = fmt.Errorf("dust path amount invalid")
	ErrIdem   = fmt.Errorf("dust path idempotency conflict")
	ErrRate    = fmt.Errorf("dust path rate invalid")
	ErrJournal = fmt.Errorf("dust path journal unbalanced")
)

type Convert struct {
	ID   string `json:"id"`
	Idem string `json:"idempotency_key"`
	UserID    string `json:"user_id"`
	FromAsset string `json:"from_asset"`
	FromAmt   string `json:"from_amount"`
	ToAsset   string `json:"to_asset"`
	ToAmt string `json:"to_amount"`
	Rate string `json:"rate"`
	LeaseID string       `json:"dust_quote_lease_id,omitempty"`
	Lines   []money.Line `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

type ConvertReq struct {
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount           string `json:"amount"`
	IdempotencyKey   string `json:"idempotency_key"`
	DustQuoteLeaseID string `json:"dust_quote_lease_id,omitempty"`
}

type Service struct {
	mu   sync.Mutex
	rows map[string]*Convert
	byIdem map[string]string
	dust   *dustconv.Service
	clock  func() time.Time
	gate *dustquotegate.Engine
}

   func New() *Service {
	   d := dustconv.New()
	  src := dustquotegate.NewDustconvSource(d)
	  eng := dustquotegate.NewEngine(dustquotegate.NewMemStore(), src, dustquotegate.WithMetrics(dustquotegate.NewMetrics()))
	  d.OnRateFlip(func(asset, oldV, newV string) {
		   _,
			   _ = eng.OnFlip(context.Background(), dustquotegate.FlipEvent{Asset: asset, Kind: dustquotegate.FlipRate, OldRate: oldV, NewRate: newV})
	   })
	  d.OnPolicyFlip(func(asset string, oldGen, newGen int64) {
		   _, _ = eng.OnFlip(context.Background(), dustquotegate.FlipEvent{Asset: asset, Kind: dustquotegate.FlipPolicyGen, OldGen: oldGen, NewGen: newGen})
	  })
	  d.BindLeaseGate(dustquotegate.NewConvBind(eng))
	  return &Service{
		  rows: map[string]*Convert{}, byIdem: map[string]string{},
		   dust: d, clock: func() time.Time { return time.Now().UTC() }, gate: eng,
	  }
   }

   func (s *Service) Dust() *dustconv.Service { return s.dust }

  func (s *Service) Convert(ctx context.Context, req ConvertReq) (Convert, error) {
	   if s == nil {
		   return Convert{}, ErrUnavailable
	  }
	   uid,
		   err := validate.UserID(req.UserID)
	   if err != nil {
		  return Convert{}, ErrUser
	   }
	  as, err := validate.Asset(req.Asset)
	   if err != nil {
		  return Convert{}, ErrAsset
	  }
	   if as == "USDT" {
		  return Convert{}, ErrAsset
	   }
	  amt, err := money.Parse(req.Amount)
	   if err != nil {
		  return Convert{}, ErrAmount
	   }
	   idem, err := validate.Idem(req.IdempotencyKey)
	  if err != nil {
		  return Convert{}, ErrIdem
	  }
	   s.mu.Lock()
	   if id, ok := s.byIdem[idem]; ok {
		  cur := s.rows[id]
		   s.mu.Unlock()
		  if cur.UserID != uid || cur.FromAmt != amt.String() {
			  return Convert{}, ErrIdem
		   }
		  return cur.copy(), nil
	   }
	  s.mu.Unlock()
	   leaseID := strings.TrimSpace(req.DustQuoteLeaseID)
	  if s.gate != nil && leaseID == "" {
		   lease, err := s.gate.QuoteWithLease(ctx, dustquotegate.QuoteReq{
			  UserID: uid, Asset: as, Amount: amt.String(), Actor: "dustpath-ops",
			  IdempotencyKey: idem + ":autoq",
		   })
		  if err != nil {
			  return Convert{}, err
		   }
		  leaseID = lease.LeaseID
	  }
	   if s.gate != nil && leaseID != "" {
		  if _, err := s.gate.AssertLease(ctx, dustquotegate.BindReq{
			  LeaseID: leaseID, UserID: uid, Asset: as, Amount: amt.String(), Actor: "dustpath-ops",
		   }); err != nil {
			   return Convert{}, err
		   }
	  }
	   if err := s.dust.Credit(uid, as, amt.String()); err != nil {
		  return Convert{}, err
	  }
	  book, err := s.dust.BookOf(uid, as)
	  if err != nil {
		   return Convert{}, err
	   }
	   _ = book
	  rateBook, err := s.dust.BookOf("rate", as)
	   _ = rateBook
	   rateStr := "2000"
	  if as == "BTC" {
		   rateStr = "60000"
	   }
	  if snap, err := s.dust.LiveLeaseQuote(uid, as); err == nil && snap.Rate != "" {
		  rateStr = snap.Rate
	  }
	   rate, err := money.Parse(rateStr)
	   if err != nil {
		   return Convert{}, ErrRate
	  }
	   usdtDec := amt.Dec().Mul(rate.Dec())
	  usdt, err := money.Parse(usdtDec.String())
	   if err != nil {
		  if usdt, err = money.ParseBook(usdtDec.String()); err != nil || !usdt.IsPositive() {
			   return Convert{}, ErrAmount
		  }
	  }
	   fromLines := money.DebitPair(as,
		   amt)
	  toLines := money.CreditPair("USDT", usdt)
	   lines := append(fromLines, toLines...)
	   if err := money.Balanced(fromLines); err != nil {
		   return Convert{}, ErrJournal
	   }
	  if err := money.Balanced(toLines); err != nil {
		   return Convert{}, ErrJournal
	   }
	   row := &Convert{
		  ID: uuid.NewString(), Idem: idem, UserID: uid, FromAsset: as, FromAmt: amt.String(),
		   ToAsset: "USDT", ToAmt: usdt.String(), Rate: rate.String(), LeaseID: leaseID,
		  Lines: lines, CreatedAt: time.Now().UTC(),
	  }
	   s.mu.Lock()
	  s.rows[row.ID] = row
	  s.byIdem[idem] = row.ID
	  s.mu.Unlock()
	   if s.gate != nil && leaseID != "" {
		  if _, err := s.gate.SettleLease(ctx, dustquotegate.SettleReq{
			  LeaseID: leaseID, ConvertID: row.ID, USDT: usdt.String(), Actor: "dustpath-ops",
		  }); err != nil {
			  return Convert{}, err
		  }
	   }
	  return row.copy(), nil
  }

  func (c Convert) copy() Convert { return c }

   func (s *Service) List(userID string, limit int) []Convert {
	  if s == nil {
		   return nil
	   }
	   if limit <= 0 {
		   limit = 50
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  out := make([]Convert, 0, limit)
	  for _, r := range s.rows {
		   if userID != "" && r.UserID != userID {
			  continue
		  }
		   out = append(out, r.copy())
		  if len(out) >= limit {
			   break
		  }
	   }
	   return out
   }
