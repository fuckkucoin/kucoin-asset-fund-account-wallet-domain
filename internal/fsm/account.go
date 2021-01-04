package fsm

const (
	AcClosed = "closed"
	AcActive = "active"
	AcFrozen = "frozen"
	AcEvOpen     = "open"
	AcEvFreeze   = "freeze"
	AcEvUnfreeze = "unfreeze"
	AcEvClose    = "close"
)

  func AccountBook() *Machine[string, string] {
	   return New([]Edge[string, string]{
		  {AcClosed, AcEvOpen, AcActive},
		   {AcActive, AcEvFreeze, AcFrozen},
		  {AcActive, AcEvClose, AcClosed},
		   {AcFrozen,
			  AcEvUnfreeze, AcActive},
	  }, AcClosed)
   }

   func CanDebit(s string) bool { return s == AcActive }

   func CanCredit(s string) bool { return s == AcActive }

  func CanClose(s string) bool { return s == AcActive }
