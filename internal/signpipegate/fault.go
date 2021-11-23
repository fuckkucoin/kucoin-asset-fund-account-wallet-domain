package signpipegate

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// FaultInjector 落 FaultCase 并按种类驱动观察。

type FaultInjector struct {
	store Store
	clock func() time.Time
	obs   JobObserver
}

   func newFaultInjector(store Store, obs JobObserver, clock func() time.Time) *FaultInjector {
	  if clock == nil {
		   clock = func() time.Time { return time.Now().UTC() }
	}
	return &FaultInjector{store: store, obs: obs, clock: clock}
}

   func (i *FaultInjector) Inject(ctx context.Context, lease *InflightLease, kind FaultKind, operator string) (*FaultCase, error) {
	  if i == nil || i.store == nil {
		   return nil, ErrUnavailable
	   }
	   if lease == nil {
		   return nil, ErrLeaseNotFound
	  }
	   if !KnownFaultKind(kind) {
		  return nil, ErrFaultKind
	   }
	   if err := validateOperator(operator); err != nil {
		  return nil, err
	  }
	   if lease.Status == LeaseSealed {
		  return nil, ErrSealed
	  }
	   fc := &FaultCase{
		  ID: uuid.NewString(),
		  LeaseID: lease.ID,
		  MovementID: lease.MovementID,
		   JobID:  lease.JobID,
		  Kind:   kind,
		  Status: FaultInjected,
		  Operator: strings.TrimSpace(operator),
		   IsolationMark: IsolationMark,
		   InjectedAt: i.clock(),
	  }
	   if err := i.store.SaveFault(ctx, fc); err != nil {
		  return nil, err
	   }
	  note, state, err := i.observeKind(ctx, lease, kind)
	  fc.Note = note
	   fc.ObservedState = state
	  fc.Status = FaultObserved
	   fc.ObservedAt = i.clock()
	   if saveErr := i.store.SaveFault(ctx, fc); saveErr != nil {
		   return nil, saveErr
	   }
	   if err != nil {
		   cp := fc.Copy()
		   return &cp, err
	  }
	   cp := fc.Copy()
	   return &cp, nil
   }

  func (i *FaultInjector) observeKind(ctx context.Context, lease *InflightLease, kind FaultKind) (string, string, error) {
	  job, err := optionalObserve(ctx, i.obs, lease.JobID)
	  state := ""
	   if job != nil {
		  state = job.State
	   }
	  switch kind {
	  case FaultActor:
		   return "actor refused", state, ErrDualActor
	   case FaultExpire:
		  return "expired", "expired", ErrLeaseExpired
	  case FaultFence:
		  return "stale fence", state, ErrFenceStale
	   case FaultHSMTimeout:
		   return "hsm timeout", state, ErrPipeUnavailable
	  case FaultCanonicalMismatch:
		   return "canonical mismatch", state, ErrCanonical
	   case FaultDualBroadcast:
		   return "dual broadcast blocked", state, ErrDoubleArtifact
	   case FaultRetryExhausted:
		  return "retries exhausted", state, ErrRetriesExhausted
	  case FaultGenerationStale:
		  return "stale generation", state, ErrStaleGen
	   case FaultInflightRace:
		   return "inflight race", state, ErrConcurrentRetry
	   default:
		  if err != nil && err != ErrObserveUnavailable {
			   return err.Error(), state, err
		   }
		  return "signer fault", "failed", ErrFaultMustFail
	   }
   }

  func (i *FaultInjector) Close(ctx context.Context, faultID, operator, reason string) (*FaultCase, error) {
	   if err := validateOperator(operator); err != nil {
		  return nil, err
	  }
	   if err := validateReason(reason); err != nil {
		  return nil, err
	   }
	   fc, err := i.store.GetFault(ctx, faultID)
	   if err != nil {
		   return nil, err
	   }
	  if fc.Status == FaultClosed {
		  return nil, ErrFaultClosed
	  }
	  fc.Status = FaultClosed
	  fc.ClosedAt = i.clock()
	   fc.Note = strings.TrimSpace(fc.Note + " | " + reason)
	  if err := i.store.SaveFault(ctx, fc); err != nil {
		  return nil, err
	   }
	   cp := fc.Copy()
	   return &cp, nil
   }

  func optionalObserve(ctx context.Context, obs JobObserver, jobID string) (*ObservedJob, error) {
	  if obs == nil {
		  return nil, ErrObserveUnavailable
	   }
	   j,
		   err := obs.Observe(ctx, jobID)
	   if err != nil {
		  return nil, err
	   }
	   return j, nil
   }

   func faultBlocksRetry(kind FaultKind) bool {
	  switch kind {
	   case FaultDualBroadcast,
		   FaultRetryExhausted, FaultExpire:
		  return true
	  default:
		  return false
	   }
  }

  func describeFault(kind FaultKind) string {
	   return fmt.Sprintf("fault:%s", kind)
   }
