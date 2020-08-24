package money

import "fmt"

// Line 复式一边。同一行不能又借又贷。
type Line struct {
	Asset  string
	Debit  Amount
	Credit Amount
	Memo string
}

// Balanced 按资产轧平。不平整批拒绝，避免半边入账。

  func Balanced(lines []Line) error {
	  if len(lines) == 0 {
		   return fmt.Errorf("empty journal")
	  }
	  type tot struct{ d, c Amount }
	  by := map[string]*tot{}
	  for i, e := range lines {
		   if e.Debit.IsNegative() || e.Credit.IsNegative() {
			  return fmt.Errorf("entry %d negative", i)
		   }
		   if !e.Debit.IsZero() && !e.Credit.IsZero() {
			  return fmt.Errorf("entry %d debits and credits", i)
		  }
		  if e.Debit.IsZero() && e.Credit.IsZero() {
			  return fmt.Errorf("entry %d empty", i)
		   }
		   t := by[e.Asset]
		   if t == nil {
			   t = &tot{d: Zero(), c: Zero()}
			  by[e.Asset] = t
		   }
		   var err error
		  t.d, err = t.d.Add(e.Debit)
		   if err != nil {
			   return err
		  }
		   t.c, err = t.c.Add(e.Credit)
		  if err != nil {
			  return err
		  }
	  }
	  for asset, t := range by {
		  if !t.d.Equal(t.c) {
			  return fmt.Errorf("unbalanced %s debit %s credit %s", asset, t.d, t.c)
		  }
	  }
	  return nil
  }

// FreezePair available → frozen，同一科目内转移。
   func FreezePair(asset string, amt Amount) []Line {
	   return []Line{
		   {Asset: asset, Debit: amt, Memo: "freeze_available"},
		   {Asset: asset, Credit: amt, Memo: "freeze_frozen"},
	  }
   }

// UnfreezePair frozen → available。
  func UnfreezePair(asset string, amt Amount) []Line {
	   return []Line{
		  {Asset: asset, Debit: amt, Memo: "unfreeze_frozen"},
		  {Asset: asset, Credit: amt, Memo: "restore_available"},
	  }
   }
