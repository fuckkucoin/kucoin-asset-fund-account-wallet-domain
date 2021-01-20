package deposit

import (
	"strings"
	"sync"
	"time"
)

// SlotState 输出槽状态。credited 与 reversed 都拒绝另一笔贷记。
type SlotState string

const (
	SlotOpen     SlotState = "open"
	SlotCredited SlotState = "credited"
	SlotReversed SlotState = "reversed"
	SlotLocked   SlotState = "locked"
)

// OutputSlot 自然键 (chain, tx_hash, output_index)。
type OutputSlot struct {
	Chain  string `json:"chain"`
	TxHash string `json:"tx_hash"`
	OutputIndex int `json:"output_index"`
	UserID string `json:"user_id"`
	Asset      string `json:"asset"`
	Amount     string `json:"amount"`
	TransferID string `json:"transfer_id"`
	Idem       string `json:"idempotency_key"`
	State SlotState `json:"state"`
	Reason string    `json:"reason,omitempty"`
	At     time.Time `json:"at"`
}

// Slots 进程内一次贷记闸。不改账本 Apply。
type Slots struct {
	mu   sync.Mutex
	rows map[string]*OutputSlot
}

  func NewSlots() *Slots {
	  return &Slots{rows: map[string]*OutputSlot{}}
   }

  func slotKey(chain, hash string, output int) string {
	   return CanonicalChain(chain) + "|" + strings.TrimSpace(hash) + "|" + itoa(output)
   }

   func itoa(n int) string {
	  if n == 0 {
		   return "0"
	  }
	  neg := n < 0
	  if neg {
		   n = -n
	  }
	   var b [16]byte
	   i := len(b)
	  for n > 0 {
		  i--
		   b[i] = byte('0' + n%10)
		   n /= 10
	  }
	  if neg {
		  i--
		  b[i] = '-'
	   }
	   return string(b[i:])
  }

// Allow 放行第一笔，或同一幂等键的回放。
// 槽已存在且幂等键对不上时一律拒绝，包括已冲正后用新键再入一次。
  func (s *Slots) Allow(chain, hash, idem string, output int) error {
	   if s == nil {
		   return ErrSecondCredit
	  }
	   k := slotKey(chain, hash, output)
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  cur, ok := s.rows[k]
	  if !ok {
		   return nil
	  }
	   idem = strings.TrimSpace(idem)
	  if idem != "" && cur.Idem == idem && cur.State == SlotCredited {
		   return nil
	   }
	   return ErrSecondCredit
   }

// Commit 记下第一笔成功贷记。同一幂等回放不改槽。

   func (s *Slots) Commit(slot OutputSlot) error {
	  if s == nil {
		  return ErrSecondCredit
	   }
	   slot.Chain = CanonicalChain(slot.Chain)
	  slot.TxHash = strings.TrimSpace(slot.TxHash)
	  slot.Idem = strings.TrimSpace(slot.Idem)
	   slot.TransferID = strings.TrimSpace(slot.TransferID)
	   if slot.Chain == "" || slot.TxHash == "" {
		  return ErrPatrolInput
	  }
	  if slot.State == "" {
		   slot.State = SlotCredited
	  }
	   if slot.At.IsZero() {
		  slot.At = time.Now().UTC()
	   }
	  k := slotKey(slot.Chain, slot.TxHash, slot.OutputIndex)
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   if cur, ok := s.rows[k]; ok {
		  if slot.Idem != "" && cur.Idem == slot.Idem {
			   return nil
		   }
		   if cur.TransferID != "" && cur.TransferID == slot.TransferID && cur.State == slot.State {
			   return nil
		   }
		   return ErrSecondCredit
	  }
	   cp := slot
	   s.rows[k] = &cp
	  return nil
   }

// Lock 差额或未贷记确认后锁死该输出，后续贷记失败。
  func (s *Slots) Lock(chain, hash, reason string, output int) error {
	  if s == nil {
		   return ErrSecondCredit
	  }
	   k := slotKey(chain, hash, output)
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   if cur, ok := s.rows[k]; ok {
		   if cur.State == SlotReversed || cur.State == SlotLocked || cur.State == SlotCredited {
			   if reason != "" && cur.Reason == "" {
				   cur.Reason = reason
			   }
			   if cur.State == SlotCredited {
				  cur.State = SlotLocked
				   cur.Reason = reason
			   }
			  return nil
		  }
	  }
	   s.rows[k] = &OutputSlot{
		  Chain: CanonicalChain(chain), TxHash: strings.TrimSpace(hash), OutputIndex: output,
		  State: SlotLocked, Reason: reason, At: time.Now().UTC(),
	   }
	   return nil
   }

// MarkReversed 冲正后保持拒绝第二笔贷记。

   func (s *Slots) MarkReversed(chain, hash, reason string, output int) {
	  if s == nil {
		  return
	   }
	  k := slotKey(chain, hash, output)
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  cur, ok := s.rows[k]
	   if !ok {
		   cur = &OutputSlot{Chain: CanonicalChain(chain), TxHash: strings.TrimSpace(hash), OutputIndex: output}
		   s.rows[k] = cur
	   }
	   cur.State = SlotReversed
	   if reason != "" {
		  cur.Reason = reason
	  }
	   cur.At = time.Now().UTC()
  }

// Get 读取输出槽。
  func (s *Slots) Get(chain, hash string, output int) (OutputSlot, bool) {
	   if s == nil {
		   return OutputSlot{}, false
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   cur, ok := s.rows[slotKey(chain, hash, output)]
	  if !ok {
		  return OutputSlot{}, false
	   }
	  return *cur, true
  }

// BlockedConfirm 确认写路径：已冲正或已锁定不得再确认入账。
  func (s *Slots) BlockedConfirm(chain, hash string, output int) error {
	  cur,
		  ok := s.Get(chain, hash, output)
	  if !ok {
		  return nil
	  }
	   if cur.State == SlotReversed || cur.State == SlotLocked {
		  return ErrSecondCredit
	  }
	  return nil
  }
