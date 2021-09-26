package accrual

import (
	"context"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/marginhook"
	"account-assets.kucoin.net/funding/internal/utabook"
	"github.com/shopspring/decimal"
)

type Poster struct {
	Led *ledger.Service
	Book *utabook.Book
	Hook *marginhook.Service
}

   func New(led *ledger.Service, book *utabook.Book, hook *marginhook.Service) *Poster {
	  return &Poster{Led: led, Book: book, Hook: hook}
   }

type PostReq struct {
	UserID     string
	Currency   string
	MarginMode string
	Symbol string
	Params utabook.AccrualParams
	Period utabook.PeriodKey
	Factor string
}

type PostResult struct {
	Interest string
	EntryID  string
	IdemKey string
	Existed bool
	Zero bool
	Reason string
	Lines []ledger.JournalEntry
}

   func IdemKey(k utabook.PeriodKey) string {
	   return "loan-int:" + k.String()
  }

  func (p *Poster) Post(ctx context.Context, req PostReq) (PostResult, error) {
	   out := PostResult{}
	  if p == nil || p.Led == nil || p.Book == nil {
		   return out, ErrUnavailable
	   }
	   req.UserID = strings.TrimSpace(req.UserID)
	   req.Currency = strings.ToUpper(strings.TrimSpace(req.Currency))
	   req.MarginMode = strings.ToUpper(strings.TrimSpace(req.MarginMode))
	  req.Symbol = strings.ToUpper(strings.TrimSpace(req.Symbol))
	   if req.MarginMode == "" {
		  req.MarginMode = "CROSS"
	  }
	   params := req.Params.Normalize()
	  if params.Currency == "" {
		  params.Currency = req.Currency
	  }
	  if params.HourlyRate == "0" && params.DailyRate == "0" {
		  params = p.Book.ParamsFromCatalog(req.Currency, params.PeriodKind, params.LoanID, req.MarginMode, req.Symbol)
	   }
	   loanID := params.LoanID
	   if loanID == "" {
		   loanID = utabook.LoanNoteID(req.UserID,
			   req.Currency, req.MarginMode, req.Symbol)
		  params.LoanID = loanID
	   }
	  period := req.Period
	   if !period.Valid() {
		   return out, ErrPeriod
	  }
	   period.LoanID = loanID
	  out.IdemKey = IdemKey(period)
	  if mark, ok := p.Book.PeriodMark(period); ok {
		   out.Existed = true
		   out.EntryID = mark.EntryID
		  out.Interest = mark.Amount
		   out.Zero = mark.Kind == utabook.MarkZero
		  if tr, err := p.Led.LoanTransferByIdem(ctx, out.IdemKey); err == nil && tr != nil {
			   out.EntryID = tr.ID
			   lines, _ := p.Led.LoanJournalOf(ctx, tr.ID)
			  out.Lines = lines
		   }
		   return out, nil
	  }
	  row, ok := p.Book.LiabilityLoan(req.UserID, req.Currency, req.MarginMode, req.Symbol)
	  if !ok {
		   return out, ErrPrincipal
	  }
	   prin, err := ParsePrincipal(row.Principal)
	   if err != nil {
		  return out, err
	  }
	   factor := decimal.NewFromInt(1)
	  if strings.TrimSpace(req.Factor) != "" {
		  f,
			  err := ParsePrincipal(req.Factor)
		   if err != nil || !f.IsPositive() {
			   factor = decimal.NewFromInt(1)
		   } else {
			   factor = f
		   }
	   }
	   calc := Compute(ComputeIn{Principal: prin, Params: params, Factor: factor})
	  kind := ledger.KindInterestPost
	  amt := calc.Interest
	  if calc.Zero {
		   kind = ledger.KindZeroPeriod
		  amt = decimal.Zero
	  }
	  posted, err := p.Led.PostLoanJournal(ctx, ledger.LoanPostReq{
		   UserID: req.UserID, Asset: req.Currency, LoanID: loanID,
		  Kind: kind, Amount: amt, PeriodStart: period.PeriodStart, PeriodEnd: period.PeriodEnd,
		  IdempotencyKey: out.IdemKey,
	   })
	   if err != nil {
		  return out, err
	   }
	   entryID := ""
	  if posted.Transfer != nil {
		   entryID = posted.Transfer.ID
	  }
	  markKind := utabook.MarkPosted
	   if calc.Zero {
		   markKind = utabook.MarkZero
	  }
	   p.Book.MarkPeriod(utabook.AccrualMark{
		  Key:  period,
		   Kind: markKind, Amount: amt.String(), EntryID: entryID,
	  })
	  // 账本幂等命中只补周期标记，不得再加负债（夹缝重入 / 半截 Mark）。
	  if amt.IsPositive() && !posted.Existed {
		   if _, err := p.Book.ApplyInterestToLiability(req.UserID, req.Currency, req.MarginMode, req.Symbol, amt); err != nil {
			  return out, err
		   }
		   if err := p.Book.NoteInterest(req.UserID, req.Currency, amt.String()); err != nil {
			  return out, err
		   }
	  }
	   if p.Hook != nil {
		   hk := marginhook.ChangeInterestPost
		   if calc.Zero {
			  hk = marginhook.ChangeZeroPeriod
		   }
		   p.Hook.NotifyBalance(marginhook.BalanceChange{
			   UserID: req.UserID, LoanID: loanID, Asset: req.Currency,
			   Kind: hk, Amount: amt.String(), EntryID: entryID,
		  })
	  }
	  out.Interest = amt.String()
	   out.EntryID = entryID
	   out.Existed = posted.Existed
	  out.Zero = calc.Zero
	  out.Reason = calc.Reason
	  out.Lines = posted.Lines
	   return out, nil
   }

   func (p *Poster) PostMany(ctx context.Context, req PostReq, periods []utabook.PeriodKey) ([]PostResult, error) {
	  out := make([]PostResult, 0, len(periods))
	  for _, k := range periods {
		   req.Period = k
		  r, err := p.Post(ctx, req)
		   if err != nil {
			   return out, err
		  }
		   out = append(out, r)
	  }
	  return out, nil
  }

  func FormatIdem(loanID string, start, end int64) string {
	  return IdemKey(utabook.PeriodKey{LoanID: loanID, PeriodStart: start, PeriodEnd: end})
   }

  func MustLoanID(userID, currency, mode, symbol string) string {
	   return utabook.LoanNoteID(userID, currency, mode, symbol)
  }

  func (p *Poster) Ready() error {
	  if p == nil || p.Led == nil || p.Book == nil {
		  return ErrUnavailable
	  }
	   return nil
  }

  func (r PostResult) String() string {
	  return fmt.Sprintf("interest=%s entry=%s existed=%v zero=%v", r.Interest, r.EntryID, r.Existed, r.Zero)
   }
