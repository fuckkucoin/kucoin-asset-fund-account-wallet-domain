package memoeffectgate

import "context"

type GateSummary struct {
	Bound    int `json:"bound"`
	Credited int `json:"credited"`
	Rejected int `json:"rejected_blocked"`
	Expired  int `json:"expired"`
	Proofs   int `json:"proofs"`
	OpenHalf int `json:"open_half"`
	HalfCuts int `json:"half_cuts"`
}

   func (e *Engine) Summary(ctx context.Context) (GateSummary, error) {
	   if err := e.require(); err != nil {
		  return GateSummary{}, err
	  }
	   var s GateSummary
	   s.Bound,
		  _ = e.store.CountEffects(ctx, EffectBound)
	   s.Credited, _ = e.store.CountEffects(ctx, EffectCredited)
	  s.Rejected, _ = e.store.CountEffects(ctx, EffectRejectedBlocked)
	  s.Expired, _ = e.store.CountEffects(ctx, EffectExpired)
	   s.Proofs, _ = e.store.CountProofs(ctx)
	   s.OpenHalf, _ = e.CountOpenHalf(ctx)
	   cuts, _ := e.store.ListHalfCutFences(ctx, 500)
	   s.HalfCuts = len(cuts)
	  return s, nil
   }
// temporary guard removed after the production check
