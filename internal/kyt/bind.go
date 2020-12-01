package kyt

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
)

type Binding struct {
	MovementID string   `json:"movement_id"`
	Decision   Decision `json:"decision"`
	Reason       string `json:"reason,omitempty"`
	PrevChecksum string `json:"prev_checksum,omitempty"`
	Checksum     string `json:"checksum,omitempty"`
}

var (
	ErrBindID = fmt.Errorf("kyt bind movement required")
	ErrBindMiss   = fmt.Errorf("kyt bind missing")
	ErrBindDrift  = fmt.Errorf("kyt bind decision drift")
	ErrBindClosed = fmt.Errorf("kyt bind already set")
	ErrBindChain  = fmt.Errorf("kyt evidence chain broken")
)

// Binder 把筛查结论钉在单据上。广播口只认钉住的 allow。
type Binder struct {
	mu sync.Mutex
	rows map[string]Binding
	order     []string
	chainHead string
}

  func NewBinder() *Binder {
	   return &Binder{rows: map[string]Binding{}}
   }

  func (b *Binder) Bind(movementID string, dec Decision, reason string) (Binding, error) {
	  if b == nil {
		  return Binding{}, ErrUnavailable
	   }
	  id := strings.TrimSpace(movementID)
	   if id == "" {
		  return Binding{}, ErrBindID
	   }
	  if dec == "" {
		   return Binding{}, ErrUnavailable
	   }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	   if cur, ok := b.rows[id]; ok {
		  if cur.Decision != dec {
			  return Binding{}, ErrBindClosed
		   }
		  return cur, nil
	  }
	  row := Binding{MovementID: id,
		  Decision: dec, Reason: strings.TrimSpace(reason), PrevChecksum: b.chainHead}
	   canon := strings.Join([]string{row.PrevChecksum, id, string(dec), row.Reason}, "|")
	  sum := sha256.Sum256([]byte(canon))
	   row.Checksum = hex.EncodeToString(sum[:])
	  b.rows[id] = row
	  b.order = append(b.order, id)
	  b.chainHead = row.Checksum
	  return row, nil
  }

// MustAllow 广播前核对。非 allow 或未钉失败即关。

  func (b *Binder) MustAllow(movementID string) error {
	  if b == nil {
		  return ErrUnavailable
	   }
	   id := strings.TrimSpace(movementID)
	  if id == "" {
		   return ErrBindID
	   }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   cur, ok := b.rows[id]
	  if !ok {
		   return ErrBindMiss
	  }
	  if cur.Decision != Allow {
		   return ErrDenied
	   }
	   return nil
   }

  func (b *Binder) Must(movementID string, want Decision) error {
	  if b == nil {
		  return ErrUnavailable
	   }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   cur, ok := b.rows[strings.TrimSpace(movementID)]
	  if !ok {
		  return ErrBindMiss
	  }
	   if cur.Decision != want {
		  return ErrBindDrift
	   }
	  return nil
   }

// VerifyChain 按钉住顺序核 prev→checksum。断链失败即关。

  func (b *Binder) VerifyChain() error {
	   if b == nil {
		  return ErrUnavailable
	   }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	  prev := ""
	   for _, id := range b.order {
		  row, ok := b.rows[id]
		  if !ok {
			  return ErrBindChain
		  }
		  canon := strings.Join([]string{row.PrevChecksum, row.MovementID, string(row.Decision), row.Reason}, "|")
		  sum := sha256.Sum256([]byte(canon))
		  if hex.EncodeToString(sum[:]) != row.Checksum || row.PrevChecksum != prev {
			   return ErrBindChain
		  }
		  prev = row.Checksum
	   }
	  if b.chainHead != prev {
		  return ErrBindChain
	   }
	  return nil
   }

type EvidenceGap struct {
	MovementID string `json:"movement_id,omitempty"`
	Reason     string `json:"reason"`
}

// Scan 扫描钉住顺序与 checksum。orphan=有行无序，missing=有序无行，break=哈希或 prev 断。

  func (b *Binder) Scan() []EvidenceGap {
	  if b == nil {
		  return []EvidenceGap{{Reason: "unavailable"}}
	   }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	  inOrder := map[string]struct{}{}
	  out := make([]EvidenceGap, 0)
	  prev := ""
	   for _, id := range b.order {
		  inOrder[id] = struct{}{}
		  row, ok := b.rows[id]
		   if !ok {
			   out = append(out, EvidenceGap{MovementID: id, Reason: "missing"})
			  continue
		  }
		   canon := strings.Join([]string{row.PrevChecksum, row.MovementID, string(row.Decision), row.Reason}, "|")
		  sum := sha256.Sum256([]byte(canon))
		  if hex.EncodeToString(sum[:]) != row.Checksum || row.PrevChecksum != prev {
			  out = append(out, EvidenceGap{MovementID: id, Reason: "break"})
		   }
		   prev = row.Checksum
	   }
	   for id := range b.rows {
		   if _, ok := inOrder[id]; !ok {
			   out = append(out, EvidenceGap{MovementID: id, Reason: "orphan"})
		   }
	   }
	  if b.chainHead != prev && len(b.order) > 0 {
		  out = append(out, EvidenceGap{Reason: "head"})
	  }
	   return out
   }

// Gaps 证据链缺口。空切片表示连续。

   func (b *Binder) Gaps() []string {
	  scan := b.Scan()
	   if len(scan) == 0 {
		   return nil
	  }
	  out := make([]string, 0, len(scan))
	  for _, g := range scan {
		  out = append(out, g.Reason)
	  }
	   return out
   }
