package fxmark

import (
	"errors"
	"account-assets.kucoin.net/funding/internal/quote"
)

var (
	ErrQuoteEmpty = quote.ErrQuoteEmpty
	ErrQuoteStale   = quote.ErrQuoteStale
	ErrQuoteIllegal = quote.ErrQuoteIllegal
	ErrQuoteDiverge = quote.ErrQuoteDiverge
	ErrQuoteTimeout = quote.ErrQuoteTimeout
	ErrPair         = errors.New("fxmark pair required")
	ErrAmount = errors.New("fxmark amount invalid")
	ErrLedger = errors.New("fxmark ledger required")
)
