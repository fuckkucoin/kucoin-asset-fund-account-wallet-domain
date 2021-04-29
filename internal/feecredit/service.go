package feecredit

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/feeeng"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Service struct {
	mu     sync.Mutex
	rows   map[string]*Credit
	byIdem map[string]string
	daily map[string]money.Amount
	cap  money.Amount
	fees *feeeng.Service
	led   *ledger.Service
	kv    persist.KV
	clock func() time.Time
}

   func New() *Service {
	   cap, _ := money.Parse("100000")
	   return &Service{
		   rows: map[string]*Credit{}, byIdem: map[string]string{},
		   daily: map[string]money.Amount{}, cap: cap, fees: feeeng.New(),
		   kv: persist.NewMemKV(),
		   clock: func() time.Time { return time.Now().UTC() },
	   }
   }

  func (s *Service) BindLedger(led *ledger.Service) *Service {
	  if s != nil {
		   s.led = led
	   }
	  return s
   }

  func (s *Service) SetCap(cap string) error {
	   if s == nil {
		   return ErrUnavailable
	   }
	  amt, err := money.Parse(cap)
	  if err != nil {
		  return ErrAmount
	  }
	  s.mu.Lock()
	  s.cap = amt
	   s.mu.Unlock()
	  return nil
  }

   func (s *Service) now() time.Time {
	   if s == nil || s.clock == nil {
		  return time.Now().UTC()
	   }
	   return s.clock()
   }

  func dayKey(user, asset string, at time.Time) string {
	  return user + "|" + asset + "|" + at.UTC().Format("2006-01-02")
  }

   func (s *Service) Post(ctx context.Context, req PostReq) (Applied, error) {
	  if s == nil {
		  return Applied{}, ErrUnavailable
	  }
	   uid, err := validate.UserID(req.UserID)
	   if err != nil {
		   return Applied{}, ErrUser
	  }
	   kind, err := ParseKind(req.Kind)
	   if err != nil {
		  return Applied{}, err
	   }
	   as, err := validate.Asset(req.Asset)
	  if err != nil {
		  return Applied{}, ErrAsset
	   }
	  if _, err := money.Parse(req.Gross); err != nil {
		  return Applied{}, ErrAmount
	   }
	   idem,
		  err := validate.Idem(req.IdempotencyKey)
	  if err != nil {
		  return Applied{}, ErrIdem
	   }
	  feeKind := req.FeeKind
	   if feeKind == "" {
		   if kind == KindRebate {
			  feeKind = "withdraw"
		  } else {
			   feeKind = "transfer"
		   }
	  }
	   q, err := s.fees.Quote(feeeng.QuoteReq{Kind: feeKind, Asset: as, Gross: req.Gross})
	   if err != nil {
		   return Applied{}, fmtQuote(err)
	  }
	  var lines []money.Line
	  switch kind {
	   case KindTake:
		   lines, err = journalTake(as, q)
	  default:
		   lines, err = journalRebate(as, q)
	   }
	   if err != nil {
		   return Applied{}, err
	   }
	   s.mu.Lock()
	   if id, ok := s.byIdem[idem]; ok {
		   cur := s.rows[id]
		  s.mu.Unlock()
		   if cur.UserID != uid || cur.Asset != as || cur.Gross != q.Gross || cur.Kind != kind {
			  return Applied{}, ErrIdem
		   }
		   return Applied{Credit: cur.Copy(),
			   Quote: q}, nil
	   }
	   if kind == KindRebate {
		  dk := dayKey(uid, as, s.now())
		  used := s.daily[dk]
		   add, _ := money.ParseBook(q.Fee)
		  next, err := used.Add(add)
		  if err != nil || next.Cmp(s.cap) > 0 {
			   s.mu.Unlock()
			  return Applied{}, ErrCap
		   }
		  s.daily[dk] = next
	   }
	   s.mu.Unlock()
	  ledgerID := ""
	  if s.led != nil && kind == KindRebate {
		  amt, err := money.ParseBook(q.Fee)
		   if err == nil && amt.IsPositive() {
			   tr, err := s.led.Create(ctx, ledger.CreateReq{
				   UserID: uid, Direction: ledger.FiatOnramp, Asset: as,
				  Amount: amt.Dec(), IdempotencyKey: "feecredit:" + idem, WaitConfirm: true,
			  })
			   if err == nil {
				  ledgerID = tr.ID
			   }
		  }
	   }
	   row := &Credit{
		   ID: uuid.NewString(), Idem: idem, UserID: uid, Kind: kind, Asset: as,
		  Gross: q.Gross, Fee: q.Fee, Net: q.Net, QuoteID: q.Schedule, LedgerID: ledgerID,
		   Lines:     lines,
		  CreatedAt: s.now(),
	  }
	  s.mu.Lock()
	   s.rows[row.ID] = row
	   s.byIdem[idem] = row.ID
	   s.mu.Unlock()
	  _ = ctx
	   return Applied{Credit: row.Copy(), Quote: q}, nil
  }

  func fmtQuote(err error) error {
	  return err
  }

   func (s *Service) Get(id string) (Credit, error) {
	   if s == nil {
		  return Credit{}, ErrUnavailable
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   row, ok := s.rows[strings.TrimSpace(id)]
	   if !ok {
		  return Credit{}, ErrUnavailable
	   }
	   return row.Copy(), nil
  }

  func (s *Service) List(userID string, limit int) []Credit {
	  if s == nil {
		  return nil
	   }
	   if limit <= 0 {
		   limit = 50
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   out := make([]Credit, 0, limit)
	   for _, r := range s.rows {
		   if userID != "" && r.UserID != userID {
			  continue
		  }
		  out = append(out, r.Copy())
		  if len(out) >= limit {
			  break
		  }
	   }
	  return out
   }

const kindFee = "feecredit"

  func (s *Service) Save(ctx context.Context, c Credit) (Credit, error) {
	  if s == nil || s.kv == nil {
		   return Credit{}, ErrUnavailable
	   }
	  body, err := json.Marshal(c)
	  if err != nil {
		  return Credit{}, err
	  }
	   err = s.kv.InTx(ctx, func(ctx context.Context) error {
		  cur, ferr := s.kv.Get(ctx, kindFee, c.Idem)
		  exp := persist.Fence{}
		  if ferr == nil {
			  exp = cur.Fence
		   } else if !errors.Is(ferr, persist.ErrNotFound) {
			   return ferr
		  }
		   return s.kv.Put(ctx, persist.Row{Kind: kindFee, Key: c.Idem, Body: body}, exp)
	  })
	  if err != nil {
		   if errors.Is(err, persist.ErrStaleFence) {
			  return Credit{}, ErrFence
		   }
		  return Credit{}, err
	   }
	  got, err := s.kv.Get(ctx, kindFee, c.Idem)
	   if err != nil {
		   return Credit{}, err
	   }
	  var out Credit
	   if err := json.Unmarshal(got.Body, &out); err != nil {
		  return Credit{}, err
	   }
	  out.Fence = got.Fence
	   return out, nil
  }
