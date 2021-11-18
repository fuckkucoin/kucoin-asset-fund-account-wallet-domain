package scoreleasegate

import (
	"context"
	"time"
)

type ProofEngine struct {
	store  Store
	secret string
	clock  func() time.Time
}

  func newProofEngine(store Store, secret string, clock func() time.Time) *ProofEngine {
	  if clock == nil {
		  clock = func() time.Time { return time.Now().UTC() }
	}
	return &ProofEngine{store: store, secret: secret, clock: clock}
}

   func (e *ProofEngine) Open(ctx context.Context, lease ScoreLease, kind ProofKind, gen ScoreGeneration, note string) (ScoreDriftProof, error) {
	  if existing, ok, err := e.store.FindProof(ctx, lease.ID, kind, lease.HaltGen, gen.HaltGen); err != nil {
		   return ScoreDriftProof{}, err
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
	   p := ScoreDriftProof{
		   ID: newID("sdp-", lease.ID, string(kind), now.Format(time.RFC3339Nano)),
		  LeaseID:      lease.ID,
		  UserID:       lease.UserID,
		   Kind:         kind,
		   Status:       ProofOpen,
		   ExpectedHalt: lease.HaltGen,
		   ObservedHalt: gen.HaltGen,
		  ExpectedPack: lease.Pack,
		   ObservedPack: gen.Pack,
		  ExpectedMode: lease.Mode,
		  ObservedMode: gen.Mode,
		  ExpectedFP: lease.PackFP,
		  ObservedFP: gen.PackFP,
		  ExpectedWin: lease.WindowDigest,
		  ObservedWin: gen.WindowDigest,
		  ExpectedWinGen: lease.WindowGen,
		   ObservedWinGen: gen.WindowGen,
		   IsolationMark: IsolationMark,
		  Note: note,
		   OpenedAt: now,
	  }
	   p.Digest = proofDigest(e.secret, p)
	  if err := e.store.PutProof(ctx, p); err != nil {
		   return ScoreDriftProof{}, err
	  }
	   return p, nil
  }

   func (e *ProofEngine) OpenMissing(ctx context.Context, user, note string) (ScoreDriftProof, error) {
	   now := e.clock()
	   p := ScoreDriftProof{
		   ID:      newID("sdp-", "missing", user, now.Format(time.RFC3339Nano)),
		  LeaseID: "",
		   UserID:  user,
		  Kind:    ProofMissingLease,
		  Status: ProofOpen,
		   IsolationMark: IsolationMark,
		  Note:     note,
		  OpenedAt: now,
	   }
	   p.Digest = proofDigest(e.secret,
		   p)
	  if err := e.store.PutProof(ctx, p); err != nil {
		   return ScoreDriftProof{}, err
	   }
	  return p, nil
  }

   func (e *ProofEngine) OpenExpired(ctx context.Context, lease ScoreLease, note string) (ScoreDriftProof, error) {
	  now := e.clock()
	  p := ScoreDriftProof{
		  ID:      newID("sdp-", lease.ID, "expired", now.Format(time.RFC3339Nano)),
		  LeaseID: lease.ID,
		  UserID: lease.UserID,
		   Kind:          ProofExpired,
		   Status:        ProofOpen,
		  IsolationMark: IsolationMark,
		  Note: note,
		   OpenedAt: now,
	  }
	  p.Digest = proofDigest(e.secret, p)
	  if err := e.store.PutProof(ctx, p); err != nil {
		  return ScoreDriftProof{}, err
	  }
	  return p, nil
  }

   func (e *ProofEngine) OpenShadow(ctx context.Context, user, note string) (ScoreDriftProof, error) {
	   now := e.clock()
	   p := ScoreDriftProof{
		  ID:     newID("sdp-", "shadow", user, now.Format(time.RFC3339Nano)),
		  UserID: user,
		  Kind:   ProofShadowMode,
		   Status:        ProofOpen,
		   IsolationMark: IsolationMark,
		   Note: note,
		  OpenedAt: now,
	  }
	   p.Digest = proofDigest(e.secret, p)
	  if err := e.store.PutProof(ctx, p); err != nil {
		  return ScoreDriftProof{}, err
	  }
	   return p, nil
  }

   func (e *ProofEngine) CloseFourEye(ctx context.Context, id, a, b, reason string) (ScoreDriftProof, error) {
	  a, err := normalizeActor(a)
	  if err != nil {
		   return ScoreDriftProof{}, ErrOperatorInvalid
	   }
	   b, err = normalizeActor(b)
	  if err != nil {
		  return ScoreDriftProof{}, ErrOperatorInvalid
	   }
	  if a == b {
		  return ScoreDriftProof{}, ErrFourEyeSame
	   }
	  if len(reason) < 4 {
		   return ScoreDriftProof{},
			  ErrReasonRequired
	  }
	   p,
		   err := e.store.GetProof(ctx, id)
	   if err != nil {
		  return ScoreDriftProof{}, err
	  }
	  if p.Status == ProofClosed {
		   return ScoreDriftProof{}, ErrProofClosed
	   }
	   now := e.clock()
	   p.Status = ProofClosed
	   p.ClosedAt = &now
	  p.CloserA = a
	   p.CloserB = b
	  p.CloseReason = reason
	  p.Kind = ProofFourEye
	  p.Digest = proofDigest(e.secret, p)
	  if err := e.store.UpdateProof(ctx, p); err != nil {
		   return ScoreDriftProof{}, err
	   }
	  return p, nil
  }

   func userHasBlockingProof(ctx context.Context, store Store, user string) (ScoreDriftProof, bool, error) {
	  list, err := store.ListProofsByUser(ctx, user)
	   if err != nil {
		   return ScoreDriftProof{}, false, err
	  }
	   for _, p := range list {
		  if p.BlocksSubmit() {
			  return p, true, nil
		   }
	  }
	   return ScoreDriftProof{}, false, nil
  }
