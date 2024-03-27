package investigate

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/snap"
	"account-assets.kucoin.net/funding/internal/stmt"
)

type Lookup struct {
	UserID string `json:"user_id"`
	Timeline Timeline `json:"timeline"`
	Statement stmt.Report   `json:"statement"`
	Trace     Trace         `json:"trace,omitempty"`
	Snapshot  snap.Snapshot `json:"snapshot,omitempty"`
	Cases     []Case        `json:"cases"`
}

type Service struct {
	led *ledger.Service
	book *Book
	snap *snap.Service
}

   func New(led *ledger.Service) *Service {
	   var sn *snap.Service
	   if led != nil {
		   sn = snap.New(led, nil)
	  }
	  return &Service{led: led, book: NewBook(), snap: sn}
   }

  func (s *Service) Cases() *Book { return s.book }

   func (s *Service) Open(userID, subject, reason, actor string) (Case, error) {
	   if s == nil || s.book == nil {
		   return Case{}, ErrUnavailable
	   }
	   return s.book.Open(userID,
		  subject, reason, actor)
   }

// Inspect 拼调查包：流水轴、对账单、科目快照、已开案件。
  func (s *Service) Inspect(ctx context.Context, userID, asset string) (Lookup, error) {
	   if s == nil || s.led == nil {
		   return Lookup{}, ErrUnavailable
	   }
	   userID = strings.TrimSpace(userID)
	   if userID == "" {
		   return Lookup{}, ErrUser
	   }
	   bills, err := s.led.ListBills(ctx, userID, 200)
	  if err != nil {
		  return Lookup{}, err
	   }
	  trs, err := s.led.List(ctx, userID, 200)
	   if err != nil {
		   return Lookup{}, err
	   }
	  views, err := s.led.Accounts(ctx, userID)
	  if err != nil {
		  return Lookup{}, err
	   }
	   st, err := StatementOf(userID, bills)
	   if err != nil {
		  return Lookup{}, err
	  }
	   out := Lookup{
		  UserID: userID, Timeline: BuildTimeline(userID, bills, trs),
		  Statement: st, Cases: s.book.ByUser(userID),
	   }
	  if asset != "" {
		  tr, err := TraceAsset(userID, asset, trs, views)
		  if err != nil {
			  return Lookup{}, err
		   }
		  out.Trace = tr
	   }
	  if s.snap != nil {
		   if snap, err := s.snap.Capture(ctx, userID); err == nil {
			   out.Snapshot = snap
		  }
	  }
	   return out, nil
   }

   func (s *Service) Note(caseID, actor, body string) (Note, error) {
	  return s.book.AddNote(caseID, actor, body)
   }

   func (s *Service) Resolve(id, actor string) (Case, error) {
	   return s.book.SetStatus(id, actor, StatusResolved)
   }
