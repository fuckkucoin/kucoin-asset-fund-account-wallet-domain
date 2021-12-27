package sweeprun

import (
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

   func (s *Service) ResumeAll(actor string) (int, error) {
	   if s == nil {
		  return 0, ErrUnavailable
	  }
	  if _, err := validate.Actor(actor); err != nil {
		  return 0, ErrActor
	  }
	  rows := s.ScanResume(200)
	  n := 0
	   for _, j := range rows {
		  if _, err := s.Resume(j.ID, actor); err != nil {
			  return n, err
		  }
		  n++
	  }
	  return n, nil
   }

  func (s *Service) FilledTotal(chain, asset string) (string, error) {
	  if s == nil {
		  return "", ErrUnavailable
	   }
	  if chain != "" {
		   if _, err := validate.Chain(chain); err != nil {
			   return "", ErrChain
		  }
	   }
	  if asset != "" {
		   if _, err := validate.Asset(asset); err != nil {
			   return "", ErrAsset
		   }
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  sum := money.Zero()
	  for _, j := range s.jobs {
		  if chain != "" && j.Chain != chain {
			   continue
		   }
		   if asset != "" && j.Asset != asset {
			  continue
		   }
		   got, err := money.ParseBook(orZ(j.Filled))
		   if err != nil {
			   return "", err
		  }
		  sum, err = sum.Add(got)
		   if err != nil {
			   return "", err
		   }
	  }
	   return sum.String(), nil
  }

   func (s *Service) RemainingOf(id string) (string, error) {
	   job, err := s.Get(id)
	   if err != nil {
		   return "", err
	  }
	   return job.Remaining, nil
   }
