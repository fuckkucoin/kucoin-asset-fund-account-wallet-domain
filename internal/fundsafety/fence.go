package fundsafety

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
)

   func (s *Service) fenceOfLocked(key string) persist.Fence {
	   if s.fences == nil {
		   return persist.Fence{}
	   }
	   return s.fences[key]
   }

// CheckClaimFence 写认领行前核对 epoch/token。旧 token 不得改行。
   func (s *Service) CheckClaimFence(chain, hash string, epoch int64, token string) error {
	   if s == nil {
		   return ErrUnavailable
	   }
	   if s.Halted() {
		  return ErrHalt
	  }
	  ch, err := validate.Chain(chain)
	   if err != nil {
		  return ErrHash
	  }
	  h, err := validate.TxHash(hash)
	   if err != nil {
		   return ErrHash
	  }
	   k := hashKey(ch, h)
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  f := s.fenceOfLocked(k)
	   if f.Epoch == 0 || f.Token == "" {
		   return ErrFence
	  }
	  if !f.Match(epoch, token) {
		   return ErrFence
	   }
	  return nil
   }

// BumpClaimFence 换 token 并 +1 epoch。必须在业务锁内、写成功之后。

  func (s *Service) BumpClaimFence(chain, hash string, epoch int64, token string) (persist.Fence, error) {
	   if s == nil {
		   return persist.Fence{}, ErrUnavailable
	  }
	   if s.Halted() {
		  return persist.Fence{}, ErrHalt
	  }
	  ch, err := validate.Chain(chain)
	   if err != nil {
		  return persist.Fence{}, ErrHash
	  }
	   h, err := validate.TxHash(hash)
	   if err != nil {
		   return persist.Fence{}, ErrHash
	   }
	  k := hashKey(ch, h)
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   f := s.fenceOfLocked(k)
	  if !f.Match(epoch, token) {
		   return persist.Fence{}, ErrFence
	  }
	  cur, ok := s.seen[k]
	   if !ok {
		   return persist.Fence{}, ErrHash
	   }
	   next := persist.Fence{Epoch: f.Epoch + 1,
		   Token: persist.NewToken()}
	  s.fences[k] = next
	  cur.Epoch = next.Epoch
	  cur.Token = next.Token
	  s.seen[k] = cur
	   return next, nil
   }

// BindLedgerID 给已认领哈希挂账本号。必须持当前围栏。
  func (s *Service) BindLedgerID(chain, hash, ledgerID string, epoch int64, token string) (CreditClaim, error) {
	   if s == nil {
		   return CreditClaim{}, ErrUnavailable
	  }
	  if s.Halted() {
		  return CreditClaim{}, ErrHalt
	  }
	   ch, err := validate.Chain(chain)
	  if err != nil {
		  return CreditClaim{}, ErrHash
	   }
	   h, err := validate.TxHash(hash)
	   if err != nil {
		   return CreditClaim{}, ErrHash
	  }
	  id := strings.TrimSpace(ledgerID)
	  if id == "" {
		   return CreditClaim{}, ErrHash
	   }
	   k := hashKey(ch, h)
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  cur, ok := s.seen[k]
	  if !ok {
		  return CreditClaim{}, ErrHash
	   }
	   f := s.fenceOfLocked(k)
	  if !f.Match(epoch, token) {
		  return CreditClaim{}, ErrFence
	  }
	   if cur.Ledger != "" && cur.Ledger != id {
		  return CreditClaim{}, ErrDup
	  }
	  cur.Ledger = id
	   next := persist.Fence{Epoch: f.Epoch + 1, Token: persist.NewToken()}
	  s.fences[k] = next
	  cur.Epoch = next.Epoch
	  cur.Token = next.Token
	  s.seen[k] = cur
	   return cur, nil
   }

  func (s *Service) ClaimFence(chain, hash string) (persist.Fence, bool) {
	  if s == nil {
		   return persist.Fence{}, false
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  f, ok := s.fences[hashKey(chain, hash)]
	  return f, ok
   }
