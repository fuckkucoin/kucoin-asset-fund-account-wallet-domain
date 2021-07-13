package hasync

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("ha sync unavailable")
	ErrActor       = fmt.Errorf("ha sync actor invalid")
	ErrHalt        = fmt.Errorf("ha sync halted")
	ErrNotReady = fmt.Errorf("ha sync not ready")
	ErrFence = fmt.Errorf("ha sync write fence")
	ErrSplit    = fmt.Errorf("ha sync split brain")
	ErrFailover = fmt.Errorf("ha sync failover refused")
)
