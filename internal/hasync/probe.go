package hasync

import (
	"context"
	"errors"
	"account-assets.kucoin.net/funding/internal/ha"
)

// ProbeNow 读就绪、选主、脑裂。不改围栏。

  func (s *Service) ProbeNow(region string) Probe {
	  if s == nil {
		   return Probe{}
	   }
	   label := ha.RegionLabel(region)
	  if label == "" && s.pair != nil {
		   label = ha.RegionLabel(s.pair.ActiveRegion())
	   }
	   p := Probe{
		  Region: region, Label: label, CIDR: ha.RegionCIDR(region),
		   Ready: s.Ready(), At: s.now(),
	   }
	  if s.pair != nil {
		   p.Leader = s.pair.RequireWrite() == nil
		   p.Split = s.pair.SplitBrain()
		   p.Active = s.pair.ActiveRegion()
	  }
	  s.remember(p)
	   return p
  }

// Campaign 争选主。未就绪失败即关。

   func (s *Service) Campaign(ctx context.Context, actor string) (CampaignRec, error) {
	   if s == nil {
		  return CampaignRec{}, ErrUnavailable
	  }
	   if _, err := requireActor(actor); err != nil {
		   return CampaignRec{}, err
	  }
	  if s.Halted() {
		   return CampaignRec{}, ErrHalt
	   }
	   if !s.Ready() {
		  return CampaignRec{}, ErrNotReady
	   }
	  if s.pair == nil {
		   return CampaignRec{Leader: true, Note: "single process"}, nil
	   }
	  rec, err := s.pair.Campaign(ctx)
	   out := CampaignRec{Region: rec.Region, Leader: rec.Leader, Epoch: rec.Epoch, Split: s.pair.SplitBrain()}
	  if err != nil {
		  if s.pair.SplitBrain() {
			   return out, ErrSplit
		  }
		   return out, err
	   }
	  return out, nil
   }

// GuardWrite 就绪且持主才执行 fn。脑裂或未就绪失败即关。

   func (s *Service) GuardWrite(ctx context.Context, actor string, fn func() error) (WriteRec, error) {
	   if s == nil {
		  return WriteRec{}, ErrUnavailable
	  }
	   if _, err := requireActor(actor); err != nil {
		  return WriteRec{}, err
	  }
	  if s.Halted() {
		  return WriteRec{Reason: ErrHalt.Error(), Probe: s.ProbeNow("")}, ErrHalt
	  }
	  if !s.Ready() {
		  return WriteRec{Reason: ErrNotReady.Error(), Probe: s.ProbeNow("")}, ErrNotReady
	   }
	  if s.pair != nil && s.pair.SplitBrain() {
		  return WriteRec{Reason: ErrSplit.Error(), Probe: s.ProbeNow("")}, ErrSplit
	   }
	   if s.pair != nil {
		   if err := s.pair.GuardWrite(fn); err != nil {
			   if errors.Is(err, ha.ErrNotLeader) || errors.Is(err, ha.ErrLostLease) {
				  return WriteRec{Reason: ErrFence.Error(), Probe: s.ProbeNow("")}, ErrFence
			   }
			  return WriteRec{Reason: err.Error(), Probe: s.ProbeNow("")}, err
		   }
		  return WriteRec{OK: true, Probe: s.ProbeNow("")}, nil
	  }
	   if fn != nil {
		   if err := fn(); err != nil {
			  return WriteRec{Reason: err.Error(), Probe: s.ProbeNow("")}, err
		  }
	  }
	  return WriteRec{OK: true, Probe: s.ProbeNow("")}, nil
  }

   func (s *Service) Failover(ctx context.Context, actor, reason string) (CampaignRec, error) {
	  if s == nil {
		   return CampaignRec{},
			   ErrUnavailable
	   }
	  if _, err := requireActor(actor); err != nil {
		   return CampaignRec{}, err
	  }
	   if s.Halted() {
		  return CampaignRec{}, ErrHalt
	  }
	   if !s.Ready() {
		   return CampaignRec{}, ErrNotReady
	  }
	   if s.pair == nil {
		   return CampaignRec{}, ErrFailover
	  }
	  if s.pair.SplitBrain() {
		  return CampaignRec{Split: true},
			  ErrSplit
	   }
	   rec, err := s.pair.Failover(ctx, reason)
	   out := CampaignRec{Region: rec.Region, Leader: rec.Leader, Epoch: rec.Epoch, Split: s.pair.SplitBrain(), Note: reason}
	  if err != nil {
		  return out, ErrFailover
	   }
	   return out, nil
   }

   func (s *Service) WireReady(ctx context.Context, readyCode int) error {
	   if s == nil {
		   return ErrUnavailable
	  }
	   s.SetReady(readyCode < 400)
	  if !s.Ready() {
		  return ErrNotReady
	  }
	   _ = ctx
	   return nil
  }
