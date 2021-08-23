package leddiff

import (
	"strings"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/validate"
)

   func normalize(p Position) (Position, error) {
	  uid, err := validate.UserID(p.UserID)
	  if err != nil {
		  return Position{}, ErrUser
	  }
	  if uid == "system" {
		  return Position{}, ErrUser
	  }
	   book, err := validate.Book(p.Book)
	   if err != nil {
		   return Position{}, ErrBook
	   }
	  as, err := validate.Asset(p.Asset)
	  if err != nil {
		  return Position{}, ErrAsset
	  }
	  av, err := parseSnap(p.Available)
	  if err != nil {
		   return Position{}, err
	  }
	  fr, err := parseSnap(p.Frozen)
	   if err != nil {
		  return Position{}, err
	  }
	   return Position{UserID: uid, Book: book, Asset: as, Available: av.String(), Frozen: fr.String()}, nil
  }

  func posKey(p Position) string {
	   return p.UserID + "|" + p.Book + "|" + p.Asset
   }

  func orZ(s string) string {
	   if strings.TrimSpace(s) == "" {
		  return "0"
	  }
	   return s
  }

// parseSnap 快照金额允许 0，仍拒科学计数。ParseBook 会放过 1e2，这里补闸。
   func parseSnap(s string) (money.Amount, error) {
	   s = orZ(s)
	   for _, r := range s {
		  if r == 'e' || r == 'E' {
			   return money.Zero(), ErrAmount
		  }
	  }
	  got, err := money.ParseBook(s)
	  if err != nil {
		  return money.Zero(), ErrAmount
	   }
	  return got, nil
  }

// Diff 两边快照按 user|book|asset 一致。正变动合计必须等于负变动绝对值，否则 ErrImbalance。
  func (s *Service) Diff(leftID, rightID string) (Report, error) {
	   if s == nil {
		  return Report{}, ErrUnavailable
	   }
	   left, err := s.Get(leftID)
	  if err != nil {
		   return Report{}, err
	   }
	  right, err := s.Get(rightID)
	   if err != nil {
		   return Report{}, err
	   }
	   rep, err := Compare(left, right)
	   rep.At = s.now()
	   if rep.ID != "" {
		   s.mu.Lock()
		   cp := rep
		  s.reps[cp.ID] = &cp
		  s.mu.Unlock()
	   }
	   if err != nil {
		  return rep.Copy(), err
	   }
	   return rep.Copy(), nil
   }

   func Compare(left, right Snapshot) (Report, error) {
	  by := map[string]Position{}
	   for _, p := range left.Positions {
		   n, err := normalize(p)
		   if err != nil {
			   return Report{}, err
		  }
		  by[posKey(n)] = n
	  }
	   seen := map[string]bool{}
	   lines := make([]Line, 0)
	   pos := money.Zero()
	   neg := money.Zero()
	   addLine := func(before, after Position) error {
		   bav, err := parseSnap(before.Available)
		  if err != nil {
			  return err
		   }
		  aav, err := parseSnap(after.Available)
		  if err != nil {
			  return err
		  }
		   bfr,
			   err := parseSnap(before.Frozen)
		   if err != nil {
			  return err
		   }
		   afr,
			   err := parseSnap(after.Frozen)
		  if err != nil {
			   return err
		   }
		  dAvail := aav.Dec().Sub(bav.Dec())
		  dFr := afr.Dec().Sub(bfr.Dec())
		  net := dAvail.Add(dFr)
		  if net.IsPositive() {
			   next, err := pos.Add(money.FromDecimal(net))
			  if err != nil {
				   return err
			  }
			   pos = next
		   } else if net.IsNegative() {
			  next,
				  err := neg.Add(money.FromDecimal(net.Abs()))
			  if err != nil {
				   return err
			   }
			  neg = next
		   }
		   if net.IsZero() && dAvail.IsZero() && dFr.IsZero() {
			   return nil
		  }
		   lines = append(lines, Line{
			  UserID: after.UserID, Book: after.Book, Asset: after.Asset,
			   BeforeAvail: bav.String(), AfterAvail: aav.String(),
			   BeforeFrozen: bfr.String(),
			   AfterFrozen:  afr.String(),
			  DeltaAvail:   dAvail.String(), DeltaFrozen: dFr.String(),
		   })
		   return nil
	  }
	   for _, p := range right.Positions {
		   n, err := normalize(p)
		  if err != nil {
			   return Report{}, err
		   }
		   seen[posKey(n)] = true
		  before := by[posKey(n)]
		  if before.UserID == "" {
			   before = Position{UserID: n.UserID, Book: n.Book, Asset: n.Asset, Available: "0", Frozen: "0"}
		   }
		   if err := addLine(before, n); err != nil {
			   return Report{}, err
		   }
	  }
	   for k, before := range by {
		  if seen[k] {
			  continue
		   }
		  after := Position{UserID: before.UserID, Book: before.Book, Asset: before.Asset, Available: "0", Frozen: "0"}
		   if err := addLine(before, after); err != nil {
			  return Report{}, err
		  }
	   }
	  if err := money.Conserve(pos, neg); err != nil {
		  return Report{
			   ID: uuid.NewString(), LeftID: left.ID, RightID: right.ID, Lines: lines,
			   Positive: pos.String(), Negative: neg.String(), Balanced: false, Disclaimer: disclaimer,
		  }, ErrImbalance
	   }
	   return Report{
		  ID: uuid.NewString(), LeftID: left.ID, RightID: right.ID, Lines: lines,
		  Positive: pos.String(), Negative: neg.String(), Balanced: true, Disclaimer: disclaimer,
	  }, nil
  }
