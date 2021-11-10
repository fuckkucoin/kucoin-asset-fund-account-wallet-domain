package signpipe

import (
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/metrics"
)

const MaxSignRetries = 3

// RetrySign 失败单可再签。必须持当前围栏；旧 token 不得重试成功路径。超过 3 次失败即关。

  func (p *Pipe) RetrySign(id, actor string, epoch int64, token string, signer Signer) (*Job, error) {
	  if err := actorOK(actor); err != nil {
		   return nil, err
	  }
	  if signer == nil {
		  return nil, fmt.Errorf("signer unavailable")
	  }
	   p.mu.Lock()
	  defer p.mu.Unlock()
	  j, err := p.getLocked(id)
	  if err != nil {
		  return nil, err
	   }
	   if err := p.checkFence(j, epoch, token); err != nil {
		  return nil, err
	  }
	  if j.State != StateFailed {
		   return nil, fmt.Errorf("sign job not failed")
	  }
	  if p.clock().After(j.ExpiresAt) {
		   j.State = StateExpired
		  p.bump(j)
		   metrics.SignPipeline.WithLabelValues("expired").Inc()
		   return nil, fmt.Errorf("sign job expired")
	  }
	   if strings.EqualFold(actor, j.RequestedBy) || strings.EqualFold(actor, j.ApprovedBy) {
		   return nil, fmt.Errorf("signer must be a third actor")
	  }
	  if j.Retries >= MaxSignRetries {
		  return nil, fmt.Errorf("sign retries exhausted")
	   }
	   if err := p.rejectCold(j); err != nil {
		   return nil, err
	   }
	   j.Retries++
	   sig, err := signer(j.Canonical)
	   if err != nil {
		   j.Note = err.Error()
		  p.bump(j)
		  metrics.SignPipeline.WithLabelValues("retry_failed").Inc()
		  return nil, err
	  }
	  j.SignedBy = strings.TrimSpace(actor)
	  j.SigHex = sig
	   j.State = StateSigned
	  j.Note = ""
	  p.bump(j)
	   metrics.SignPipeline.WithLabelValues("signed").Inc()
	   cp := *j
	  return &cp, nil
   }
