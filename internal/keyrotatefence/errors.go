package keyrotatefence

import "errors"

var (
	ErrUnavailable = errors.New("keyrotatefence unavailable")
	ErrTicketRequired = errors.New("keyrotatefence ticket_id required")
	ErrKeyIDRequired  = errors.New("keyrotatefence key_id required")
	ErrActorRequired = errors.New("keyrotatefence actor required")
	ErrWindowNotFound = errors.New("keyrotatefence window not found")
	ErrInvalidPhase   = errors.New("keyrotatefence invalid phase transition")
	ErrWindowBlocked  = errors.New("keyrotatefence window blocked")
	ErrWindowSealed   = errors.New("keyrotatefence window already sealed")
	ErrOldKeyRetired  = errors.New("keyrotatefence old key retired; use refused")
	ErrNewKeyMissing  = errors.New("keyrotatefence new key missing after cut; fail-closed")
	ErrNewKeyMismatch = errors.New("keyrotatefence new key fingerprint mismatch")
	ErrIdemConflict   = errors.New("keyrotatefence idempotency payload conflict")
	ErrLocalHMAC      = errors.New("keyrotatefence production refuses local-hmac")
	ErrDemoKey        = errors.New("keyrotatefence production refuses in-repo demo private key")
	ErrProdFallback    = errors.New("keyrotatefence production fallback to demo material refused")
	ErrSnapshotCorrupt = errors.New("keyrotatefence snapshot corrupt")
)
