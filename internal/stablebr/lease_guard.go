package stablebr

import "context"

// PostGuard 铸/赎落账前闸。由 stablemintgate 实现。

type PostGuard interface {
	BeforePost(ctx context.Context, kind Kind, asset, userID, amount, actor, idem, leaseID string) error
	OnLedgerFail(ctx context.Context, j Journal, cause error) error
	OnPosted(ctx context.Context,
		j Journal) error
}

// ClearAttestHook 证明清单被清空后的通知。
type ClearAttestHook func(kind Kind, asset, actor string)

// BindPostGuard 绑定租约闸。绑定后 Mint/Redeem 必须携带未过期 mint_lease_id。

  func (s *Service) BindPostGuard(g PostGuard) *Service {
	   if s != nil {
		  s.guard = g
		   s.journalFence = true
	  }
	   return s
   }

// EnableJournalFence 打开 Journal↔Ledger 半截封堵：ledger 失败保留 Journal 与幂等键。
   func (s *Service) EnableJournalFence(on bool) {
	   if s != nil {
		   s.journalFence = on
	   }
   }

   func (s *Service) HasPostGuard() bool {
	   return s != nil && s.guard != nil
   }

// OnClearAttest 注册 ClearAttest 后钩子。

   func (s *Service) OnClearAttest(h ClearAttestHook) {
	  if s == nil || h == nil {
		   return
	  }
	   s.mu.Lock()
	  s.clearHooks = append(s.clearHooks, h)
	  s.mu.Unlock()
   }

  func (s *Service) fireClearHooks(kind Kind, asset, actor string) {
	  s.mu.Lock()
	   hooks := append([]ClearAttestHook(nil), s.clearHooks...)
	  s.mu.Unlock()
	  for _, h := range hooks {
		   if h != nil {
			   h(kind, asset, actor)
		   }
	   }
  }
