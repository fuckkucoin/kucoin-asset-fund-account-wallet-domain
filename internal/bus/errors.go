package bus

import "errors"

var (
	ErrNoBroker = errors.New("kafka broker empty")
	ErrPublish  = errors.New("kafka publish failed")
	ErrUnavailable = errors.New("bus unavailable")
)
