package holdlayerapi

import (
	"context"
	"account-assets.kucoin.net/funding/internal/holdopsgate"
)

type Service struct {
	gate *holdopsgate.Service
}

   func New(gate *holdopsgate.Service) *Service {
	  if gate == nil {
		  gate = holdopsgate.NewService("", nil)
	   }
	   return &Service{gate: gate}
   }

   func (s *Service) Gate() *holdopsgate.Service { return s.gate }

  func (s *Service) Place(ctx context.Context, body PlaceBody) (OpsReply, error) {
	  lease, err := s.gate.Issue(ctx, holdopsgate.IssueReq{
		   Kind:   holdopsgate.OpsPlace,
		  UserID: body.UserID, Asset: body.Asset, Amount: body.Amount,
		  TokenID: body.TokenID, GapID: body.GapID, Ref: body.Ref, Reason: body.Reason,
		   Actor: body.Actor,
		   Nonce: body.Nonce,
	  })
	   if err != nil {
		  return OpsReply{}, err
	   }
	  got, proof, err := s.gate.Exec(ctx, holdopsgate.ExecReq{
		  LeaseID: lease.ID, Kind: holdopsgate.OpsPlace, UserID: body.UserID,
		  Asset: body.Asset, Amount: body.Amount, TokenID: lease.TokenID, GapID: body.GapID,
		  Actor: body.Actor, Consume: true,
	  })
	   return pack(s, ctx, got, proof, err)
   }

  func (s *Service) Release(ctx context.Context, body ReleaseBody) (OpsReply, error) {
	   leaseID := body.LeaseID
	   if leaseID == "" {
		   lease, err := s.gate.Issue(ctx, holdopsgate.IssueReq{
			  Kind: holdopsgate.OpsRelease, UserID: body.UserID, Asset: body.Asset, Amount: body.Amount,
			  TokenID: body.TokenID, GapID: body.GapID, Actor: body.Actor, Nonce: body.Nonce,
		  })
		   if err != nil {
			   return OpsReply{}, err
		  }
		   leaseID = lease.ID
	   }
	  got, proof, err := s.gate.Exec(ctx, holdopsgate.ExecReq{
		  LeaseID: leaseID, Kind: holdopsgate.OpsRelease, UserID: body.UserID,
		  Asset: body.Asset, Amount: body.Amount, TokenID: body.TokenID, GapID: body.GapID,
		   Actor: body.Actor, Ticket: body.Ticket, Consume: true,
	   })
	  return pack(s, ctx, got, proof, err)
   }

  func (s *Service) Ack(ctx context.Context, body GapBody) (OpsReply, error) {
	  lease, err := s.gate.Issue(ctx, holdopsgate.IssueReq{
		  Kind: holdopsgate.OpsGapAck, UserID: body.UserID, Asset: body.Asset, Amount: body.Amount,
		   TokenID: body.TokenID, GapID: body.GapID, Actor: body.Actor, Nonce: body.Nonce,
	  })
	  if err != nil {
		  return OpsReply{}, err
	   }
	   got, proof, err := s.gate.Exec(ctx, holdopsgate.ExecReq{
		  LeaseID: lease.ID, Kind: holdopsgate.OpsGapAck, UserID: body.UserID,
		  Asset: body.Asset, Amount: body.Amount, TokenID: body.TokenID, GapID: body.GapID,
		   Actor: body.Actor, Consume: false,
	   })
	  return pack(s, ctx, got, proof, err)
   }

  func (s *Service) Close(ctx context.Context, body GapBody) (OpsReply, error) {
	  leaseID := body.LeaseID
	   if leaseID == "" {
		  lease, err := s.gate.Issue(ctx, holdopsgate.IssueReq{
			  Kind: holdopsgate.OpsGapClose, UserID: body.UserID, Asset: body.Asset, Amount: body.Amount,
			  TokenID: body.TokenID, GapID: body.GapID, Actor: body.Actor, Nonce: body.Nonce,
		  })
		  if err != nil {
			  return OpsReply{}, err
		  }
		  leaseID = lease.ID
	  }
	  got, proof, err := s.gate.Exec(ctx, holdopsgate.ExecReq{
		  LeaseID: leaseID,
		   Kind: holdopsgate.OpsGapClose, UserID: body.UserID,
		   Asset: body.Asset, Amount: body.Amount, TokenID: body.TokenID, GapID: body.GapID,
		   Actor: body.Actor, Ticket: body.Ticket, Consume: true,
	   })
	   return pack(s,
		   ctx, got, proof, err)
   }

  func pack(s *Service, ctx context.Context, lease *holdopsgate.HoldOpsLease, proof *holdopsgate.HoldOpsRaceProof, err error) (OpsReply, error) {
	   gen, _ := s.gate.GetGeneration(ctx)
	  rep := OpsReply{Lease: lease, Proof: proof, Gen: gen.OpsGen}
	  return rep, err
   }
