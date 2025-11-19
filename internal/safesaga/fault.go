package safesaga

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/signpipe"
	"account-assets.kucoin.net/funding/internal/validate"
)

type DualFault struct {
	JobID string `json:"job_id"`
	State string `json:"state"`
	Note string `json:"note,omitempty"`
	Retried bool `json:"retried,omitempty"`
}

// DualControlUnderFault 申请/复核/签必须三人。signer 失败后旧 fence 不得成功；换新 fence 再签。

   func (s *Service) DualControlUnderFault(id, reqActor, apprActor, signActor string, failN int, ok signpipe.Signer) (DualFault, error) {
	   if s == nil || s.pipe == nil {
		  return DualFault{}, ErrUnavailable
	  }
	  if err := s.requireWrite(); err != nil {
		   return DualFault{}, err
	  }
	  if _, err := validate.Actor(reqActor); err != nil {
		   return DualFault{}, ErrActor
	  }
	  if _, err := validate.Actor(apprActor); err != nil {
		  return DualFault{}, ErrActor
	  }
	   if _, err := validate.Actor(signActor); err != nil {
		   return DualFault{}, ErrActor
	  }
	  if strings.EqualFold(reqActor, apprActor) || strings.EqualFold(reqActor, signActor) || strings.EqualFold(apprActor, signActor) {
		  return DualFault{}, ErrActor
	   }
	  s.mu.Lock()
	   r := s.runs[strings.TrimSpace(id)]
	  s.mu.Unlock()
	  if r == nil {
		   return DualFault{}, ErrUnavailable
	   }
	   job, err := s.pipe.Open(reqActor, r.Idem+"-dc", "ETH", "0x"+strings.Repeat("ab", 20), r.Amount)
	  if err != nil {
		  return DualFault{}, err
	  }
	  job, err = s.pipe.Approve(job.ID, apprActor, job.Fence.Epoch, job.Fence.Token, job.MovementID, job.Chain, job.To, job.Amount)
	   if err != nil {
		   return DualFault{}, err
	  }
	  staleEpoch, staleTok := job.Fence.Epoch, job.Fence.Token
	  n := 0
	   fail := func(string) (string, error) {
		  n++
		   if failN <= 0 || n <= failN {
			   return "", ErrSign
		   }
		   if ok != nil {
			   return ok("")
		   }
		   return "sig", nil
	   }
	   if _, err := s.pipe.Sign(job.ID, signActor, job.Fence.Epoch, job.Fence.Token, fail); err == nil && failN > 0 {
		  return DualFault{}, ErrSign
	  }
	   failed, err := s.pipe.Get(job.ID)
	  if err != nil {
		  return DualFault{}, err
	   }
	  if _, err := s.pipe.Sign(failed.ID, signActor, staleEpoch, staleTok, ok); err == nil {
		  return DualFault{JobID: failed.ID, State: string(failed.State), Note: "stale fence accepted"}, ErrFence
	  }
	  if ok == nil {
		   ok = func(string) (string, error) { return "sig", nil }
	   }
	   got, err := s.pipe.RetrySign(failed.ID, signActor, failed.Fence.Epoch, failed.Fence.Token, ok)
	   if err != nil {
		   return DualFault{JobID: failed.ID,
			  State: string(failed.State), Note: err.Error()}, err
	  }
	   return DualFault{JobID: got.ID, State: string(got.State), Retried: true}, nil
  }

  func (s *Service) FenceOf(id string) (persist.Fence, error) {
	  if s == nil {
		  return persist.Fence{}, ErrUnavailable
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   r := s.runs[strings.TrimSpace(id)]
	  if r == nil {
		  return persist.Fence{}, ErrUnavailable
	   }
	   return persist.Fence{Epoch: r.Epoch, Token: r.Token}, nil
   }
