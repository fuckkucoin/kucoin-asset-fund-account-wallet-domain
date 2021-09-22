package oracle

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("oracle unavailable")
	ErrStale = fmt.Errorf("oracle tick stale")
	ErrMissing = fmt.Errorf("oracle tick missing")
	ErrPair  = fmt.Errorf("oracle pair invalid")
	ErrPrice = fmt.Errorf("oracle price invalid")
	ErrQuote = fmt.Errorf("oracle quote invalid")
	ErrCircuit = fmt.Errorf("oracle circuit open")
)
