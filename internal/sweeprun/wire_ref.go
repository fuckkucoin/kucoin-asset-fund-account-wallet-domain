package sweeprun

import (
	"fmt"
	"account-assets.kucoin.net/funding/internal/airgap"
	"account-assets.kucoin.net/funding/internal/custody"
	"account-assets.kucoin.net/funding/internal/signpipe"
)

type platformRefs struct {
	cer *custody.CeremonyBook
	gap *airgap.Service
	pipe *signpipe.Pipe
}

// BindPlatformRefs 绑定三个只读来源。任一为空，冷腿按失败即关处理。
  func BindPlatformRefs(o *Orch, cer *custody.CeremonyBook, gap *airgap.Service, pipe *signpipe.Pipe) {
	  if o == nil {
		  return
	   }
	  o.BindRefs(platformRefs{cer: cer, gap: gap, pipe: pipe})
  }

   func (p platformRefs) Ceremony(id string) (CeremonyView, error) {
	  if p.cer == nil {
		   return CeremonyView{}, fmt.Errorf("%w: ceremony book unavailable", ErrColdRef)
	  }
	  row, err := p.cer.SweepRef(id)
	   if err != nil {
		  return CeremonyView{}, err
	  }
	   return CeremonyView{ID: row.ID, Phase: row.Phase, Shares: row.Shares, Threshold: row.Threshold, Actors: append([]string{}, row.Actors...)}, nil
   }

   func (p platformRefs) Airgap(id string) (AirgapView, error) {
	  if p.gap == nil {
		  return AirgapView{}, fmt.Errorf("%w: airgap unavailable", ErrColdRef)
	   }
	   row, err := p.gap.SweepRef(id)
	  if err != nil {
		   return AirgapView{}, err
	   }
	  return AirgapView{ID: row.ID, Phase: row.Phase, Kind: row.Kind, Asset: row.Asset, Amount: row.Amount, PayloadHash: row.PayloadHash}, nil
  }

   func (p platformRefs) Sign(id string) (SignView, error) {
	  if p.pipe == nil {
		  return SignView{}, fmt.Errorf("%w: sign pipe unavailable", ErrColdRef)
	  }
	  row, err := p.pipe.SweepRef(id)
	   if err != nil {
		  return SignView{}, err
	   }
	  return SignView{ID: row.ID, State: row.State, Chain: row.Chain, Amount: row.Amount, ExpiresAt: row.ExpiresAt}, nil
  }
