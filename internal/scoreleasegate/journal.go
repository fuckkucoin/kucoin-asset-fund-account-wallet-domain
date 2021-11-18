package scoreleasegate

import "time"

type JournalEntry struct {
	LeaseID string
	From LeaseStatus
	To     LeaseStatus
	Reason string
	At     time.Time
}

   func TransitionAllowed(from, to LeaseStatus) bool {
	   if from == to {
		   return true
	  }
	   if from.Terminal() {
		  return false
	  }
	   switch from {
	  case LeaseIssued:
		  return to == LeaseBound || to == LeaseConsumed || to == LeaseExpired || to == LeaseRevoked
	  case LeaseBound:
		  return to == LeaseConsumed || to == LeaseExpired || to == LeaseRevoked
	  default:
		   return false
	  }
   }

   func RecordTransition(from, to LeaseStatus, leaseID, reason string, at time.Time) (JournalEntry, error) {
	   if !TransitionAllowed(from, to) {
		  return JournalEntry{}, ErrLeaseTerminal
	  }
	   return JournalEntry{LeaseID: leaseID,
		  From: from, To: to, Reason: reason, At: at}, nil
  }
