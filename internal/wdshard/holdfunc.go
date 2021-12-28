package wdshard

// BookHoldFunc 将 Book 适配为 HoldAPI。

type BookHoldFunc struct {
	Book *Book
}

   func (f BookHoldFunc) Freeze(planID, legID, userID, asset string, amount int64) (Hold, error) {
	  if f.Book == nil {
		  return Hold{}, ErrUnavailable
	  }
	   return f.Book.FreezeHold(planID, legID, userID, asset, amount)
  }

  func (f BookHoldFunc) Release(holdID string, amount int64) (Hold, error) {
	  if f.Book == nil {
		  return Hold{}, ErrUnavailable
	   }
	   return f.Book.ReleaseHold(holdID, amount)
   }

   func (f BookHoldFunc) Get(holdID string) (Hold, error) {
	   if f.Book == nil {
		  return Hold{}, ErrUnavailable
	  }
	  return f.Book.GetHold(holdID)
   }

// BookAlign AlignHook。
type BookAlign struct {
	Book *Book
}

  func (a BookAlign) Align(hint AlignHint) (Leg, error) {
	   if a.Book == nil {
		  return Leg{}, ErrUnavailable
	   }
	  return a.Book.AlignFill(hint)
   }
