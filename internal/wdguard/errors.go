package wdguard

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("withdraw guard unavailable")
	ErrUser        = fmt.Errorf("withdraw guard user invalid")
	ErrChain       = fmt.Errorf("withdraw guard chain invalid")
	ErrDest = fmt.Errorf("withdraw guard dest invalid")
	ErrAsset = fmt.Errorf("withdraw guard asset invalid")
	ErrAmount = fmt.Errorf("withdraw guard amount invalid")
	ErrActor  = fmt.Errorf("withdraw guard actor invalid")
	ErrHalt   = fmt.Errorf("withdraw guard halted")
	ErrFence    = fmt.Errorf("withdraw guard write fence")
	ErrSanction = fmt.Errorf("withdraw guard sanctions hit")
	ErrPolicy   = fmt.Errorf("withdraw guard policy refuse")
	ErrScore = fmt.Errorf("withdraw guard risk refuse")
	ErrKYT   = fmt.Errorf("withdraw guard kyt refuse")
	ErrAllow = fmt.Errorf("withdraw guard destination not allowlisted")
	ErrAbuseKey = fmt.Errorf("withdraw guard abuse key must use wd: prefix")
	// ErrTripleDrift 广播前三联租约世代漂移，拒绝放行。
	ErrTripleDrift = fmt.Errorf("withdraw guard triple lease drift detected")
	// ErrTripleLeaseRequired 广播前未持有有效三联租约。
	ErrTripleLeaseRequired = fmt.Errorf("withdraw guard triple lease required before broadcast")
)
