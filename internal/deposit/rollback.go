package deposit

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Face 只读入金相位。确认中与已冲正必须能分开。

type Face struct {
	Chain       string `json:"chain"`
	TxHash      string `json:"tx_hash"`
	OutputIndex int    `json:"output_index"`
	Asset string `json:"asset"`
	UserID     string `json:"user_id"`
	TransferID string `json:"transfer_id,omitempty"`
	Phase      string `json:"phase"`
	PhaseLabel string `json:"phase_label"`
	Confirming bool   `json:"confirming"`
	Reversed   bool   `json:"reversed"`
	Amount     string `json:"amount,omitempty"`
	Explain    string `json:"explain,omitempty"`
	AvailBefore  string `json:"avail_before,omitempty"`
	AvailAfter   string `json:"avail_after,omitempty"`
	ReverseEntry string `json:"reverse_entry,omitempty"`
	Reason string `json:"reason,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// RollbackNote 一次提前入账或冲正的审计行。

type RollbackNote struct {
	Chain  string `json:"chain"`
	TxHash string `json:"tx_hash"`
	OutputIndex int `json:"output_index"`
	Asset        string    `json:"asset"`
	UserID       string    `json:"user_id"`
	TransferID   string    `json:"transfer_id"`
	Amount       string    `json:"amount"`
	AvailBefore  string    `json:"avail_before"`
	AvailAfter   string    `json:"avail_after"`
	ReverseEntry string    `json:"reverse_entry"`
	Phase        string    `json:"phase"`
	Reason       string    `json:"reason"`
	Explain      string    `json:"explain"`
	At           time.Time `json:"at"`
}

// RollBook 提前入账与冲正审计。
type RollBook struct {
	mu sync.Mutex
	rows map[string]*RollbackNote
	log  []RollbackNote
}

  func NewRollBook() *RollBook {
	  return &RollBook{rows: map[string]*RollbackNote{}}
   }

  func (b *RollBook) key(chain, hash string, output int) string {
	  return slotKey(chain, hash, output)
  }

// MarkEarly 贷记已发生但确认数未满。读面为确认中，不得当成最终成功。
   func (b *RollBook) MarkEarly(note RollbackNote) (Face, error) {
	   if b == nil {
		  return Face{}, ErrFaceMiss
	   }
	  note = cleanNote(note)
	   if note.Chain == "" || note.TxHash == "" {
		  return Face{}, ErrPatrolInput
	  }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  k := b.key(note.Chain, note.TxHash, note.OutputIndex)
	  if cur, ok := b.rows[k]; ok && cur.Phase == PhaseReversed {
		   return faceOf(cur),
			   ErrSecondCredit
	   }
	  if cur, ok := b.rows[k]; ok && cur.Phase == PhaseCredited && cur.TransferID != "" && note.TransferID != "" && cur.TransferID != note.TransferID {
		  return faceOf(cur),
			  ErrSecondCredit
	  }
	   note.Phase = PhaseConfirming
	   note.Explain = explainConfirming(note)
	  note.At = time.Now().UTC()
	  cp := note
	  b.rows[k] = &cp
	   b.log = append(b.log, cp)
	  return faceOf(&cp), nil
   }

// MarkFinal 确认数已满且未冲正。读面离开确认中。

   func (b *RollBook) MarkFinal(chain, hash, transferID string, output int) (Face, error) {
	  if b == nil {
		   return Face{}, ErrFaceMiss
	   }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	  k := b.key(chain, hash, output)
	   cur, ok := b.rows[k]
	  if !ok {
		   cur = &RollbackNote{Chain: CanonicalChain(chain), TxHash: strings.TrimSpace(hash), OutputIndex: output, TransferID: transferID}
		   b.rows[k] = cur
	   }
	  if cur.Phase == PhaseReversed {
		   return faceOf(cur),
			  ErrSecondCredit
	   }
	   cur.Phase = PhaseCredited
	   if transferID != "" {
		   cur.TransferID = transferID
	  }
	   cur.Explain = "确认数已满，入金已贷记，读面不再显示确认中。"
	   cur.At = time.Now().UTC()
	  b.log = append(b.log, *cur)
	  return faceOf(cur), nil
  }

// Rollback 记录冲正。可用余额前后值与冲正分录必须同时留下。
   func (b *RollBook) Rollback(note RollbackNote) (Face, error) {
	   if b == nil {
		  return Face{}, ErrFaceMiss
	   }
	  note = cleanNote(note)
	  if note.Chain == "" || note.TxHash == "" {
		   return Face{}, ErrPatrolInput
	   }
	  if note.ReverseEntry == "" || note.AvailBefore == "" || note.AvailAfter == "" {
		  return Face{}, ErrFaceForge
	  }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  k := b.key(note.Chain, note.TxHash, note.OutputIndex)
	   cur, ok := b.rows[k]
	   if ok && cur.Phase == PhaseReversed {
		  return faceOf(cur), ErrSecondCredit
	   }
	   if !ok {
		  cur = &RollbackNote{Chain: note.Chain, TxHash: note.TxHash, OutputIndex: note.OutputIndex}
		  b.rows[k] = cur
	   }
	  if note.AvailBefore == "" {
		   note.AvailBefore = cur.AvailBefore
	  }
	  cur.Asset = firstNonEmpty(note.Asset,
		  cur.Asset)
	   cur.UserID = firstNonEmpty(note.UserID, cur.UserID)
	  cur.TransferID = firstNonEmpty(note.TransferID, cur.TransferID)
	   cur.Amount = firstNonEmpty(note.Amount, cur.Amount)
	  cur.AvailBefore = note.AvailBefore
	  cur.AvailAfter = note.AvailAfter
	   cur.ReverseEntry = note.ReverseEntry
	  cur.Reason = firstNonEmpty(note.Reason, "reorg")
	   cur.Phase = PhaseReversed
	  cur.Explain = explainReversed(*cur)
	  cur.At = time.Now().UTC()
	  b.log = append(b.log, *cur)
	  return faceOf(cur), nil
   }

// Face 只读相位。没有记录时 ok=false。

   func (b *RollBook) Face(chain, hash string, output int) (Face, bool) {
	   if b == nil {
		   return Face{}, false
	   }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	  cur, ok := b.rows[b.key(chain, hash, output)]
	   if !ok {
		   return Face{}, false
	  }
	  return faceOf(cur), true
  }

// Audit 按输出返回审计行，新的在前。
   func (b *RollBook) Audit(chain, hash string, output int, limit int) []RollbackNote {
	  if b == nil {
		  return nil
	   }
	   if limit <= 0 {
		   limit = 20
	  }
	  want := b.key(chain, hash, output)
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  out := make([]RollbackNote,
		   0, limit)
	   for i := len(b.log) - 1; i >= 0 && len(out) < limit; i-- {
		   row := b.log[i]
		   if b.key(row.Chain, row.TxHash, row.OutputIndex) != want && hash != "" {
			   continue
		  }
		  out = append(out, row)
	   }
	  return out
   }

  func cleanNote(n RollbackNote) RollbackNote {
	  n.Chain = CanonicalChain(n.Chain)
	  n.TxHash = strings.TrimSpace(n.TxHash)
	  n.Asset = strings.ToUpper(strings.TrimSpace(n.Asset))
	   n.UserID = strings.TrimSpace(n.UserID)
	   n.TransferID = strings.TrimSpace(n.TransferID)
	   n.Amount = strings.TrimSpace(n.Amount)
	  n.AvailBefore = strings.TrimSpace(n.AvailBefore)
	   n.AvailAfter = strings.TrimSpace(n.AvailAfter)
	   n.ReverseEntry = strings.TrimSpace(n.ReverseEntry)
	  n.Reason = strings.TrimSpace(n.Reason)
	   return n
   }

  func faceOf(n *RollbackNote) Face {
	  if n == nil {
		   return Face{}
	  }
	   f := Face{
		   Chain:  n.Chain,
		  TxHash: n.TxHash, OutputIndex: n.OutputIndex,
		  Asset: n.Asset, UserID: n.UserID, TransferID: n.TransferID,
		  Phase: n.Phase, Amount: n.Amount, Explain: n.Explain,
		  AvailBefore: n.AvailBefore, AvailAfter: n.AvailAfter,
		  ReverseEntry: n.ReverseEntry, Reason: n.Reason,
	  }
	  if !n.At.IsZero() {
		  f.UpdatedAt = n.At.UTC().Format(time.RFC3339)
	   }
	   switch n.Phase {
	   case PhaseConfirming:
		   f.Confirming = true
	  case PhaseReversed:
		  f.Reversed = true
	   }
	  f.PhaseLabel = FaceLabel(n.Phase)
	  return f
   }

  func explainConfirming(n RollbackNote) string {
	  return fmt.Sprintf("提前入账 %s %s，可用余额暂记 %s，确认数未满，读面为确认中。输出 %s/%s#%d 在冲正完成前不得再次贷记。",
		  n.Amount, n.Asset, blankZero(n.AvailBefore), n.Chain, shortHash(n.TxHash), n.OutputIndex)
   }

  func explainReversed(n RollbackNote) string {
	   return fmt.Sprintf("可用余额由 %s 调整为 %s，冲正分录 %s，原因：%s。该输出不得再次贷记。",
		   blankZero(n.AvailBefore), blankZero(n.AvailAfter), n.ReverseEntry, blankReason(n.Reason))
  }

  func blankZero(s string) string {
	   if strings.TrimSpace(s) == "" {
		   return "0"
	   }
	   return s
   }

   func blankReason(s string) string {
	   if strings.TrimSpace(s) == "" {
		  return "确认回退"
	  }
	  return s
  }

   func shortHash(h string) string {
	   if len(h) <= 12 {
		   return h
	  }
	  return h[:6] + "…" + h[len(h)-4:]
  }

   func firstNonEmpty(a, b string) string {
	   if strings.TrimSpace(a) != "" {
		  return a
	   }
	   return b
  }
