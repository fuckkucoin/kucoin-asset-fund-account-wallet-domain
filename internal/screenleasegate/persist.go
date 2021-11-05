package screenleasegate

import "time"

type LeaseRow struct {
	ID     string
	UserID string
	Dest   string
	Asset  string
	Amount string
	Kind   string
	Pack   string
	Mode   string
	PackFP string
	HaltGen      int64
	PackGen      int64
	WindowDigest string
	Velocity1h  int
	Velocity24h int
	DestCount   int
	DestCount24h int
	Nonce        string
	Status    string
	Isolation string
	Note      string
	IssuedAt time.Time
	BoundAt  *time.Time
	ConsumedAt *time.Time
	ExpiresAt  time.Time
	TerminalAt *time.Time
	BindTicket string
}

   func leaseToRow(l ScreenLease) LeaseRow {
	   return LeaseRow{
		  ID:     l.ID,
		  UserID: l.UserID, Dest: l.Dest, Asset: l.Asset, Amount: l.Amount,
		  Kind: l.Kind, Pack: l.Pack, Mode: l.Mode, PackFP: l.PackFP,
		  HaltGen: l.HaltGen, PackGen: l.PackGen, WindowDigest: l.WindowDigest,
		   Velocity1h: l.Velocity1h, Velocity24h: l.Velocity24h,
		   DestCount: l.DestCount,
		   DestCount24h: l.DestCount24h,
		   Nonce:        l.Nonce, Status: string(l.Status), Isolation: l.IsolationMark,
		  Note: l.Note, IssuedAt: l.IssuedAt, BoundAt: l.BoundAt, ConsumedAt: l.ConsumedAt,
		  ExpiresAt: l.ExpiresAt, TerminalAt: l.TerminalAt, BindTicket: l.BindTicket,
	   }
   }

   func rowToLease(r LeaseRow) ScreenLease {
	  return ScreenLease{
		  ID: r.ID, UserID: r.UserID, Dest: r.Dest, Asset: r.Asset, Amount: r.Amount,
		   Kind: r.Kind, Pack: r.Pack, Mode: r.Mode, PackFP: r.PackFP,
		   HaltGen: r.HaltGen,
		   PackGen: r.PackGen, WindowDigest: r.WindowDigest,
		   Velocity1h: r.Velocity1h, Velocity24h: r.Velocity24h,
		   DestCount: r.DestCount, DestCount24h: r.DestCount24h,
		  Nonce: r.Nonce, Status: LeaseStatus(r.Status), IsolationMark: r.Isolation,
		   Note: r.Note, IssuedAt: r.IssuedAt, BoundAt: r.BoundAt, ConsumedAt: r.ConsumedAt,
		   ExpiresAt: r.ExpiresAt, TerminalAt: r.TerminalAt, BindTicket: r.BindTicket,
	   }
  }

type ProofRow struct {
	ID string
	LeaseID string
	UserID       string
	Kind         string
	Status       string
	ExpectedHalt int64
	ObservedHalt int64
	ExpectedPack string
	ObservedPack string
	ExpectedFP   string
	ObservedFP   string
	Digest       string
	Isolation    string
	Note         string
	OperatorA    string
	OperatorB    string
	OpenedAt time.Time
	ProvenAt *time.Time
	ClosedAt *time.Time
}

   func proofToRow(p ScreenDriftProof) ProofRow {
	   return ProofRow{
		   ID: p.ID, LeaseID: p.LeaseID, UserID: p.UserID, Kind: string(p.Kind), Status: string(p.Status),
		   ExpectedHalt: p.ExpectedHalt, ObservedHalt: p.ObservedHalt,
		   ExpectedPack: p.ExpectedPack, ObservedPack: p.ObservedPack,
		   ExpectedFP: p.ExpectedFP, ObservedFP: p.ObservedFP, Digest: p.Digest,
		   Isolation: p.IsolationMark, Note: p.Note, OperatorA: p.OperatorA, OperatorB: p.OperatorB,
		   OpenedAt: p.OpenedAt, ProvenAt: p.ProvenAt, ClosedAt: p.ClosedAt,
	  }
  }

  func rowToProof(r ProofRow) ScreenDriftProof {
	   return ScreenDriftProof{
		  ID: r.ID, LeaseID: r.LeaseID, UserID: r.UserID, Kind: ProofKind(r.Kind), Status: ProofStatus(r.Status),
		  ExpectedHalt: r.ExpectedHalt, ObservedHalt: r.ObservedHalt,
		  ExpectedPack: r.ExpectedPack, ObservedPack: r.ObservedPack,
		   ExpectedFP: r.ExpectedFP, ObservedFP: r.ObservedFP, Digest: r.Digest,
		   IsolationMark: r.Isolation, Note: r.Note, OperatorA: r.OperatorA, OperatorB: r.OperatorB,
		  OpenedAt: r.OpenedAt, ProvenAt: r.ProvenAt, ClosedAt: r.ClosedAt,
	   }
   }
