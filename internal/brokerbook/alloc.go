package brokerbook

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/google/uuid"
)

// Alloc 主↔子划拨。前后主+子合计必须 Conserve。

  func (s *Service) Alloc(req AllocReq) (Alloc, error) {
	  if s == nil {
		   return Alloc{}, ErrUnavailable
	  }
	  actor, err := validate.Actor(req.Actor)
	   if err != nil {
		   return Alloc{}, ErrActor
	   }
	   as, err := validate.Asset(req.Asset)
	   if err != nil {
		  return Alloc{}, ErrAsset
	  }
	  amt, err := money.Parse(req.Amount)
	   if err != nil {
		   return Alloc{}, ErrAmount
	   }
	  idem, err := validate.Idem(req.IdempotencyKey)
	   if err != nil {
		   return Alloc{}, ErrIdem
	   }
	   dir := strings.ToLower(strings.TrimSpace(req.Dir))
	   if dir != "to_sub" && dir != "to_master" {
		   return Alloc{}, ErrState
	  }
	  master, err := s.GetMaster(req.MasterID)
	   if err != nil {
		  return Alloc{}, err
	   }
	   sub, err := s.GetSub(req.SubID)
	  if err != nil {
		  return Alloc{}, err
	   }
	  if sub.MasterID != master.ID {
		   return Alloc{}, ErrSub
	  }
	   s.mu.Lock()
	  if id, ok := s.byIdem[idem]; ok {
		   cur := s.allocs[id]
		  s.mu.Unlock()
		  if cur.Amount != amt.String() || cur.Dir != dir || cur.SubID != sub.ID {
			  return Alloc{}, ErrIdem
		   }
		  return cur.Copy(), nil
	  }
	  live, ok := s.subs[sub.ID]
	  if !ok {
		   s.mu.Unlock()
		  return Alloc{}, ErrSub
	  }
	  if live.Fenced {
		  // 若已注入 FenceGuard，尝试通过世代+令牌校验；通过则允许有权限的运维写路径接着。
		  if err := s.fenceGuardCheck(sub.ID, req.FenceGeneration, req.FenceToken); err != nil {
			  s.mu.Unlock()
			   return Alloc{}, ErrFence
		  }
		  // FenceGuard 未注入或校验通过：仍需检查是否携带了有效令牌，否则拒绝。
		   if s.fenceGuard == nil {
			   s.mu.Unlock()
			   return Alloc{}, ErrFence
		  }
	   }
	  if dir == "to_master" && !hasPrivLocked(live.Copy(), PrivTransfer) {
		  s.mu.Unlock()
		  return Alloc{}, ErrPriv
	  }
	   beforeM, err := s.availLocked(master.ID, as)
	   if err != nil {
		  s.mu.Unlock()
		   return Alloc{}, err
	  }
	  beforeS, err := s.availLocked(sub.ID, as)
	  if err != nil {
		   s.mu.Unlock()
		   return Alloc{}, err
	  }
	   before, err := beforeM.Add(beforeS)
	  if err != nil {
		  s.mu.Unlock()
		   return Alloc{}, err
	  }
	   if dir == "to_sub" {
		  if err := s.subLocked(master.ID, as, amt); err != nil {
			  s.mu.Unlock()
			  return Alloc{}, err
		  }
		  if err := s.addLocked(sub.ID, as, amt); err != nil {
			   _ = s.addLocked(master.ID, as, amt)
			  s.mu.Unlock()
			  return Alloc{}, err
		  }
	  } else {
		  if err := s.subLocked(sub.ID, as, amt); err != nil {
			  s.mu.Unlock()
			  return Alloc{}, err
		   }
		  if err := s.addLocked(master.ID, as, amt); err != nil {
			  _ = s.addLocked(sub.ID, as, amt)
			   s.mu.Unlock()
			   return Alloc{}, err
		  }
	  }
	   afterM, err := s.availLocked(master.ID, as)
	  if err != nil {
		  s.mu.Unlock()
		   return Alloc{}, err
	   }
	  afterS, err := s.availLocked(sub.ID, as)
	  if err != nil {
		   s.mu.Unlock()
		   return Alloc{}, err
	  }
	   after, err := afterM.Add(afterS)
	   if err != nil {
		  s.mu.Unlock()
		   return Alloc{}, err
	  }
	   if err := money.Conserve(before, after); err != nil {
		  s.mu.Unlock()
		  return Alloc{}, ErrConserve
	   }
	   row := &Alloc{
		   ID: uuid.NewString(), IdempotencyKey: idem, MasterID: master.ID, SubID: sub.ID,
		  Asset: as, Amount: amt.String(), Dir: dir, Actor: actor, At: s.now(),
	   }
	  s.allocs[row.ID] = row
	  s.byIdem[idem] = row.ID
	  s.mu.Unlock()
	   return row.Copy(), nil
   }

  func hasPrivLocked(sub Sub, want Priv) bool {
	  for _, p := range sub.Privileges {
		  if p == want {
			  return true
		  }
	  }
	  return false
   }
