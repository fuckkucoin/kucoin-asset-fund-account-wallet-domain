package holdopsgate

import (
	"context"
	"time"
)

type ProofEngine struct {
	store Store
	secret string
	clock  func() time.Time
}

  func newProofEngine(store Store, secret string, clock func() time.Time) *ProofEngine {
	   return &ProofEngine{store: store, secret: secret, clock: clock}
  }

  func (e *ProofEngine) Open(ctx context.Context, p HoldOpsRaceProof) (HoldOpsRaceProof, error) {
	   if e == nil || e.store == nil {
		   return HoldOpsRaceProof{}, ErrUnavailable
	  }
	   now := e.clock()
	   if p.ID == "" {
		   p.ID = newProofID(e.secret, string(p.Kind), p.TokenID, p.GapID, p.LoserLease)
	  }
	  if p.Status == "" {
		  p.Status = ProofOpen
	   }
	   p.IsolationMark = IsolationMark
	   p.OpenedAt = now
	  if p.Status == ProofProven && p.ProvenAt.IsZero() {
		  p.ProvenAt = now
	   }
	  if err := e.store.PutProof(ctx, p); err != nil {
		   return HoldOpsRaceProof{}, err
	  }
	   return p, nil
  }

   func (e *ProofEngine) OpenMissing(ctx context.Context, userID, note string) (HoldOpsRaceProof, error) {
	  return e.Open(ctx, HoldOpsRaceProof{
		   UserID: userID,
		   Kind:   ProofMissingLease,
		  Status: ProofProven,
		  Note: note,
	  })
  }

  func (e *ProofEngine) OpenDualRelease(ctx context.Context, winner, loser HoldOpsLease) (HoldOpsRaceProof, error) {
	  return e.Open(ctx, HoldOpsRaceProof{
		   LeaseID: loser.ID,
		   OpsID:   loser.OpsID,
		   TokenID: loser.TokenID,
		  GapID:  loser.GapID,
		  UserID: loser.UserID,
		   Kind:   ProofDualRelease,
		  Status: ProofProven,
		   ExpectedGen: winner.OpsGen,
		   ObservedGen: loser.OpsGen,
		  WinnerLease: winner.ID,
		   LoserLease:  loser.ID,
		   Note: "concurrent release: only winner consumed",
	  })
  }

  func (e *ProofEngine) OpenReleaseClose(ctx context.Context, winner, loser HoldOpsLease) (HoldOpsRaceProof, error) {
	   return e.Open(ctx, HoldOpsRaceProof{
		  LeaseID: loser.ID,
		   OpsID:   loser.OpsID,
		  TokenID: firstNonEmpty(loser.TokenID, winner.TokenID),
		  GapID:  firstNonEmpty(loser.GapID, winner.GapID),
		   UserID: loser.UserID,
		  Kind:   ProofReleaseClose,
		  Status:      ProofProven,
		  ExpectedGen: winner.OpsGen,
		  ObservedGen: loser.OpsGen,
		   WinnerLease: winner.ID,
		  LoserLease:  loser.ID,
		   Note:        "release vs gap close: loser fail closed",
	   })
  }

   func (e *ProofEngine) OpenGapOpenToken(ctx context.Context, lease HoldOpsLease, n int) (HoldOpsRaceProof, error) {
	   return e.Open(ctx, HoldOpsRaceProof{
		  LeaseID: lease.ID,
		  OpsID:   lease.OpsID,
		   TokenID: lease.TokenID,
		  GapID:   lease.GapID,
		   UserID:      lease.UserID,
		   Kind:        ProofGapOpenToken,
		  Status:      ProofProven,
		   ExpectedGen: lease.OpsGen,
		   ObservedGen: lease.OpsGen,
		   LoserLease:     lease.ID,
		   OpenTokenCount: n,
		   Note: "gap close denied while open token remains",
	  })
  }

  func (e *ProofEngine) OpenLeaseFail(ctx context.Context, lease HoldOpsLease, kind ProofKind, note string) (HoldOpsRaceProof, error) {
	   return e.Open(ctx, HoldOpsRaceProof{
		  LeaseID: lease.ID,
		   OpsID:   lease.OpsID,
		   TokenID: lease.TokenID,
		   GapID:   lease.GapID,
		  UserID: lease.UserID,
		  Kind: kind,
		   Status:      ProofProven,
		  ExpectedGen: lease.OpsGen,
		   LoserLease:  lease.ID,
		   Note: note,
	  })
   }

  func (e *ProofEngine) UserBlocked(ctx context.Context, userID string, writeKind OpsKind) error {
	   if writeKind != OpsRelease && writeKind != OpsGapClose {
		  return nil
	  }
	  list, err := e.store.ListProofsByUser(ctx, userID)
	  if err != nil {
		  return err
	  }
	   for _, p := range list {
		   if !p.BlocksWrites() {
			  continue
		   }
		   switch p.Kind {
		   case ProofAckNotRelease:
			   continue
		  case ProofDualRelease, ProofReleaseClose, ProofGapOpenToken, ProofConcurrent, ProofStaleGen:
			  return ErrProofBlocked
		   }
	  }
	  return nil
   }

  func (e *ProofEngine) FourEye(ctx context.Context, req FourEyeReq) (HoldOpsRaceProof, error) {
	  eye, err := normActor(req.Eye)
	   if err != nil {
		   return HoldOpsRaceProof{}, ErrOperatorInvalid
	  }
	  if stringsTrim(req.Reason) == "" {
		  return HoldOpsRaceProof{}, ErrReasonRequired
	  }
	  p, err := e.store.GetProof(ctx, stringsTrim(req.ProofID))
	   if err != nil {
		  return HoldOpsRaceProof{}, err
	   }
	  if p.Status == ProofClosed {
		  return p, ErrProofClosed
	   }
	  if p.EyeA == "" {
		  p.EyeA = eye
		  p.ClearReason = stringsTrim(req.Reason)
		  _ = e.store.UpdateProof(ctx, p)
		   return p, nil
	  }
	   if p.EyeA == eye {
		   return p, ErrFourEyeSame
	  }
	  to, ok := CanProofTransit(p.Status, "close")
	  if !ok {
		  return p, ErrProofClosed
	  }
	  p.EyeB = eye
	   p.Status = to
	  p.ClosedAt = e.clock()
	  p.ClearReason = firstNonEmpty(p.ClearReason, stringsTrim(req.Reason))
	   if err := e.store.UpdateProof(ctx, p); err != nil {
		  return p, err
	  }
	   return p, nil
  }
