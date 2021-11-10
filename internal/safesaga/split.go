package safesaga

import (
	"context"
	"account-assets.kucoin.net/funding/internal/ha"
	"account-assets.kucoin.net/funding/internal/validate"
)

// RefuseSplit 脑裂或无主时不得 Open/Advance。
  func (s *Service) RefuseSplit() error {
	  if s == nil {
		  return ErrUnavailable
	  }
	  if s.Halted() {
		   return ErrHalt
	  }
	   return s.requireWrite()
  }

   func (s *Service) ActiveRegion() string {
	   if s == nil || s.pair == nil {
		   return ""
	   }
	   return s.pair.ActiveRegion()

// AdvanceOrRefuse 先过双区闸再推进。脑裂不消耗围栏。

   func (s *Service) AdvanceOrRefuse(ctx context.Context, id, actor string, epoch int64, token string) (Run, error) {
	   if err := s.RefuseSplit(); err != nil {
		  return Run{}, err
	  }
	  if _, err := validate.Actor(actor); err != nil {
		   return Run{}, ErrActor
	   }
	  return s.Advance(ctx, id, actor, epoch, token)
  }

// BindCampaign 争主后再绑。两边都自称主则 MarkSplit。

  func BindCampaign(s *Service, pair *ha.Pair) (*Service, error) {
	  if s == nil || pair == nil {
		  return s, ErrUnavailable
	   }
	  if pair.SplitBrain() {
		   s.BindPair(pair)
		   return s, ErrSplit
	  }
	   if _, err := pair.Campaign(context.Background()); err != nil {
		  s.BindPair(pair)
		  return s, ErrSplit
	  }
	  s.BindPair(pair)
	  return s, nil
  }

  func (s *Service) RegionNote() map[string]any {
	   if s == nil || s.pair == nil {
		   return map[string]any{
			   "host": "pg-0.kucoin.net",
			   "sg":  "10.18.0.0/16",
			  "tyo": "10.19.0.0/16",
		  }
	  }
	  st := s.pair.Status()
	  st["host"] = "pg-0.kucoin.net"
	   st["sg"] = "10.18.0.0/16"
	   st["tyo"] = "10.19.0.0/16"
	   return st
  }
