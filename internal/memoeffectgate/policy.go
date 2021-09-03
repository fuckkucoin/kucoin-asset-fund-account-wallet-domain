package memoeffectgate

type PolicyRow struct {
	Name string
	TicketPending bool
	HasEffect     bool
	Occupied      bool
	AutoClaim bool
	ObserveOK    bool
	ApplyOK      bool
	Reject       bool
	AllowApprove bool
	NeedProof    bool
	BlockSecond bool
}

   func DefaultPolicyMatrix() []PolicyRow {
	  return []PolicyRow{
		   {Name: "happy", TicketPending: true, HasEffect: true, ObserveOK: true, ApplyOK: true, AllowApprove: true},
		  {Name: "auto-claim occupied", TicketPending: true, Occupied: true, AutoClaim: true, AllowApprove: false, NeedProof: true},
		   {Name: "other holder", TicketPending: true, Occupied: true, AllowApprove: false, NeedProof: true},
		   {Name: "not pending", TicketPending: false, HasEffect: false, AllowApprove: false, NeedProof: true},
		   {Name: "missing effect after bind fail", TicketPending: true, HasEffect: false, AllowApprove: false},
		  {Name: "observe fail half-cut", TicketPending: true, HasEffect: true, ObserveOK: false, AllowApprove: false, NeedProof: true},
		   {Name: "apply fail half-cut", TicketPending: true, HasEffect: true, ObserveOK: true, ApplyOK: false, AllowApprove: false, NeedProof: true, BlockSecond: true},
		  {Name: "reject after credit", TicketPending: true, HasEffect: true, ObserveOK: true, ApplyOK: true, Reject: true, NeedProof: true},
		   {Name: "reject no credit", TicketPending: true, HasEffect: true, Reject: true, AllowApprove: false},
		  {Name: "retry resume", TicketPending: true, HasEffect: true, ObserveOK: true, ApplyOK: false, BlockSecond: true},
		   {Name: "expired bound", TicketPending: true, HasEffect: true, AllowApprove: false},
		   {Name: "idem replay credited", TicketPending: true, HasEffect: true, ObserveOK: true, ApplyOK: true, AllowApprove: true},
		   {Name: "second full credit", TicketPending: true, HasEffect: true, ObserveOK: true, ApplyOK: true, BlockSecond: true},
		  {Name: "quarantine reject", TicketPending: true, HasEffect: true, ObserveOK: true, Reject: true, NeedProof: true},
		  {Name: "force reverse reject", TicketPending: true, HasEffect: true, ObserveOK: true, Reject: true, NeedProof: true},
	  }
  }

   func EvaluatePolicy(row PolicyRow) error {
	   if err := FailClosedApprove(row.TicketPending, row.HasEffect, row.Occupied, row.AutoClaim); err != nil {
		   if row.AllowApprove {
			  return err
		  }
		   return nil
	  }
	  if row.BlockSecond {
		   if err := MustBlockSecondCredit(2, false); err == nil && !row.AllowApprove {
			  return ErrSecondFullCredit
		   }
	   }
	  if row.Reject && row.ObserveOK {
		   if err := MustBlockSilentKeep(true, true, row.NeedProof); err != nil {
			   return err
		   }
	   }
	   return nil
  }
