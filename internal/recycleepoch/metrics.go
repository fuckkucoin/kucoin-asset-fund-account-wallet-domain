package recycleepoch

import (
	"sync/atomic"
)

// Counters 进程内计数，供 cmd 暴露 /metrics 文本或断言。
type Counters struct {
	MarkOK        atomic.Int64
	ReadyOK       atomic.Int64
	ReadyDenied   atomic.Int64
	AssignOK      atomic.Int64
	ObserveOK     atomic.Int64
	ObserveDenied atomic.Int64
	ProofOpened   atomic.Int64
	ProofAcked atomic.Int64
	StaleEpoch atomic.Int64
	MissingEpoch atomic.Int64
	ForcedReuse atomic.Int64
	LateOldUser atomic.Int64
}

  func (c *Counters) Snapshot() map[string]int64 {
	  if c == nil {
		   return map[string]int64{}
	   }
	   return map[string]int64{
		   "recycleepoch_mark_ok":      c.MarkOK.Load(),
		   "recycleepoch_ready_ok":     c.ReadyOK.Load(),
		  "recycleepoch_ready_denied": c.ReadyDenied.Load(),
		   "recycleepoch_assign_ok": c.AssignOK.Load(),
		   "recycleepoch_observe_ok":     c.ObserveOK.Load(),
		   "recycleepoch_observe_denied": c.ObserveDenied.Load(),
		  "recycleepoch_proof_opened": c.ProofOpened.Load(),
		  "recycleepoch_proof_acked":   c.ProofAcked.Load(),
		   "recycleepoch_stale_epoch":   c.StaleEpoch.Load(),
		   "recycleepoch_missing_epoch": c.MissingEpoch.Load(),
		  "recycleepoch_forced_reuse":  c.ForcedReuse.Load(),
		   "recycleepoch_late_old_user": c.LateOldUser.Load(),
	   }
   }

   func (c *Counters) NoteObserve(err error) {
	   if c == nil {
		  return
	  }
	  if err == nil {
		   c.ObserveOK.Add(1)
		   return
	   }
	  c.ObserveDenied.Add(1)
	   switch {
	  case err == ErrEpochStale:
		  c.StaleEpoch.Add(1)
	   case err == ErrEpochMissing:
		  c.MissingEpoch.Add(1)
	  case err == ErrForcedReuse:
		   c.ForcedReuse.Add(1)
	  case err == ErrWrongUserCredit, err == ErrQuarantined:
		   c.LateOldUser.Add(1)
	   }
  }

   func (c *Counters) NoteReady(err error) {
	  if c == nil {
		   return
	  }
	   if err == nil {
		  c.ReadyOK.Add(1)
		  return
	  }
	  c.ReadyDenied.Add(1)
	  if err == ErrForcedReuse || err == ErrCoolNotElapsed {
		   c.ForcedReuse.Add(1)
	   }
  }

   func (c *Counters) NoteProofOpen() {
	   if c != nil {
		   c.ProofOpened.Add(1)
	  }
  }

   func (c *Counters) NoteProofAck() {
	  if c != nil {
		  c.ProofAcked.Add(1)
	   }
  }
