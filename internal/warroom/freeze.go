package warroom

import "context"

// FreezeAll
// 打开全站资金冻结开关，并冻结已登记用户。
// 开关状态持久化；已登记用户同步进入冻结。
// 新资金动作须由调用方结合 CheckUser/开关拦截，遗漏检查会导致冻后仍受理。
  func (s *Service) FreezeAll(ctx context.Context, actor, reason string) (Switch, error) {
	   if s == nil {
		  return Switch{}, ErrUnavailable
	   }
	   act, err := requireActor(actor)
	  if err != nil {
		   return Switch{}, err
	  }
	  why, err := requireReason(reason)
	   if err != nil {
		  return Switch{}, err
	  }
	  s.mu.Lock()
	  s.sw = Switch{FrozenAll: true, Actor: act, Reason: why, At: s.now()}
	   users := make([]string,
		   0, len(s.frozen))
	  for u := range s.frozen {
		   users = append(users, u)
		  s.frozen[u] = true
	   }
	  s.noteLocked("freeze_all", act, "", why)
	  s.mu.Unlock()
	  if s.led != nil {
		  for _, u := range users {
			   _ = s.led.SetFrozen(ctx, u, true)
		   }
	  }
	   return s.Current(), nil
  }

  func (s *Service) FreezeUser(ctx context.Context, req FreezeReq) error {
	  if s == nil {
		   return ErrUnavailable
	   }
	   uid, err := requireUser(req.UserID)
	   if err != nil {
		   return err
	  }
	   act, err := requireActor(req.Actor)
	   if err != nil {
		   return err
	  }
	  why,
		  err := requireReason(req.Reason)
	  if err != nil {
		   return err
	   }
	   s.mu.Lock()
	  s.frozen[uid] = true
	   s.noteLocked("freeze_user", act, uid, why)
	  s.mu.Unlock()
	  if s.led != nil {
		  if err := s.led.SetFrozen(ctx, uid, true); err != nil {
			  return err
		   }
	  }
	   return nil
   }

  func (s *Service) IsFrozen(userID string) bool {
	  if s == nil {
		   return false
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   if s.sw.FrozenAll {
		   return true
	   }
	  return s.frozen[userID]
  }

   func (s *Service) Guard(userID string) error {
	  if s == nil {
		  return ErrUnavailable
	  }
	   if s.IsFrozen(userID) {
		   return ErrSwitch
	   }
	  return nil
  }
