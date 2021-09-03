package notify

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("notify consumer unavailable")
	ErrSink        = fmt.Errorf("notify sink rejected")
	ErrKind = fmt.Errorf("unknown notify kind")
	ErrDead     = fmt.Errorf("notify event dead-lettered")
	ErrUnsigned = fmt.Errorf("notify channel unsigned")
	ErrDest     = fmt.Errorf("notify channel url not allowed")
)
