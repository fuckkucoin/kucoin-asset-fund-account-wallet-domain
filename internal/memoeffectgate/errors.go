package memoeffectgate

import (
	"errors"
	"fmt"
)

var (
	ErrNilEngine        = errors.New("memoeffectgate: engine is nil")
	ErrStoreUnavailable = errors.New("memoeffectgate: store unavailable")
	ErrMutexUnavailable = errors.New("memoeffectgate: claimmutex adapter unavailable")
	ErrBookUnavailable   = errors.New("memoeffectgate: ticket book unavailable")
	ErrCreditUnavailable = errors.New("memoeffectgate: credit port unavailable")
	ErrTicketRequired    = errors.New("memoeffectgate: ticket_id required")
	ErrTicketNotFound    = errors.New("memoeffectgate: memo ticket not found")
	ErrTicketNotPending = errors.New("memoeffectgate: ticket not pending; fail-closed")
	ErrAssetRequired    = errors.New("memoeffectgate: asset required")
	ErrOutputOccupied = errors.New("memoeffectgate: output already taken by other leg")
	ErrAutoClaimOccupied = errors.New("memoeffectgate: output already auto-credited")
	ErrEffectRequired    = errors.New("memoeffectgate: memo_effect required; fail-closed")
	ErrEffectNotFound    = errors.New("memoeffectgate: memo effect not found")
	ErrEffectExpired     = errors.New("memoeffectgate: memo effect expired")
	ErrEffectNotBound    = errors.New("memoeffectgate: memo effect not bound")
	ErrIllegalTransition = errors.New("memoeffectgate: illegal effect state transition")
	ErrFenceUnavailable  = errors.New("memoeffectgate: ticket credit fence unavailable")
	ErrFenceHalfCut     = errors.New("memoeffectgate: half-cut discoverable; block second full credit")
	ErrSecondFullCredit = errors.New("memoeffectgate: second full credit refused")
	ErrObserveFailed = errors.New("memoeffectgate: ObserveCredit failed")
	ErrApplyFailed   = errors.New("memoeffectgate: ApplyInbound failed")
	ErrRejectAfterCredit   = errors.New("memoeffectgate: DecideMemo rejected after observed credit; proof required")
	ErrSilentKeepForbidden = errors.New("memoeffectgate: silent keep of observed credit forbidden")
	ErrProofNotFound       = errors.New("memoeffectgate: half credit proof not found")
	ErrProofNotOpen        = errors.New("memoeffectgate: half credit proof not open")
	ErrOpsReverseDenied = errors.New("memoeffectgate: ops reverse denied")
	ErrActorRequired    = errors.New("memoeffectgate: ops actor required")
	ErrStatusInvalid    = errors.New("memoeffectgate: decide status invalid")
	ErrIdemConflict     = errors.New("memoeffectgate: idempotency key bound to another effect")
	ErrOutputKeyInvalid = errors.New("memoeffectgate: output key invalid")
	ErrAcquireFailed    = errors.New("memoeffectgate: output acquire failed")
	ErrTTLInvalid    = errors.New("memoeffectgate: ttl must be positive")
	ErrChainRequired = errors.New("memoeffectgate: chain required")
	ErrTxHashRequired = errors.New("memoeffectgate: tx_hash required")
	ErrHalfIllegal    = errors.New("memoeffectgate: illegal half-proof transition")
	ErrFenceIllegal   = errors.New("memoeffectgate: illegal fence phase")
	ErrBindMemoFailed   = errors.New("memoeffectgate: BindMemo failed")
	ErrQuotaineRequired = errors.New("memoeffectgate: quarantine or reverse required after credit")
)

   func wrapProof(kind ProofKind, err error) error {
	  if err == nil {
		  return fmt.Errorf("memoeffectgate: proof %s", kind)
	   }
	  return fmt.Errorf("%w: %s", err, kind)
  }
