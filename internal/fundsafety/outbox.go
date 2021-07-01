package fundsafety

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

  func creditKey(chain, hash, user, asset string) string {
	   return hashKey(chain, hash) + "|" + strings.TrimSpace(user) + "|" + strings.ToUpper(strings.TrimSpace(asset))
   }

// EnqueueCredit 同一 (chain,hash,user,asset) 只落一笔出箱。金额必须过 Parse。
   func (s *Service) EnqueueCredit(chain, hash, user, asset, amount string) (CreditOutbox, error) {
	   if s == nil {
		  return CreditOutbox{}, ErrUnavailable
	  }
	   ch, err := validate.Chain(chain)
	  if err != nil {
		  return CreditOutbox{}, ErrHash
	  }
	  h, err := validate.TxHash(hash)
	  if err != nil {
		  return CreditOutbox{}, ErrHash
	  }
	   uid, err := validate.UserID(user)
	   if err != nil {
		  return CreditOutbox{}, ErrUser
	  }
	  as, err := validate.Asset(asset)
	   if err != nil {
		   return CreditOutbox{}, ErrAsset
	   }
	   if err := s.GuardBoardLane(context.Background(), "deposit", uid, ch, as); err != nil {
		  return CreditOutbox{}, err
	   }
	   if err := rejectSci(amount); err != nil {
		   return CreditOutbox{}, err
	   }
	  amt, err := money.Parse(amount)
	  if err != nil {
		  return CreditOutbox{}, ErrAmount
	   }
	  if !amt.IsPositive() {
		   return CreditOutbox{}, ErrNegative
	   }
	  k := creditKey(ch, h, uid, as)
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  if cur, ok := s.credits[k]; ok {
		   if cur.Amount == amt.String() {
			  return cur, nil
		   }
		   return CreditOutbox{}, ErrOutbox
	  }
	   row := CreditOutbox{
		   Key: k, Chain: ch, TxHash: h, UserID: uid, Asset: as, Amount: amt.String(),
		   Epoch: 1, Token: persist.NewToken(),
	   }
	   s.credits[k] = row
	   return row, nil
   }

// DeliverCredit 按围栏投递。已投递回放；旧 token 拒。

   func (s *Service) DeliverCredit(key string, epoch int64, token string) (CreditOutbox, error) {
	   if s == nil {
		  return CreditOutbox{}, ErrUnavailable
	  }
	  if err := s.GuardBoardLane(context.Background(), "sweep", "", "", ""); err != nil {
		  return CreditOutbox{}, err
	   }
	   key = strings.TrimSpace(key)
	  if key == "" {
		  return CreditOutbox{}, ErrOutbox
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  cur, ok := s.credits[key]
	   if !ok {
		  return CreditOutbox{}, ErrOutbox
	  }
	   if cur.SentAt != nil {
		  return cur, nil
	   }
	  if cur.Epoch != epoch || cur.Token == "" || cur.Token != token {
		   return CreditOutbox{}, ErrFence
	  }
	  now := s.now()
	  cur.SentAt = &now
	   cur.Epoch++
	   cur.Token = persist.NewToken()
	  s.credits[key] = cur
	   return cur, nil
  }

  func (s *Service) Credit(key string) (CreditOutbox, error) {
	  if s == nil {
		  return CreditOutbox{}, ErrUnavailable
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   cur, ok := s.credits[strings.TrimSpace(key)]
	   if !ok {
		  return CreditOutbox{}, ErrOutbox
	  }
	  return cur, nil
  }

   func (s *Service) PendingCredits() []CreditOutbox {
	   if s == nil {
		  return nil
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   out := make([]CreditOutbox, 0, len(s.credits))
	   for _, c := range s.credits {
		  if c.SentAt == nil {
			  out = append(out, c)
		  }
	  }
	  return out
   }
