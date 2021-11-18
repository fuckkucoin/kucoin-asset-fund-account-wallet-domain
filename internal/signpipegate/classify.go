package signpipegate

import (
	"strings"
	"time"
)

// ClassifyObserve 根据作业观察与时钟给出建议故障种类。

   func ClassifyObserve(job *ObservedJob, now time.Time) (FaultKind, CloseoutKind, string) {
	  if job == nil {
		  return FaultHSMTimeout, CloseFault, "observe empty"
	   }
	  st := strings.ToLower(strings.TrimSpace(job.State))
	   switch st {
	   case "failed":
		  if job.Retries >= MaxRetryGenerations {
			   return FaultRetryExhausted, CloseExhausted, "retries exhausted"
		   }
		   return FaultSigner, CloseFault, "signer failed"
	   case "expired":
		  return FaultExpire, CloseTTL, "job expired"
	   case "signed":
		  return FaultDualBroadcast, CloseDoubleGuard, "already signed"
	  case "cancelled":
		  return FaultActor, CloseAbort, "cancelled"
	   }
	  if !job.ExpiresAt.IsZero() && !now.Before(job.ExpiresAt) {
		  return FaultExpire, CloseTTL, "ttl crossed"
	   }
	  if strings.TrimSpace(job.Canonical) == "" {
		  return FaultCanonicalMismatch, CloseFault, "canonical empty"
	   }
	   return FaultSigner, CloseFault, "default signer surface"
  }

// ClassifyInjectError 将注入返回错误归类。

   func ClassifyInjectError(err error) FaultKind {
	  if err == nil {
		   return FaultSigner
	   }
	  switch err {
	   case ErrDualActor:
		   return FaultActor
	  case ErrLeaseExpired:
		   return FaultExpire
	  case ErrFenceStale:
		   return FaultFence
	   case ErrCanonical:
		  return FaultCanonicalMismatch
	   case ErrDoubleArtifact:
		  return FaultDualBroadcast
	   case ErrRetriesExhausted:
		   return FaultRetryExhausted
	   case ErrStaleGen, ErrSuperseded:
		   return FaultGenerationStale
	  case ErrConcurrentRetry, ErrInflightOccupied:
		   return FaultInflightRace
	   case ErrPipeUnavailable:
		  return FaultHSMTimeout
	   default:
		  return FaultSigner
	   }
  }

  func ShouldOpenCloseout(kind FaultKind) bool {
	  return faultBlocksRetry(kind)
  }

  func CloseoutFromFault(kind FaultKind) CloseoutKind {
	   switch kind {
	  case FaultExpire:
		  return CloseTTL
	   case FaultRetryExhausted:
		  return CloseExhausted
	  case FaultDualBroadcast:
		   return CloseDoubleGuard
	  case FaultActor:
		   return CloseAbort
	   case FaultGenerationStale, FaultInflightRace:
		   return CloseSupersede
	  default:
		  return CloseFault
	  }
   }
