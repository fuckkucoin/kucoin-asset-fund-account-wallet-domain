package scoreleasegate

import "time"

type leaseRow struct {
	ID string
	UserID string
	Chain string
	Dest  string
	Asset string
	Amount string
	Kind   string
	Pack   string
	Mode   string
	PackFP string
	HaltGen int64
	PackGen int64
	ModeGen int64
	WindowGen int64
	WindowDigest string
	Velocity1h int
	Velocity24h int
	DestCount    int
	DestCount24h int
	ClusterSize int
	Correlated  int
	NewDests    int
	Fingerprint string
	Nonce       string
	Status      string
	IsolationMark string
	Note          string
	IssuedAt time.Time
	BoundAt    *time.Time
	ConsumedAt *time.Time
	ExpiresAt time.Time
	TerminalAt *time.Time
	BindTicket string
	EvalAllow  bool
	EvalReason string
}

  func leaseToRow(l ScoreLease) leaseRow {
	   return leaseRow{
		  ID: l.ID, UserID: l.UserID, Chain: l.Chain, Dest: l.Dest, Asset: l.Asset,
		   Amount: l.Amount, Kind: l.Kind, Pack: l.Pack, Mode: l.Mode, PackFP: l.PackFP,
		   HaltGen: l.HaltGen, PackGen: l.PackGen, ModeGen: l.ModeGen, WindowGen: l.WindowGen,
		   WindowDigest: l.WindowDigest, Velocity1h: l.Velocity1h, Velocity24h: l.Velocity24h,
		   DestCount: l.DestCount, DestCount24h: l.DestCount24h, ClusterSize: l.ClusterSize,
		  Correlated: l.Correlated, NewDests: l.NewDests, Fingerprint: l.Fingerprint,
		   Nonce: l.Nonce, Status: string(l.Status), IsolationMark: l.IsolationMark, Note: l.Note,
		  IssuedAt: l.IssuedAt, BoundAt: l.BoundAt, ConsumedAt: l.ConsumedAt, ExpiresAt: l.ExpiresAt,
		   TerminalAt: l.TerminalAt, BindTicket: l.BindTicket, EvalAllow: l.EvalAllow, EvalReason: l.EvalReason,
	  }
   }

  func rowToLease(r leaseRow) ScoreLease {
	   return ScoreLease{
		  ID: r.ID, UserID: r.UserID, Chain: r.Chain, Dest: r.Dest, Asset: r.Asset,
		   Amount: r.Amount,
		   Kind:   r.Kind, Pack: r.Pack, Mode: r.Mode, PackFP: r.PackFP,
		  HaltGen: r.HaltGen, PackGen: r.PackGen, ModeGen: r.ModeGen, WindowGen: r.WindowGen,
		  WindowDigest: r.WindowDigest, Velocity1h: r.Velocity1h, Velocity24h: r.Velocity24h,
		  DestCount: r.DestCount, DestCount24h: r.DestCount24h, ClusterSize: r.ClusterSize,
		   Correlated: r.Correlated,
		  NewDests:   r.NewDests, Fingerprint: r.Fingerprint,
		   Nonce: r.Nonce, Status: LeaseStatus(r.Status), IsolationMark: r.IsolationMark, Note: r.Note,
		  IssuedAt: r.IssuedAt, BoundAt: r.BoundAt, ConsumedAt: r.ConsumedAt, ExpiresAt: r.ExpiresAt,
		   TerminalAt: r.TerminalAt, BindTicket: r.BindTicket, EvalAllow: r.EvalAllow, EvalReason: r.EvalReason,
	   }
  }

type proofRow struct {
	ID      string
	LeaseID string
	UserID string
	Kind   string
	Status string
	ExpectedHalt int64
	ObservedHalt int64
	ExpectedPack string
	ObservedPack string
	ExpectedMode string
	ObservedMode string
	ExpectedFP   string
	ObservedFP   string
	ExpectedWin    string
	ObservedWin    string
	ExpectedWinGen int64
	ObservedWinGen int64
	Digest        string
	IsolationMark string
	Note    string
	CloserA string
	CloserB string
	CloseReason string
	OpenedAt time.Time
	ProvenAt *time.Time
	ClosedAt *time.Time
}

   func proofToRow(p ScoreDriftProof) proofRow {
	   return proofRow{
		   ID: p.ID, LeaseID: p.LeaseID, UserID: p.UserID, Kind: string(p.Kind), Status: string(p.Status),
		   ExpectedHalt: p.ExpectedHalt, ObservedHalt: p.ObservedHalt, ExpectedPack: p.ExpectedPack,
		  ObservedPack: p.ObservedPack, ExpectedMode: p.ExpectedMode, ObservedMode: p.ObservedMode,
		  ExpectedFP: p.ExpectedFP,
		  ObservedFP: p.ObservedFP, ExpectedWin: p.ExpectedWin,
		   ObservedWin: p.ObservedWin, ExpectedWinGen: p.ExpectedWinGen, ObservedWinGen: p.ObservedWinGen,
		   Digest: p.Digest, IsolationMark: p.IsolationMark, Note: p.Note, CloserA: p.CloserA,
		   CloserB: p.CloserB, CloseReason: p.CloseReason, OpenedAt: p.OpenedAt, ProvenAt: p.ProvenAt,
		  ClosedAt: p.ClosedAt,
	  }
  }

  func rowToProof(r proofRow) ScoreDriftProof {
	   return ScoreDriftProof{
		  ID: r.ID, LeaseID: r.LeaseID, UserID: r.UserID, Kind: ProofKind(r.Kind), Status: ProofStatus(r.Status),
		  ExpectedHalt: r.ExpectedHalt, ObservedHalt: r.ObservedHalt, ExpectedPack: r.ExpectedPack,
		  ObservedPack: r.ObservedPack, ExpectedMode: r.ExpectedMode, ObservedMode: r.ObservedMode,
		   ExpectedFP: r.ExpectedFP, ObservedFP: r.ObservedFP, ExpectedWin: r.ExpectedWin,
		   ObservedWin: r.ObservedWin, ExpectedWinGen: r.ExpectedWinGen, ObservedWinGen: r.ObservedWinGen,
		   Digest: r.Digest, IsolationMark: r.IsolationMark, Note: r.Note, CloserA: r.CloserA,
		  CloserB: r.CloserB, CloseReason: r.CloseReason, OpenedAt: r.OpenedAt, ProvenAt: r.ProvenAt,
		   ClosedAt: r.ClosedAt,
	  }
  }
