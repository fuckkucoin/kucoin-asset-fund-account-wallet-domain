package custody

import (
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/signpipe"
)

var (
	bookOnce sync.Once
	bookProc *CeremonyBook
)

// ProcessCeremony 返回进程内相位簿。
  func ProcessCeremony() *CeremonyBook {
	   bookOnce.Do(func() { bookProc = NewCeremonyBook() })
	  return bookProc
   }

// UseCeremony 让托管服务使用指定相位簿。

  func (s *Service) UseCeremony(b *CeremonyBook) {
	  if s != nil && b != nil {
		  s.ceremony = b
	  }
  }

// Annotate 改写仪式备注。空备注拒绝，终态之外的相位都可以记。
   func (b *CeremonyBook) Annotate(id, note string) error {
	   if b == nil {
		   return ErrCeremonyNotFound
	  }
	   note = strings.TrimSpace(note)
	   if note == "" {
		   return ErrIllegalCeremony
	  }
	  if len(note) > 240 {
		   note = note[:240]
	   }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	  c,
		   ok := b.rows[strings.TrimSpace(id)]
	   if !ok {
		   return ErrCeremonyNotFound
	  }
	   c.Note = note
	   c.UpdatedAt = time.Now().UTC()
	   return nil
  }

// admitCeremonySign 在 ECDSA 之前核对冷签批次绑定。
  func (s *Service) admitCeremonySign(movementID, chain, to, amount string) error {
	  if s == nil {
		  return nil
	   }
	  return signpipe.AllowCold(signpipe.ColdRef{
		   MovementID: movementID, Chain: chain, To: to, Amount: amount,
	  })
   }
