package quotemultigate

import "account-assets.kucoin.net/funding/internal/quote"

var (
	ErrQuoteEmpty = quote.ErrQuoteEmpty
	ErrQuoteStale = quote.ErrQuoteStale
	ErrQuoteIllegal = quote.ErrQuoteIllegal
	ErrQuoteDiverge = quote.ErrQuoteDiverge
	ErrQuoteTimeout = quote.ErrQuoteTimeout
	ErrClockRequired = quote.ErrClockRequired
	ErrPairRequired   = quote.ErrPairRequired
	ErrHouseOnConfirm = quote.ErrHouseOnConfirm
	ErrSourceMasked   = quote.ErrSourceMasked
)
