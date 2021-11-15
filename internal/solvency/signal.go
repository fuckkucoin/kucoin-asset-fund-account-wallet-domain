package solvency

import (
	"context"
	"encoding/json"
)

const TopicDrift = "aa.solvency.drift"

   func (p *Pipeline) emitDrift(ctx context.Context, d DriftCase) {
	   if p == nil {
		   return
	  }
	  sig := DriftSignal{
		   DriftID: d.ID, AttestationID: d.AttestationID, Asset: d.Asset, Severity: d.Severity,
		   Liability: d.Liability, Observed: d.Observed, Deficit: d.Deficit, RatioBps: d.RatioBps,
		  At: d.OpenedAt, Topic: TopicDrift,
	  }
	   if p.bus != nil {
		   if body, err := json.Marshal(sig); err == nil {
			  if err := p.bus.Publish(ctx, TopicDrift, []byte(d.Asset), body); err == nil {
				   if err := p.store.MarkSignaled(ctx, d.ID); err == nil {
					  sig.Topic = TopicDrift
				   }
			  }
		   }
	  }
	  p.mu.Lock()
	   p.signals = append(p.signals, sig)
	   if len(p.signals) > 200 {
		   p.signals = append([]DriftSignal(nil), p.signals[len(p.signals)-200:]...)
	   }
	   p.mu.Unlock()
  }

// Signals 返回本进程已生成的漂移信号副本。

   func (p *Pipeline) Signals() []DriftSignal {
	  if p == nil {
		   return nil
	  }
	  p.mu.Lock()
	   defer p.mu.Unlock()
	   return append([]DriftSignal(nil), p.signals...)
   }
