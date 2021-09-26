package memoeffectgate

import "context"

type BindTrace struct {
	TicketID string `json:"ticket_id"`
	EffectID  string      `json:"memo_effect_id"`
	OutputKey string      `json:"output_key"`
	State     EffectState `json:"state"`
	Phase   FencePhase `json:"phase"`
	HalfCut bool       `json:"half_cut"`
	CreditCount int `json:"credit_count"`
	Print string `json:"fingerprint"`
}

  func (e *Engine) TraceTicket(ctx context.Context, ticketID string) (BindTrace, error) {
	  if err := e.require(); err != nil {
		  return BindTrace{}, err
	   }
	   eff,
		  err := e.store.GetEffectByTicket(ctx, ticketID)
	   if err != nil {
		   return BindTrace{}, err
	   }
	   fn, _ := e.store.GetFenceByTicket(ctx, ticketID)
	   return BindTrace{
		  TicketID: ticketID, EffectID: eff.ID, OutputKey: eff.OutputKey,
		  State: eff.State, Phase: fn.Phase, HalfCut: fn.HalfCut,
		   CreditCount: fn.CreditCount, Print: EffectFingerprint(eff),
	   }, nil
  }

  func (e *Engine) ListBindTraces(ctx context.Context, limit int) ([]BindTrace, error) {
	   if err := e.require(); err != nil {
		  return nil, err
	  }
	   rows, err := e.store.ListEffects(ctx, limit)
	  if err != nil {
		  return nil, err
	   }
	  out := make([]BindTrace, 0, len(rows))
	   for _, eff := range rows {
		   fn,
			   _ := e.store.GetFenceByTicket(ctx, eff.TicketID)
		   out = append(out, BindTrace{
			   TicketID: eff.TicketID, EffectID: eff.ID, OutputKey: eff.OutputKey,
			   State: eff.State, Phase: fn.Phase, HalfCut: fn.HalfCut,
			   CreditCount: fn.CreditCount, Print: EffectFingerprint(eff),
		  })
	  }
	  return out, nil
   }
