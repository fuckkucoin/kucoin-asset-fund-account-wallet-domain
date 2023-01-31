package sweeprun

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
)

  func (s *Service) Pause(id, actor string) (Job, error) {
	   if s == nil {
		   return Job{}, ErrUnavailable
	   }
	   if _, err := validate.Actor(actor); err != nil {
		   return Job{}, ErrActor
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  cur, ok := s.jobs[strings.TrimSpace(id)]
	  if !ok {
		   return Job{}, ErrJob
	  }
	  if cur.State != StateRunning && cur.State != StatePartial {
		   return cur.Copy(), ErrState
	  }
	  cur.State = StatePaused
	   cur.Actor = actor
	   cur.UpdatedAt = s.now()
	   return cur.Copy(), nil
  }

// Resume 暂停或部分成交后接着。不改金额，只把门打开给下一笔 Fill。
  func (s *Service) Resume(id, actor string) (Job, error) {
	  if s == nil {
		  return Job{}, ErrUnavailable
	  }
	  if _, err := validate.Actor(actor); err != nil {
		   return Job{}, ErrActor
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  cur,
		  ok := s.jobs[strings.TrimSpace(id)]
	  if !ok {
		  return Job{}, ErrJob
	  }
	  if !cur.State.CanResume() {
		   return cur.Copy(), ErrState
	   }
	  cur.State = StateRunning
	   cur.Actor = actor
	   cur.UpdatedAt = s.now()
	  return cur.Copy(), nil
  }

  func (s *Service) Cancel(id, actor string) (Job, error) {
	  if s == nil {
		  return Job{}, ErrUnavailable
	   }
	   if _, err := validate.Actor(actor); err != nil {
		   return Job{}, ErrActor
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   cur, ok := s.jobs[strings.TrimSpace(id)]
	   if !ok {
		   return Job{}, ErrJob
	  }
	   if cur.State.Terminal() || cur.State == StateRunning {
		   return cur.Copy(), ErrState
	  }
	  cur.State = StateCancelled
	   cur.Actor = actor
	  cur.UpdatedAt = s.now()
	  return cur.Copy(), nil
   }
