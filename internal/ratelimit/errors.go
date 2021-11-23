package ratelimit

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("rate limiter unavailable")
	ErrLimited = fmt.Errorf("rate limited")
	ErrActor   = fmt.Errorf("rate limit actor required")
	ErrAction  = fmt.Errorf("rate limit action required")
)
