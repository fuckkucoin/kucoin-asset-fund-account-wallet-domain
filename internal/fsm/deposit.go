package fsm

const (
	DpObserved = "observed"
	DpAccepted = "accepted"
	DpConfirmed = "confirmed"
	DpCredited = "credited"
	DpHeld     = "held"
	DpUnwound  = "unwound"
	DpFailed = "failed"
	DpEvAccept  = "accept"
	DpEvConfirm = "confirm"
	DpEvCredit  = "credit"
	DpEvHold    = "hold"
	DpEvUnwind = "unwind"
	DpEvFail   = "fail"
	DpEvResume = "resume"
)

  func Deposit() *Machine[string, string] {
	   return New([]Edge[string, string]{
		   {DpObserved, DpEvAccept, DpAccepted},
		  {DpObserved, DpEvFail, DpFailed},
		  {DpAccepted, DpEvConfirm, DpConfirmed},
		  {DpAccepted, DpEvHold, DpHeld},
		   {DpAccepted, DpEvFail, DpFailed},
		  {DpConfirmed, DpEvCredit, DpCredited},
		  {DpConfirmed, DpEvHold, DpHeld},
		   {DpConfirmed, DpEvFail, DpFailed},
		  {DpCredited, DpEvUnwind, DpUnwound},
		   {DpHeld, DpEvResume, DpAccepted},
		   {DpHeld, DpEvFail, DpFailed},
		  {DpHeld, DpEvUnwind, DpUnwound},
	   }, DpCredited, DpUnwound, DpFailed)
  }

   func DepositCanCredit(s string) bool {
	   return s == DpConfirmed
   }

  func DepositCanUnwind(s string) bool {
	  return s == DpCredited || s == DpHeld
  }
