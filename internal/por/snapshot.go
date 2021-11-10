package por

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/google/uuid"
)

type Snapshot struct {
	ID string `json:"id"`
	TakenAt time.Time `json:"taken_at"`
	Root string `json:"root"`
	Count int `json:"count"`
	Totals []Total `json:"totals"`
	Disclaimer string `json:"disclaimer"`
	Deprecated bool   `json:"deprecated"`
	Solvency   bool   `json:"solvency"`
	Class string `json:"class"`
	Tree  Tree   `json:"-"`
}

type Total struct {
	Asset string `json:"asset"`
	Amount string `json:"amount"`
}

// Disclaimer 工程默克尔免责声明。该快照已废弃，不得当作偿付证明。
const Disclaimer = "deprecated: engineering merkle over book rows; not a solvency attestation"

// PriorDisclaimer 改标前的原文。两种文案都不是偿付根。
const PriorDisclaimer = "engineering merkle over book rows; not a public reserve attestation"

const EngineeringClass = "engineering_merkle"

   func (s Snapshot) IsSolvency() bool { return false }

  func BuildFromStore(ctx context.Context, st ledger.Store, users []string, now time.Time) (Snapshot, error) {
	  if st == nil {
		   return Snapshot{}, ErrUnavailable
	  }
	   leaves := make([]Leaf, 0)
	   for _, uid := range users {
		   u, err := validate.UserID(uid)
		  if err != nil {
			   return Snapshot{}, ErrUser
		  }
		   if u == "system" {
			  continue
		  }
		   accs, err := st.ListUserBooks(ctx, u)
		   if err != nil {
			  return Snapshot{}, err
		  }
		   for _, a := range accs {
			  if a.UserID == "system" || !a.AccountType.UserFacing() {
				  continue
			   }
			   lf, err := ParseLeaf(a.UserID, string(a.AccountType), a.Asset, a.Available.String(), a.Frozen.String())
			   if err != nil {
				  return Snapshot{}, err
			  }
			  leaves = append(leaves, lf)
		  }
	  }
	   return seal(leaves, now)
  }

   func seal(leaves []Leaf, now time.Time) (Snapshot, error) {
	   tr, err := Build(leaves)
	   if err != nil {
		  return Snapshot{}, err
	   }
	   by := map[string]money.Amount{}
	   for _, l := range tr.Leaves {
		  av, err := money.ParseBook(l.Available)
		   if err != nil {
			   return Snapshot{}, err
		   }
		   fr, err := money.ParseBook(l.Frozen)
		   if err != nil {
			  return Snapshot{}, err
		  }
		   tot, err := av.Add(fr)
		  if err != nil {
			  return Snapshot{}, err
		  }
		   cur := by[l.Asset]
		  next, err := cur.Add(tot)
		  if err != nil {
			  return Snapshot{}, err
		   }
		  by[l.Asset] = next
	   }
	  totals := make([]Total, 0, len(by))
	  for asset, amt := range by {
		  totals = append(totals, Total{Asset: asset, Amount: amt.String()})
	  }
	  if now.IsZero() {
		  now = time.Now().UTC()
	  }
	   return Snapshot{
		   ID: uuid.NewString(), TakenAt: now.UTC(), Root: tr.Root,
		  Count: len(tr.Leaves), Totals: totals, Disclaimer: Disclaimer,
		   Deprecated: true,
		   Solvency: false, Class: EngineeringClass, Tree: tr,
	   }, nil
   }

   func (s Snapshot) Prove(userID, book, asset string) (Proof, error) {
	   b, err := validate.Book(book)
	   if err != nil {
		  b = strings.ToLower(strings.TrimSpace(book))
	   }
	   leaf := Leaf{
		   UserID: strings.TrimSpace(userID), Book: b,
		   Asset: strings.ToUpper(strings.TrimSpace(asset)),
	   }
	   for _, l := range s.Tree.Leaves {
		  if l.UserID == leaf.UserID && l.Book == leaf.Book && l.Asset == leaf.Asset {
			  return s.Tree.Prove(l)
		  }
	  }
	  return Proof{}, ErrNotFound
   }
