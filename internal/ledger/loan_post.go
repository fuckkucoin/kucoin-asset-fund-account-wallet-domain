package ledger

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// LoanPostReq 利息或还款分录入参。利率不在此结构出现。

type LoanPostReq struct {
	UserID string
	Asset        string
	LoanID       string
	Kind         LoanJournalKind
	Amount       decimal.Decimal
	InterestPart decimal.Decimal
	PrincipalPart  decimal.Decimal
	SurplusPart    decimal.Decimal
	PeriodStart    int64
	PeriodEnd      int64
	IdempotencyKey string
}

// LoanPostResult 过账结果。Existed=true 表示幂等命中。
type LoanPostResult struct {
	Transfer *Transfer
	Lines    []JournalEntry
	Existed  bool
	Skipped bool // 零利息：已处理周期、无金额分录
}

  func (r LoanPostReq) normalize() (LoanPostReq, error) {
	  r.UserID = strings.TrimSpace(r.UserID)
	  r.Asset = strings.ToUpper(strings.TrimSpace(r.Asset))
	   r.LoanID = strings.TrimSpace(r.LoanID)
	  r.IdempotencyKey = strings.TrimSpace(r.IdempotencyKey)
	  if r.UserID == "" {
		  return r, fmt.Errorf("%w: user", ErrInvalidAmount)
	   }
	   if r.Asset == "" {
		  return r, fmt.Errorf("%w: asset", ErrInvalidAmount)
	  }
	  if r.IdempotencyKey == "" {
		  return r, ErrMissingIdempotency
	   }
	  switch r.Kind {
	  case KindInterestPost, KindRepayOffset, KindZeroPeriod:
	  default:
		  return r, fmt.Errorf("%w: loan journal kind", ErrUnknownDirection)
	   }
	  return r, nil
  }

  func (r LoanPostReq) sameIntent(tr *Transfer) error {
	   if tr == nil {
		   return nil
	   }
	   if tr.UserID != r.UserID || tr.Asset != r.Asset {
		   return ErrIdempotencyConflict
	  }
	   want := r.postedAmount()
	   if !tr.Amount.Equal(want) {
		  return ErrIdempotencyConflict
	   }
	  return nil
  }

  func (r LoanPostReq) postedAmount() decimal.Decimal {
	   switch r.Kind {
	   case KindInterestPost, KindZeroPeriod:
		  if r.Amount.IsPositive() {
			  return r.Amount
		  }
		  return decimal.Zero
	   case KindRepayOffset:
		   sum := r.InterestPart.Add(r.PrincipalPart).Add(r.SurplusPart)
		   if sum.IsPositive() {
			  return sum
		  }
		   return decimal.Zero
	   default:
		   return decimal.Zero
	  }
   }

   func (r LoanPostReq) buildLines(tr *Transfer) []JournalEntry {
	  switch r.Kind {
	   case KindInterestPost:
		  return InterestPostLines(tr, r.Amount)
	   case KindRepayOffset:
		   lines := RepayOffsetLines(tr, r.InterestPart, r.PrincipalPart)
		  if r.SurplusPart.IsPositive() {
			   seq := 1
			   if n := len(lines); n > 0 {
				  seq = lines[n-1].Seq + 1
			  }
			  lines = append(lines, SurplusLines(tr, r.SurplusPart, seq)...)
		  }
		   return lines
	  default:
		   return nil
	   }
  }

// PostLoanJournal 借贷专用入账。与 Create 共用 Store / 幂等表，不进入出金风险闸。
   func (s *Service) PostLoanJournal(ctx context.Context, req LoanPostReq) (LoanPostResult, error) {
	  out := LoanPostResult{}
	   if s == nil || s.store == nil {
		  return out, ErrUnavailable
	   }
	  norm, err := req.normalize()
	  if err != nil {
		   return out, err
	  }
	  req = norm
	  ctx = WithShardUser(ctx, req.UserID)
	  if hit, ok, err := s.loanIdemHit(ctx, req); ok {
		   return hit, err
	   }
	   tr := &Transfer{
		  ID:             uuid.NewString(),
		  IdempotencyKey: req.IdempotencyKey,
		  UserID:         req.UserID,
		   Direction: FeeDebit,
		   Asset: req.Asset,
		  Amount: req.postedAmount(),
		  State:  StateCredited,
		  CreatedAt: time.Now().UTC(),
		   UpdatedAt: time.Now().UTC(),
		  MemoHint:  string(req.Kind) + "|" + req.LoanID,
	  }
	   if req.PeriodStart > 0 || req.PeriodEnd > 0 {
		  tr.TagHint = fmt.Sprintf("%d:%d", req.PeriodStart, req.PeriodEnd)
	  }
	   lines := req.buildLines(tr)
	  if len(lines) == 0 {
		  err := s.store.InTx(ctx, func(ctx context.Context) error {
			  return s.store.InsertTransfer(ctx, tr)
		   })
		   if err != nil {
			  if hit, ok, e2 := s.loanIdemHit(ctx, req); ok {
				   return hit, e2
			  }
			  return out, err
		  }
		  return LoanPostResult{Transfer: tr, Skipped: true}, nil
	  }
	  if err := ValidateJournal(lines); err != nil {
		  return out, err
	   }
	   err = s.store.InTx(ctx, func(ctx context.Context) error {
		   if err := s.store.InsertTransfer(ctx, tr); err != nil {
			   return err
		  }
		  if err := s.store.AppendJournal(ctx, lines); err != nil {
			   return err
		   }
		   for _, e := range lines {
			  acc, err := s.store.GetAccount(ctx, e.UserID, e.AccountType, e.Asset)
			  if err != nil {
				   return err
			  }
			  if err := ApplyJournalToAccount(acc, e); err != nil {
				   return err
			  }
			   if err := requireFreshBook(acc); err != nil {
				   return err
			   }
			   if err := s.store.UpsertAccount(ctx, acc); err != nil {
				  return err
			  }
		   }
		  return s.store.AppendAudit(ctx, AuditEvent{
			   EntityType: "loan_journal", EntityID: tr.ID, Action: string(req.Kind),
			  Actor: req.UserID, Detail: map[string]any{"loanId": req.LoanID, "period": tr.TagHint},
			  CreatedAt: time.Now().UTC(),
		  })
	   })
	   if err != nil {
		   if hit, ok, e2 := s.loanIdemHit(ctx, req); ok {
			   return hit, e2
		  }
		  return out, err
	  }
	  return LoanPostResult{Transfer: tr, Lines: lines}, nil
   }

   func (s *Service) loanIdemHit(ctx context.Context, req LoanPostReq) (LoanPostResult, bool, error) {
	  existing, err := s.store.GetByIdempotency(ctx, req.IdempotencyKey)
	  if err != nil || existing == nil {
		   return LoanPostResult{}, false, err
	   }
	  if err := req.sameIntent(existing); err != nil {
		  return LoanPostResult{},
			   true, err
	   }
	   lines, _ := s.store.ListJournal(ctx, existing.ID)
	   return LoanPostResult{Transfer: existing, Lines: lines, Existed: true, Skipped: existing.Amount.IsZero()}, true, nil
   }

// ChargedFee 取该转账单上已入账的手续费。只认 fee_credit / fee_debit，没有手续费分录则为 0。
  func (s *Service) ChargedFee(ctx context.Context, transferID, asset string) string {
	  lines, err := s.LoanJournalOf(ctx, transferID)
	  if err != nil || len(lines) == 0 {
		  return "0"
	  }
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  credit, debit := decimal.Zero, decimal.Zero
	  sawC, sawD := false, false
	  for _, ln := range lines {
		  if asset != "" && strings.ToUpper(ln.Asset) != asset {
			  continue
		  }
		  switch ln.Memo {
		  case "fee_credit":
			  credit = credit.Add(ln.Credit)
			  sawC = true
		  case "fee_debit":
			  debit = debit.Add(ln.Debit)
			  sawD = true
		  }
	  }
	  if sawC {
		  return credit.String()
	  }
	  if sawD {
		  return debit.String()
	  }
	  return "0"
  }

// LoanJournalOf 读一笔借贷分录（测科目方向）。
  func (s *Service) LoanJournalOf(ctx context.Context, transferID string) ([]JournalEntry, error) {
	  if s == nil || s.store == nil {
		   return nil, ErrUnavailable
	   }
	  return s.store.ListJournal(ctx, strings.TrimSpace(transferID))
   }

// LoanTransferByIdem 按幂等键取单。

   func (s *Service) LoanTransferByIdem(ctx context.Context, key string) (*Transfer, error) {
	  if s == nil || s.store == nil {
		  return nil, ErrUnavailable
	  }
	  return s.store.GetByIdempotency(ctx, strings.TrimSpace(key))
  }
