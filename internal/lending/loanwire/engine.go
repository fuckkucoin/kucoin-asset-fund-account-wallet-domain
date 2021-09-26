package loanwire

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/lending/accrual"
	"account-assets.kucoin.net/funding/internal/lending/repay"
	"account-assets.kucoin.net/funding/internal/marginhook"
	"account-assets.kucoin.net/funding/internal/utabook"
	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
)

type Engine struct {
	Book *utabook.Book
	Hook *marginhook.Service
	Led     *ledger.Service
	Store   *ledger.MemStore
	Accrual *accrual.Poster
	Repay *repay.Service
}

   func NewEngine() *Engine {
	  book := utabook.New()
	  hook := marginhook.New()
	   st := ledger.NewMemStore()
	  led := ledger.NewService(st, nil, ledger.MemLocker{}, nil, nil, zerolog.Nop(), config.TransferConfig{DisableDriftBlock: true})
	  return &Engine{
		   Book: book, Hook: hook, Led: led, Store: st,
		  Accrual: accrual.New(led,
			   book, hook),
		   Repay: repay.New(led, book, hook),
	  }
   }

  func (e *Engine) SeedCash(userID, asset, amount string) {
	   if e == nil || e.Store == nil {
		  return
	  }
	   e.Store.Seed(userID, ledger.AccountSpot, strings.ToUpper(strings.TrimSpace(asset)), amount)
   }

   func (e *Engine) OpenLoan(userID string, row utabook.LiabilityRow, tier utabook.RateTier) (utabook.LiabilityRow, error) {
	  if e == nil || e.Book == nil {
		   return row, nil
	  }
	   if err := e.Book.PutRateTier(tier); err != nil {
		  return utabook.LiabilityRow{}, err
	  }
	   row.BorrowEnabled = true
	  return e.Book.UpsertLiability(userID, row)
  }

   func (e *Engine) Params(userID, currency, mode, symbol, periodKind string) utabook.AccrualParams {
	   p := e.Book.ParamsFromCatalog(currency, periodKind, "", mode, symbol)
	   p.LoanID = utabook.LoanNoteID(userID, currency, mode, symbol)
	   return p
   }

   func (e *Engine) PostPeriod(ctx context.Context, userID, currency, mode, symbol string, period utabook.PeriodKey, factor string) (accrual.PostResult, error) {
	   p := e.Params(userID,
		  currency, mode, symbol, period.Kind)
	   if period.Kind == "" {
		  period.Kind = p.PeriodKind
	   }
	  return e.Accrual.Post(ctx, accrual.PostReq{
		   UserID: userID, Currency: currency, MarginMode: mode, Symbol: symbol,
		   Params: p, Period: period, Factor: factor,
	   })
  }

   func (e *Engine) Offset(ctx context.Context, userID, currency, mode, symbol, amount, repayID string, policy repay.Policy) (repay.Result, error) {
	   return e.Repay.Offset(ctx, repay.Req{
		   UserID: userID, Currency: currency, MarginMode: mode, Symbol: symbol,
		   Amount: amount, Policy: policy, RepayID: repayID,
	  })
  }

   func (e *Engine) Liability(userID, currency, mode, symbol string) utabook.LiabilityRow {
	  row, _ := e.Book.LiabilityLoan(userID, currency, mode, symbol)
	  return row
   }

   func (e *Engine) HookCount(kind marginhook.BalanceChangeKind) int {
	  return e.Hook.BalanceCallCount(kind)
  }

   func MustDec(s string) decimal.Decimal {
	  v, err := decimal.NewFromString(strings.TrimSpace(s))
	   if err != nil {
		   return decimal.Zero
	  }
	  return v
  }
