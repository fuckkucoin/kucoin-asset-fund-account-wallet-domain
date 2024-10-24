package p2pnet

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("p2p network unavailable")
	ErrUser        = fmt.Errorf("p2p user invalid")
	ErrDest   = fmt.Errorf("p2p destination invalid")
	ErrSelf   = fmt.Errorf("p2p self transfer refused")
	ErrAsset  = fmt.Errorf("p2p asset invalid")
	ErrAmount = fmt.Errorf("p2p amount invalid")
	ErrIdem   = fmt.Errorf("p2p idempotency conflict")
	ErrRisk   = fmt.Errorf("p2p risk screen refused")
	ErrVelocity = fmt.Errorf("p2p velocity exceeded")
	ErrWindow   = fmt.Errorf("p2p chargeback window closed")
	ErrNotFound = fmt.Errorf("p2p transfer not found")
	ErrState  = fmt.Errorf("p2p transfer illegal state")
	ErrLedger = fmt.Errorf("p2p ledger refused")
	ErrActor  = fmt.Errorf("p2p actor invalid")
	ErrReason = fmt.Errorf("p2p reason invalid")
	ErrBlocked = fmt.Errorf("p2p destination blocked")
)
