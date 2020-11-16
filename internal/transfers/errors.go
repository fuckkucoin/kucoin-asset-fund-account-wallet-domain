package transfers

import "fmt"

var (
	ErrStore         = fmt.Errorf("transfers store unavailable")
	ErrNotFound      = fmt.Errorf("transfers rec not found")
	ErrUnreclaimable = fmt.Errorf("transfers rec unreclaimable")
)
