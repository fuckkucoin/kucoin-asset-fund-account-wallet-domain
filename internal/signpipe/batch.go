package signpipe

import "fmt"

type BatchRetryResult struct {
	Tried  int `json:"tried"`
	Signed int `json:"signed"`
	Failed int `json:"failed"`
	Jobs []Job `json:"jobs"`
}

// RetryFailed 把当前失败单再签一遍。每单必须持自己的围栏；超过 MaxSignRetries 失败即关。

  func (p *Pipe) RetryFailed(actor string, signer Signer) (BatchRetryResult, error) {
	  if p == nil {
		   return BatchRetryResult{}, fmt.Errorf("sign pipeline unavailable")
	  }
	   if err := actorOK(actor); err != nil {
		   return BatchRetryResult{}, err
	   }
	  if signer == nil {
		   return BatchRetryResult{}, fmt.Errorf("signer unavailable")
	   }
	   failed := make([]Job, 0)
	   p.mu.Lock()
	  for _, j := range p.rows {
		  if j.State == StateFailed {
			   failed = append(failed, *j)
		  }
	  }
	  p.mu.Unlock()
	   out := BatchRetryResult{Tried: len(failed)}
	   for _, j := range failed {
		   got, err := p.RetrySign(j.ID, actor, j.Fence.Epoch, j.Fence.Token, signer)
		  if err != nil {
			   out.Failed++
			  if cur, gerr := p.Get(j.ID); gerr == nil {
				   out.Jobs = append(out.Jobs,
					  *cur)
			   }
			   continue
		  }
		   out.Signed++
		  out.Jobs = append(out.Jobs, *got)
	  }
	   return out, nil
  }
