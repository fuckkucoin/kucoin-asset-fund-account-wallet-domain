package fsm

const (
	WdRequested = "requested"
	WdFrozen    = "frozen"
	WdAllowlisted = "allowlisted"
	WdDecrypted   = "decrypted"
	WdKYT         = "kyt"
	WdReview = "review"
	WdSigned    = "signed"
	WdSubmitted = "submitted"
	WdConfirmed = "confirmed"
	WdFailed    = "failed"
	WdEvFreeze = "freeze"
	WdEvAllow   = "allowlist"
	WdEvDecrypt = "decrypt"
	WdEvKYT    = "kyt"
	WdEvReview = "review"
	WdEvSign   = "sign"
	WdEvSubmit = "submit"
	WdEvConfirm = "confirm"
	WdEvFail    = "fail"
)

   func Withdraw() *Machine[string, string] {
	   return New([]Edge[string, string]{
		   {WdRequested, WdEvFreeze, WdFrozen},
		   {WdRequested, WdEvFail, WdFailed},
		   {WdFrozen, WdEvAllow, WdAllowlisted},
		  {WdFrozen, WdEvFail, WdFailed},
		   {WdAllowlisted, WdEvDecrypt, WdDecrypted},
		   {WdAllowlisted, WdEvFail, WdFailed},
		   {WdDecrypted, WdEvKYT, WdKYT},
		   {WdDecrypted, WdEvFail, WdFailed},
		  {WdKYT, WdEvSign, WdSigned},
		  {WdKYT, WdEvReview, WdReview},
		   {WdKYT, WdEvFail, WdFailed},
		   {WdReview, WdEvSign, WdSigned},
		  {WdReview, WdEvFail, WdFailed},
		   {WdSigned, WdEvSubmit, WdSubmitted},
		  {WdSigned, WdEvFail, WdFailed},
		   {WdSubmitted,
			   WdEvConfirm, WdConfirmed},
		  {WdSubmitted, WdEvFail, WdFailed},
	  }, WdConfirmed, WdFailed)
   }

// ReviewIsNotBroadcast review 不能当已提交，否则会跳过签章。

  func ReviewIsNotBroadcast(s string) bool {
	  return s == WdReview
  }

  func WithdrawCanSubmit(s string) bool {
	   return s == WdSigned
   }
