package snap

import (
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Change struct {
	Book string `json:"book"`
	Asset     string `json:"asset"`
	AvailFrom string `json:"available_from"`
	AvailTo   string `json:"available_to"`
	FrozenFrom string `json:"frozen_from"`
	FrozenTo   string `json:"frozen_to"`
	DeltaAvail string `json:"delta_available"`
	DeltaFrozen string `json:"delta_frozen"`
}

type Diff struct {
	UserID string `json:"user_id"`
	FromID  string     `json:"from_id"`
	ToID    string     `json:"to_id"`
	Added   []Position `json:"added"`
	Removed []Position `json:"removed"`
	Changed []Change `json:"changed"`
	Same    int      `json:"same"`
}

// Compare 两份快照按 book|asset 一致。金额用 ParseBook，禁止科学计数混进来。

  func Compare(from, to Snapshot) (Diff, error) {
	   if from.UserID != "" && to.UserID != "" && from.UserID != to.UserID {
		  return Diff{}, fmt.Errorf("%w: user", ErrMismatch)
	  }
	  if err := from.VerifyChecksum(); err != nil {
		   return Diff{}, err
	  }
	   if err := to.VerifyChecksum(); err != nil {
		  return Diff{}, err
	  }
	  left := indexPositions(from.Positions)
	   right := indexPositions(to.Positions)
	   out := Diff{UserID: firstNonEmpty(from.UserID, to.UserID), FromID: from.ID, ToID: to.ID}
	   seen := map[string]struct{}{}
	  for k, a := range left {
		   seen[k] = struct{}{}
		  b, ok := right[k]
		  if !ok {
			   out.Removed = append(out.Removed, a)
			  continue
		   }
		  if a.Available == b.Available && a.Frozen == b.Frozen {
			  out.Same++
			  continue
		   }
		  ch, err := changeOf(a, b)
		   if err != nil {
			  return Diff{}, err
		   }
		  out.Changed = append(out.Changed, ch)
	   }
	   for k, b := range right {
		  if _, ok := seen[k]; ok {
			   continue
		   }
		   out.Added = append(out.Added, b)
	   }
	   return out, nil
   }

  func indexPositions(rows []Position) map[string]Position {
	   out := make(map[string]Position, len(rows))
	  for _, p := range rows {
		   out[positionKey(p)] = p
	  }
	   return out
  }

  func changeOf(a, b Position) (Change, error) {
	   aa, err := a.AvailAmt()
	   if err != nil {
		   return Change{}, err
	   }
	   ba, err := b.AvailAmt()
	  if err != nil {
		  return Change{}, err
	   }
	  af, err := a.FrozenAmt()
	  if err != nil {
		   return Change{}, err
	  }
	  bf, err := b.FrozenAmt()
	  if err != nil {
		  return Change{}, err
	  }
	   dAvail := signedDelta(aa, ba)
	   dFr := signedDelta(af, bf)
	   return Change{
		   Book: a.Book, Asset: a.Asset,
		  AvailFrom: a.Available, AvailTo: b.Available,
		   FrozenFrom: a.Frozen, FrozenTo: b.Frozen,
		   DeltaAvail: dAvail, DeltaFrozen: dFr,
	  }, nil
   }

  func signedDelta(from, to money.Amount) string {
	   cmp := to.Cmp(from)
	  if cmp == 0 {
		  return "0"
	   }
	  if cmp > 0 {
		  d, err := to.Sub(from)
		  if err != nil {
			   return "?"
		  }
		   return "+" + d.String()
	  }
	   d, err := from.Sub(to)
	   if err != nil {
		  return "?"
	   }
	   return "-" + d.String()
  }

   func firstNonEmpty(a, b string) string {
	  if strings.TrimSpace(a) != "" {
		  return a
	   }
	  return b
   }

   func (d Diff) Empty() bool {
	   return len(d.Added) == 0 && len(d.Removed) == 0 && len(d.Changed) == 0
  }
