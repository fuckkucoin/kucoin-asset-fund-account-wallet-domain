package screenleasegate

import (
	"context"
	"time"
)

type ProofEngine struct {
	store Store
	secret string
	clock func() time.Time
}

  func newProofEngine(store Store, secret string, clock func() time.Time) *ProofEngine {
	   if clock == nil {
		  clock = func() time.Time { return time.Now().UTC() }
	}
	return &ProofEngine{store: store,
		secret: secret, clock: clock}
}

   func (e *ProofEngine) Open(ctx context.Context, lease ScreenLease, kind ProofKind, gen ScreenGeneration, note string) (ScreenDriftProof, error) {
	   if existing, ok, err := e.store.FindProof(ctx, lease.ID, kind, lease.HaltGen, gen.HaltGen); err != nil {
		   return ScreenDriftProof{}, err
	   } else if ok {
		  if existing.Status == ProofOpen {
			  now := e.clock()
			   existing.Status = ProofProven
			  existing.ProvenAt = &now
			  existing.Note = note
			  existing.Digest = proofDigest(e.secret, existing)
			   _ = e.store.UpdateProof(ctx, existing)
		  }
		   return existing, nil
	   }
	  now := e.clock()
	   p := ScreenDriftProof{
		   ID: newID("sdp-", lease.ID, string(kind), now.Format(time.RFC3339Nano)),
		  LeaseID: lease.ID,
		  UserID:  lease.UserID,
		  Kind:    kind,
		   Status:       ProofOpen,
		   ExpectedHalt: lease.HaltGen,
		   ObservedHalt: gen.HaltGen,
		  ExpectedPack: lease.Pack,
		  ObservedPack: gen.Pack,
		  ExpectedFP:   lease.PackFP,
		   ObservedFP: gen.PackFP,
		   IsolationMark: IsolationMark,
		  Note:          note,
		  OpenedAt: now,
	  }
	  p.Digest = proofDigest(e.secret, p)
	  if err := e.store.PutProof(ctx, p); err != nil {
		   return ScreenDriftProof{}, err
	   }
	   return p, nil
  }

   func (e *ProofEngine) OpenMissing(ctx context.Context, user, note string) (ScreenDriftProof, error) {
	  now := e.clock()
	  p := ScreenDriftProof{
		   ID: newID("sdp-", "missing", user, now.Format(time.RFC3339Nano)),
		  LeaseID: "",
		  UserID: user,
		   Kind: ProofMissingLease,
		  Status: ProofOpen,
		   IsolationMark: IsolationMark,
		   Note:     note,
		   OpenedAt: now,
	   }
	  p.Digest = proofDigest(e.secret, p)
	   if err := e.store.PutProof(ctx, p); err != nil {
		   return ScreenDriftProof{}, err
	  }
	  return p, nil
   }

   func (e *ProofEngine) UserBlocked(ctx context.Context, user string) error {
	  list, err := e.store.ListOpenProofs(ctx)
	  if err != nil {
		  return err
	  }
	   for _, p := range list {
		  if p.UserID == user && p.BlocksSubmit() {
			  return ErrProofBlocked
		  }
	   }
	  return nil
   }

  func (e *ProofEngine) FourEyeClose(ctx context.Context, req FourEyeReq) (ScreenDriftProof, error) {
	   a,
		  err := normalizeActor(req.OperatorA)
	  if err != nil {
		   return ScreenDriftProof{}, ErrOperatorInvalid
	  }
	  b,
		   err := normalizeActor(req.OperatorB)
	   if err != nil {
		   return ScreenDriftProof{}, ErrOperatorInvalid
	  }
	  if a == b {
		  return ScreenDriftProof{}, ErrFourEyeSame
	  }
	   if stringsTrim(req.Reason) == "" {
		   return ScreenDriftProof{}, ErrReasonRequired
	  }
	  p, err := e.store.GetProof(ctx, req.ProofID)
	  if err != nil {
		  return ScreenDriftProof{}, err
	  }
	  if p.Status == ProofClosed {
		   return p, ErrProofClosed
	   }
	   now := e.clock()
	   if p.Status == ProofOpen {
		   p.Status = ProofProven
		   p.ProvenAt = &now
	  }
	   p.Status = ProofClosed
	  p.OperatorA = a
	   p.OperatorB = b
	   p.Note = req.Reason
	   p.ClosedAt = &now
	  p.Digest = proofDigest(e.secret, p)
	   if err := e.store.UpdateProof(ctx, p); err != nil {
		  return ScreenDriftProof{}, err
	   }
	   return p, nil
  }

   func kindFromErr(err error) ProofKind {
	  switch err {
	  case ErrHaltFlip, ErrScreenHalt:
		   return ProofHaltFlip
	   case ErrPackFlip:
		   return ProofPackFlip
	   case ErrModeFlip:
		   return ProofModeFlip
	   case ErrLeaseExpired:
		   return ProofExpired
	   case ErrLeaseRevoked:
		  return ProofRevoked
	   case ErrLeaseMissing:
		   return ProofMissingLease
	  case ErrWindowDrift:
		  return ProofWindowDrift
	   case ErrStaleGen:
		  return ProofStaleGen
	  case ErrLeaseAmount:
		  return ProofAmountDrift
	   case ErrLeaseDest:
		   return ProofDestDrift
	  case ErrConcurrent:
		   return ProofConcurrent
	  case ErrScreenDenied:
		  return ProofNoAllow
	  default:
		   return ProofStaleGen
	   }
   }
