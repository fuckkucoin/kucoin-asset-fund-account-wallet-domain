package repay

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/marginhook"
	"account-assets.kucoin.net/funding/internal/utabook"
)

type Service struct {
	Led *ledger.Service
	Book *utabook.Book
	Hook *marginhook.Service
}

  func New(led *ledger.Service, book *utabook.Book, hook *marginhook.Service) *Service {
	   return &Service{Led: led, Book: book, Hook: hook}
  }

type Req struct {
	UserID     string
	Currency   string
	MarginMode string
	Symbol     string
	Amount         string
	Policy         Policy
	IdempotencyKey string
	RepayID        string
}

type Result struct {
	Split utabook.OffsetSplit
	EntryID string
	IdemKey string
	Existed bool
	Lines   []ledger.JournalEntry
	Row utabook.LiabilityRow
}

  func (s *Service) Offset(ctx context.Context, req Req) (Result, error) {
	   out := Result{}
	   if s == nil || s.Led == nil || s.Book == nil {
		   return out, ErrUnavailable
	  }
	   req.UserID = strings.TrimSpace(req.UserID)
	  req.Currency = strings.ToUpper(strings.TrimSpace(req.Currency))
	   req.MarginMode = strings.ToUpper(strings.TrimSpace(req.MarginMode))
	  req.Symbol = strings.ToUpper(strings.TrimSpace(req.Symbol))
	   if req.MarginMode == "" {
		  req.MarginMode = "CROSS"
	  }
	  if req.Policy == "" {
		   req.Policy = PolicyReject
	   }
	  req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	  if req.IdempotencyKey == "" {
		   if strings.TrimSpace(req.RepayID) == "" {
			   return out, ErrIdem
		  }
		   req.IdempotencyKey = "loan-repay:" + strings.TrimSpace(req.RepayID)
	   }
	  out.IdemKey = req.IdempotencyKey
	  row, ok := s.Book.LiabilityLoan(req.UserID, req.Currency, req.MarginMode, req.Symbol)
	  if !ok {
		   return out, ErrLiability
	   }
	  if tr, err := s.Led.LoanTransferByIdem(ctx, req.IdempotencyKey); err == nil && tr != nil {
		   lines, _ := s.Led.LoanJournalOf(ctx, tr.ID)
		   out.EntryID = tr.ID
		  out.Existed = true
		   out.Lines = lines
		   out.Row = row
		  return out, nil
	   }
	  split, err := Split(row, req.Amount, req.Policy)
	  if err != nil {
		   return out, err
	  }
	   out.Split = split
	   loanID := utabook.LoanNoteID(req.UserID, req.Currency, req.MarginMode, req.Symbol)
	  posted, err := s.Led.PostLoanJournal(ctx, ledger.LoanPostReq{
		  UserID: req.UserID, Asset: req.Currency, LoanID: loanID,
		  Kind: ledger.KindRepayOffset,
		   InterestPart: split.Interest, PrincipalPart: split.Principal, SurplusPart: split.Surplus,
		  IdempotencyKey: req.IdempotencyKey,
	   })
	   if err != nil {
		  return out, err
	  }
	   if posted.Transfer != nil {
		  out.EntryID = posted.Transfer.ID
	   }
	   out.Existed = posted.Existed
	  out.Lines = posted.Lines
	   if !posted.Existed {
		   row,
			  err = s.Book.ApplyOffsetToLiability(req.UserID, req.Currency, req.MarginMode, req.Symbol, split)
		  if err != nil {
			   return out, err
		  }
		  if err := s.Book.NoteRepay(req.UserID, utabook.RepayRow{
			   ID: req.RepayID, Currency: req.Currency, MarginMode: req.MarginMode, Symbol: req.Symbol,
			   RepayType: "REPAY", Principal: split.Principal.String(), Interest: split.Interest.String(),
		   }); err != nil {
			  return out, err
		  }
		  if s.Hook != nil {
			  s.Hook.NotifyBalance(marginhook.BalanceChange{
				   UserID: req.UserID, LoanID: loanID, Asset: req.Currency,
				   Kind: marginhook.ChangeRepayOffset, Amount: req.Amount, EntryID: out.EntryID,
			   })
		  }
	   }
	   out.Row = row
	   return out, nil
   }
