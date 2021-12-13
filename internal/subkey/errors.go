package subkey

import "errors"

var (
	ErrPass = errors.New("subName and passphrase required")
	ErrGone = errors.New("api key not found")
	ErrAuth    = errors.New("passphrase mismatch")
	ErrEnc     = errors.New("kms envelope required")
	ErrKey     = errors.New("apiKey required")
	ErrSubName = errors.New("subName required")
	ErrQuota   = errors.New("sub api key quota exceeded")
	ErrRevoked = errors.New("api key revoked")
	ErrRotating = errors.New("api key rotation in progress")
	ErrBadPerm  = errors.New("permission invalid")
	ErrBadIP    = errors.New("ip whitelist invalid")
	ErrIPDenied = errors.New("client ip not in whitelist")
	ErrBadState = errors.New("api key state transition invalid")
	ErrAudit = errors.New("audit persist failed")
)
