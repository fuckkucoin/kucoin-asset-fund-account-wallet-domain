package backpress

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("backpressure unavailable")
	ErrFull        = fmt.Errorf("backpressure queue full")
	ErrJob         = fmt.Errorf("backpressure job invalid")
	ErrHalt = fmt.Errorf("backpressure halted")
	ErrActor = fmt.Errorf("backpressure actor invalid")
	ErrClosed = fmt.Errorf("backpressure closed")
)
