package brokerbook

import "account-assets.kucoin.net/funding/internal/brokerfence"

// FenceGuard 围栏世代校验接口（由 brokerfence.Service 满足）。
// 仅暴露 brokerbook 需要的方法，避免循环依赖。

type FenceGuard interface {
	// AllocCheck 划拨前世代 + 令牌校验。失败 fail-closed。
	AllocCheck(req brokerfence.AllocCheckReq) (brokerfence.FenceGeneration, error)
	// CreditMasterGuard CreditMaster 旁路挂截。
	// 若子账未围栏返回 nil,nil；已围栏则建 BypassCase 并返回 ErrMutexDenied。
	CreditMasterGuard(subID, masterID, asset, amount string) (*brokerfence.BypassCase, error)
}

// SetFenceGuard 注入围栏世代校验器（启动时由 cmd 层调用一次）。

   func (s *Service) SetFenceGuard(g FenceGuard) {
	  if s == nil {
		  return
	  }
	   s.mu.Lock()
	   s.fenceGuard = g
	   s.mu.Unlock()
  }

// fenceGuardCheck 若已注入 FenceGuard，执行世代+令牌校验；未注入则跳过。
// 仅校验，不改状态。

   func (s *Service) fenceGuardCheck(subID string, generation int64, token string) error {
	   s.mu.Lock()
	   g := s.fenceGuard
	   s.mu.Unlock()
	  if g == nil {
		  return nil
	   }
	   _, err := g.AllocCheck(brokerfence.AllocCheckReq{
		   SubID: subID,
		   Generation: generation,
		  Token:      token,
	  })
	  return err
  }

// creditMasterBypassCheck 若已注入 FenceGuard，执行 CreditMaster 旁路挂截。

   func (s *Service) creditMasterBypassCheck(masterID, asset, amount string) error {
	  s.mu.Lock()
	   g := s.fenceGuard
	  s.mu.Unlock()
	   if g == nil {
		  return nil
	  }
	   // 遍历 master 下所有围栏态子账，发现即产出 BypassCase。
	  subs := s.ListSubs(masterID)
	  for _, sub := range subs {
		  if !sub.Fenced {
			   continue
		  }
		   _, err := g.CreditMasterGuard(sub.ID, masterID, asset, amount)
		  if err != nil && err != brokerfence.ErrMutexDenied {
			   return err
		  }
		  if err == brokerfence.ErrMutexDenied {
			  // 有围栏子账，挡截成功——挂账并返回错误。
			   return ErrFence
		   }
	  }
	   return nil
  }
