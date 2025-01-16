package warroom

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("war room unavailable")
	ErrUser   = fmt.Errorf("war room user invalid")
	ErrActor  = fmt.Errorf("war room actor invalid")
	ErrReason = fmt.Errorf("war room reason invalid")
	ErrTicket = fmt.Errorf("war room ticket invalid")
	ErrState = fmt.Errorf("war room ticket state invalid")
	ErrFourEye = fmt.Errorf("war room four-eye required")
	ErrScope   = fmt.Errorf("war room scope invalid")
	ErrSwitch   = fmt.Errorf("war room freeze-all still on")
	ErrNotFound = fmt.Errorf("war room row not found")
	ErrSystem   = fmt.Errorf("war room cannot freeze system")
)
