package keyrotgate

import "fmt"

var (
	// 通用
	ErrUnavailable       = fmt.Errorf("keyrotgate unavailable")
	ErrInvalidTransition = fmt.Errorf("keyrotgate invalid state transition")
	// RotateLease
	ErrLeaseNotFound = fmt.Errorf("keyrotgate rotate lease not found")
	ErrLeaseExpired  = fmt.Errorf("keyrotgate rotate lease expired")
	ErrLeaseBlocked  = fmt.Errorf("keyrotgate rotate lease blocked")
	ErrLeaseTerminal = fmt.Errorf("keyrotgate rotate lease already terminal")
	ErrLeaseIDRequired = fmt.Errorf("keyrotgate rotate_lease_id required")
	ErrTicketIDRequired = fmt.Errorf("keyrotgate ticket_id required")
	ErrSlotIDRequired      = fmt.Errorf("keyrotgate slot_id required")
	ErrFingerprintRequired = fmt.Errorf("keyrotgate fingerprint required")
	ErrActorRequired       = fmt.Errorf("keyrotgate actor required")
	ErrFourEye = fmt.Errorf("keyrotgate four-eye required: approved_by must differ from opened_by")
	ErrFingerprintMismatch = fmt.Errorf("keyrotgate fingerprint mismatch: slot fingerprint changed since lease issued")
	ErrRotateGenMismatch   = fmt.Errorf("keyrotgate rotate_gen mismatch: concurrent rotation detected")
	// 在途签名互斥
	ErrInflightSignExists = fmt.Errorf("keyrotgate execute refused: inflight sign binds exist for source slot")
	ErrSignBindNotFound   = fmt.Errorf("keyrotgate sign bind not found")
	ErrSignBindTerminal    = fmt.Errorf("keyrotgate sign bind already terminal")
	ErrOperationIDRequired = fmt.Errorf("keyrotgate operation_id required")
	ErrInvalidKind         = fmt.Errorf("keyrotgate operation_kind must be sign or unlock")
	ErrSlotRetiredReject = fmt.Errorf("keyrotgate sign rejected: source slot already retired")
	// 漂移案件
	ErrDriftCaseNotFound = fmt.Errorf("keyrotgate drift case not found")
	ErrDriftCaseTerminal  = fmt.Errorf("keyrotgate drift case already closed")
	ErrProofChecksumEmpty = fmt.Errorf("keyrotgate proof_checksum required")
	ErrDualConfirm        = fmt.Errorf("keyrotgate dual confirmation required: confirmed_by != co_confirmed_by")
	ErrOpenDriftExists = fmt.Errorf("keyrotgate open drift case exists: resolve before proceeding")
	// FingerprintProof
	ErrProofNotFound = fmt.Errorf("keyrotgate fingerprint proof not found")
	ErrProofRequired = fmt.Errorf("keyrotgate fingerprint proof required for this slot")
)
