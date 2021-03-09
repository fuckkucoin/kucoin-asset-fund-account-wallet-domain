package casefencegate

import "fmt"

var (
	ErrStaleFence     = fmt.Errorf("stale case fence; concurrent decide race lost")
	ErrBareForbidden  = fmt.Errorf("bare Decide forbidden on fund path; must use DecideFenced")
	ErrTicketNotFound = fmt.Errorf("release ticket not found")
	ErrTicketExpired  = fmt.Errorf("release ticket expired; withdraw stays fail-closed")
	ErrTicketConsumed = fmt.Errorf("release ticket already consumed")
	ErrTicketStale    = fmt.Errorf("release ticket stale; fence epoch advanced")
	ErrTicketIllegal = fmt.Errorf("illegal ticket state transition")
	ErrPolicyDrift   = fmt.Errorf("policy version drift; ticket pinned to older version")
	ErrCaseNotOpen = fmt.Errorf("case not open; fence decide rejected")
	ErrCaseNotFound      = fmt.Errorf("case not found")
	ErrCaseAlreadyClosed = fmt.Errorf("case already closed; no further transitions")
	ErrMakerCheck        = fmt.Errorf("actor must be a distinct ops reviewer")
	ErrExpireMutex       = fmt.Errorf("expire-approve mutex; case already expired or decided")
	ErrProofSealed = fmt.Errorf("dual decide proof already sealed")
	ErrProofNotFound = fmt.Errorf("dual decide proof not found")
	ErrEngineNil     = fmt.Errorf("case fence gate engine unavailable")
	ErrStoreNil      = fmt.Errorf("case fence gate store unavailable")
	ErrInvalidDecision  = fmt.Errorf("decision must be approve or deny")
	ErrMovementMismatch = fmt.Errorf("movement id mismatch on submit check")
	ErrActorSelf = fmt.Errorf("actor cannot decide own case")
)
