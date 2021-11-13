package signpipegate

// Transition 描述一条合法边。

type Transition struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Event string `json:"event"`
	Guard string `json:"guard"`
	Effect string `json:"effect"`
}

   func LeaseTransitions() []Transition {
	   return []Transition{
		   {From: "armed", To: "inflight", Event: "enter", Guard: "generation==current && now<ttl && no artifact", Effect: "hold movement mutex"},
		   {From: "armed", To: "expired", Event: "ttl", Guard: "now>=expires_at", Effect: "open closeout ttl"},
		  {From: "armed", To: "aborted", Event: "abort", Guard: "not sealed", Effect: "open closeout abort"},
		  {From: "armed", To: "superseded", Event: "later_gen", Guard: "current>lease.gen", Effect: "open closeout supersede"},
		   {From: "inflight", To: "sealed", Event: "seal", Guard: "sig present && artifact unique", Effect: "bind SignedArtifact"},
		   {From: "inflight", To: "expired", Event: "ttl", Guard: "now>=expires_at", Effect: "forbid sign"},
		   {From: "inflight", To: "aborted", Event: "fault_block", Guard: "kind in expire|exhausted|dual", Effect: "open closeout"},
		   {From: "inflight", To: "superseded", Event: "cas_lost", Guard: "generation mismatch", Effect: "release mutex"},
		   {From: "sealed", To: "sealed", Event: "*", Guard: "terminal", Effect: "reject"},
		  {From: "expired", To: "expired", Event: "seal", Guard: "terminal", Effect: "ErrLeaseExpired"},
		   {From: "aborted", To: "aborted", Event: "seal", Guard: "terminal", Effect: "ErrAborted"},
	  }
  }

   func FaultTransitions() []Transition {
	   return []Transition{
		   {From: "injected", To: "observed", Event: "observe", Guard: "kind known", Effect: "record observed_state"},
		  {From: "observed", To: "closed", Event: "ops_close", Guard: "operator+reason", Effect: "clear block if allowed"},
		   {From: "closed", To: "closed", Event: "*", Guard: "terminal", Effect: "ErrFaultClosed"},
	   }
   }

  func CloseoutTransitions() []Transition {
	  return []Transition{
		  {From: "open", To: "proven", Event: "digest", Guard: "hmac attach", Effect: "block retry"},
		  {From: "proven", To: "closed", Event: "ops_close", Guard: "operator+reason+digest ok", Effect: "release block"},
		  {From: "closed", To: "closed", Event: "*", Guard: "terminal", Effect: "ErrCloseoutClosed"},
	   }
   }

   func CanTransit(table []Transition, from, to, event string) bool {
	  for _, t := range table {
		  if t.From == from && t.To == to && (t.Event == event || t.Event == "*") {
			   return true
		   }
	   }
	   return false
  }

  func LeaseCan(from LeaseStatus,
	   to LeaseStatus, event string) bool {
	  return CanTransit(LeaseTransitions(), string(from), string(to), event)
  }
