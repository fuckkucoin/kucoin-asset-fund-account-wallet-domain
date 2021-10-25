package riskgate

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("risk gate unavailable")
	ErrUser        = fmt.Errorf("risk gate user invalid")
	ErrAsset       = fmt.Errorf("risk gate asset invalid")
	ErrAmount = fmt.Errorf("risk gate amount invalid")
	ErrPack   = fmt.Errorf("risk gate pack invalid")
	ErrDeny   = fmt.Errorf("risk gate denied")
	ErrHalt   = fmt.Errorf("risk gate halted")
	ErrActor = fmt.Errorf("risk gate actor invalid")
	ErrEval = fmt.Errorf("risk gate eval failed")
)
