package daycut

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/lending/accrual"
	"account-assets.kucoin.net/funding/internal/lending/repay"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/utabook"
	"github.com/shopspring/decimal"
)

type Engine struct {
	Acc *accrual.Poster
	Rep *repay.Service
	Book *utabook.Book
	Led  *ledger.Service
	St   Store
	Cfg  Config
}

  func New(acc *accrual.Poster, book *utabook.Book, led *ledger.Service, st Store, cfg Config) *Engine {
	  if st == nil {
		   st = NewMemStore()
	   }
	   if cfg.Profile == "" {
		  cfg = Lab()
	  }
	  return &Engine{Acc: acc, Book: book, Led: led, St: st, Cfg: cfg}
   }

  func (e *Engine) Ready() error {
	   if e == nil || e.Acc == nil || e.Book == nil || e.St == nil {
		  return ErrUnavailable
	  }
	   return e.Acc.Ready()
  }

type RunReq struct {
	UserID     string
	Currency   string
	MarginMode string
	Symbol     string
	Params     utabook.AccrualParams
	CutMs int64
	Kind  string
	Require bool // 对账路径：批次必须已存在
	Rebatch bool // 显式重批（禁止）
}

type RunResult struct {
	Key     BatchKey
	Rec     BatchRec
	Posted  int
	Skip    int
	Existed bool
	Recovered bool
	Total string
	Items []accrual.PostResult
}

   func (e *Engine) Run(ctx context.Context, req RunReq) (RunResult, error) {
	  out := RunResult{}
	  if err := e.Ready(); err != nil {
		   return out, err
	  }
	   req.UserID = strings.TrimSpace(req.UserID)
	  req.Currency = strings.ToUpper(strings.TrimSpace(req.Currency))
	  req.MarginMode = strings.ToUpper(strings.TrimSpace(req.MarginMode))
	  if req.MarginMode == "" {
		  req.MarginMode = "CROSS"
	   }
	   kind := strings.ToUpper(strings.TrimSpace(req.Kind))
	   if kind == "" {
		  kind = "DAY"
	  }
	   if req.CutMs <= 0 {
		   return out, ErrWindow
	  }
	   loanID := utabook.LoanNoteID(req.UserID, req.Currency, req.MarginMode, req.Symbol)
	  key := KeyOf(loanID, kind, req.CutMs)
	   if !key.Valid() {
		   return out, ErrBatchKey
	  }
	  out.Key = key
	   prev, found, err := e.St.Get(key)
	  if err != nil {
		  return out, err
	   }
	   if req.Require && !found {
		  return out, ErrMissingBatch
	   }
	   if req.Rebatch {
		  if err := e.Cfg.RefuseRebatch(); err != nil {
			  return out, err
		   }
	  }
	  if found && prev.Status == StatusPosted && req.Rebatch {
		  return out, ErrRebatch
	  }
	  if found && prev.Status == StatusPosted {
		  out.Rec = prev
		   out.Existed = true
		  out.Skip = len(prev.Periods)
		   out.Total = prev.AccruedTotal
		   return out, nil
	  }
	   if found && prev.Status == StatusOpen {
		   out.Recovered = true
	  }
	   dayStart := utabook.AlignDayUTC(req.CutMs)
	   dayEnd := dayStart + utabook.PeriodSpanMs("DAY")
	   if kind == "HOUR" {
		   // 日切窗口内按小时扫未处理键；DAY 则单周期。
	   }
	   scanKind := kind
	  if kind != "HOUR" {
		   scanKind = "DAY"
	   }
	  periods := e.Book.UnprocessedThrough(loanID,
		   scanKind, dayStart, dayEnd)
	  if kind == "DAY" {
		   pk := utabook.PeriodOf(loanID, "DAY", dayStart)
		   if !e.Book.PeriodProcessed(pk) {
			   periods = []utabook.PeriodKey{pk}
		  } else {
			   periods = nil
		  }
	   }
	   if found && len(prev.Periods) > 0 && len(periods) == 0 && prev.Status == StatusOpen {
		   // 重启重入：周期已过账但批次未收口 → 只收口，不补记。
		   prev.Status = StatusPosted
		  if prev.AccruedTotal == "" {
			  prev.AccruedTotal = "0"
		   }
		   if err := e.St.Save(prev); err != nil {
			  return out, err
		  }
		   out.Rec = prev
		   out.Recovered = true
		  out.Total = prev.AccruedTotal
		   return out, nil
	   }
	  if !found {
		  open := BatchRec{Key: key, Status: StatusOpen, Periods: periods, AccruedTotal: "0", Attempt: 1}
		  if err := e.St.Save(open); err != nil {
			  return out, err
		  }
		  prev = open
	  } else {
		   prev.Attempt++
		  prev.Periods = periods
		   if err := e.St.Save(prev); err != nil {
			  return out, err
		  }
	   }
	  sum, _ := decimal.NewFromString("0")
	  for _, pk := range periods {
		   r, err := e.Acc.Post(ctx, accrual.PostReq{
			  UserID: req.UserID, Currency: req.Currency, MarginMode: req.MarginMode, Symbol: req.Symbol,
			  Params: req.Params, Period: pk,
		  })
		  if err != nil {
			   prev.Status = StatusFailed
			  _ = e.St.Save(prev)
			  return out, err
		  }
		   out.Items = append(out.Items, r)
		   if r.EntryID != "" {
			   prev.EntryIDs = append(prev.EntryIDs, r.EntryID)
		   }
		   if r.Existed {
			  out.Skip++
			   continue
		   }
		  out.Posted++
		   if v, err := decimal.NewFromString(r.Interest); err == nil {
			  sum = sum.Add(v)
		  }
	  }
	   prev.AccruedTotal = sum.String()
	   prev.Status = StatusPosted
	   prev.Periods = periods
	  if err := e.St.Save(prev); err != nil {
		   return out, err
	   }
	  out.Rec = prev
	   out.Total = prev.AccruedTotal
	  return out, nil
   }

// RequireBatch 对账前取批次：缺失 fail-closed，禁止漏记后补。

  func (e *Engine) RequireBatch(key BatchKey) (BatchRec, error) {
	  if e == nil || e.St == nil {
		  return BatchRec{}, ErrUnavailable
	   }
	   if !key.Valid() {
		  return BatchRec{}, ErrBatchKey
	  }
	   rec, ok, err := e.St.Get(key)
	   if err != nil {
		   return BatchRec{}, err
	   }
	   if !ok {
		  return BatchRec{}, ErrMissingBatch
	   }
	  return rec, nil
   }

// RollbackPosted 显式回退已过账批次；锁死。

   func (e *Engine) RollbackPosted(key BatchKey) error {
	   if e == nil {
		  return ErrUnavailable
	  }
	   if err := e.Cfg.RefuseRollback(); err != nil {
		   return err
	  }
	   rec, ok, err := e.St.Get(key)
	   if err != nil {
		   return err
	   }
	  if !ok {
		  return ErrMissingBatch
	  }
	   rec.Status = StatusOpen
	  return e.St.Save(rec)
  }
