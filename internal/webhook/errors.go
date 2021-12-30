package webhook

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("webhook dispatcher unavailable")
	ErrKind        = fmt.Errorf("webhook kind invalid")
	ErrEndpoint    = fmt.Errorf("webhook endpoint invalid")
	ErrSig    = fmt.Errorf("webhook signature invalid")
	ErrSink   = fmt.Errorf("webhook sink failed")
	ErrReplay = fmt.Errorf("webhook nonce reused")
	ErrDead   = fmt.Errorf("webhook dead letter missing")
)
