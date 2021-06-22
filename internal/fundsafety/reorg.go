package fundsafety

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/deposit"
	"account-assets.kucoin.net/funding/internal/validate"
)

// DepthNeed 按链给出入账确认窗。不足不得 Observe。
   func DepthNeed(chain string) int {
	  switch strings.ToUpper(strings.TrimSpace(chain)) {
	  case "BTC":
		  return 3
	   case "ETH", "BASE", "ARB", "OP", "MATIC":
		  return 12
	  case "TRON":
		  return 20
	  case "SOL":
		  return 32
	  default:
		  return 12
	  }
  }

   func confirmKey(chain, hash string) string {
	   return hashKey(chain, hash)
  }

// OpenConfirm 登记确认窗。have < need 时不得入账。
  func (s *Service) OpenConfirm(chain, hash string, have int) (ConfirmWin, error) {
	   if s == nil {
		  return ConfirmWin{}, ErrUnavailable
	   }
	   if err := s.GuardBoardLane(context.Background(), "deposit", "", "", ""); err != nil {
		  return ConfirmWin{}, err
	   }
	  ch, err := validate.Chain(chain)
	  if err != nil {
		  return ConfirmWin{}, ErrHash
	  }
	   h, err := validate.TxHash(hash)
	   if err != nil {
		  return ConfirmWin{}, ErrHash
	  }
	  need := DepthNeed(ch)
	  if have < 0 {
		  return ConfirmWin{}, ErrDepth
	  }
	  row := ConfirmWin{Chain: ch, TxHash: h, Need: need, Have: have, At: s.now()}
	  s.mu.Lock()
	   s.confirms[confirmKey(ch, h)] = row
	  s.mu.Unlock()
	  if have < need {
		  return row, ErrConfirm
	  }
	   return row, nil
   }

// AdvanceConfirm 抬确认数。回退低于已记 have 视为重组。

  func (s *Service) AdvanceConfirm(chain, hash string, have int) (ConfirmWin, error) {
	  if s == nil {
		   return ConfirmWin{}, ErrUnavailable
	   }
	  if err := s.GuardBoardLane(context.Background(), "deposit", "", "", ""); err != nil {
		  return ConfirmWin{}, err
	   }
	  ch, err := validate.Chain(chain)
	  if err != nil {
		  return ConfirmWin{}, ErrHash
	  }
	   h, err := validate.TxHash(hash)
	   if err != nil {
		   return ConfirmWin{}, ErrHash
	  }
	  k := confirmKey(ch, h)
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  cur, ok := s.confirms[k]
	  if !ok {
		  cur = ConfirmWin{Chain: ch, TxHash: h, Need: DepthNeed(ch), At: s.now()}
	   }
	  if have < cur.Have {
		  cur.Reorged = true
		   cur.Have = have
		  cur.At = s.now()
		   s.confirms[k] = cur
		  return cur, ErrReorg
	   }
	   cur.Have = have
	   cur.At = s.now()
	  s.confirms[k] = cur
	  if cur.Have < cur.Need {
		   return cur, ErrConfirm
	  }
	  return cur, nil
  }

// ReadyToCredit 确认窗够深且未重组才放过 Observe。

   func (s *Service) ReadyToCredit(chain, hash string) error {
	   if s == nil {
		  return ErrUnavailable
	   }
	  if s.Halted() {
		   return ErrHalt
	   }
	  k := confirmKey(chain, hash)
	  s.mu.Lock()
	   cur, ok := s.confirms[k]
	  s.mu.Unlock()
	  if !ok {
		   return ErrConfirm
	  }
	   if cur.Reorged {
		  return ErrReorg
	   }
	  if cur.Have < cur.Need {
		  return ErrConfirm
	   }
	  return nil
  }

// ObserveAfterConfirm 先过确认窗再认领哈希。窗不够或重组失败即关。

   func (s *Service) ObserveAfterConfirm(ctx context.Context, have int, req deposit.ObserveReq) (*deposit.ObserveReq, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	   if s.Halted() {
		  return nil, ErrHalt
	  }
	   win, err := s.OpenConfirm(req.Chain, req.TxHash, have)
	  if err != nil {
		   return nil, err
	  }
	   if win.Reorged {
		  return nil, ErrReorg
	   }
	  return &req, nil
   }

// ReorgDropBelowNeed 确认回落后清认领，避免短重组再入账。
  func (s *Service) ReorgDropBelowNeed(chain, hash string, have int) error {
	   win, err := s.AdvanceConfirm(chain, hash, have)
	  if err != nil && err != ErrReorg && err != ErrConfirm {
		   return err
	  }
	  if !win.Reorged && win.Have >= win.Need {
		  return nil
	  }
	   k := confirmKey(chain, hash)
	  s.mu.Lock()
	  delete(s.seen, k)
	  delete(s.fences, k)
	   s.mu.Unlock()
	   if win.Reorged {
		   return ErrReorg
	   }
	  return ErrConfirm
   }

  func (s *Service) Confirm(chain, hash string) (ConfirmWin, bool) {
	   if s == nil {
		  return ConfirmWin{}, false
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  cur, ok := s.confirms[confirmKey(chain, hash)]
	   return cur, ok
  }
