package amldesk

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/google/uuid"
)

type Evidence struct {
	ID string `json:"id"`
	CaseID string `json:"case_id"`
	Actor string `json:"actor"`
	Kind string `json:"kind"`
	Checksum string `json:"checksum"`
	Bytes    int    `json:"bytes"`
	Note     string `json:"note,omitempty"`
	AttachedAt time.Time `json:"attached_at"`
}

// Attach 只存摘要。原文不入库，避免把链上私钥或用户证件落进案卷。
   func (b *Book) Attach(caseID, actor, kind, body, note string) (Evidence, error) {
	   if b == nil {
		  return Evidence{}, ErrUnavailable
	   }
	  act, err := actorOK(actor)
	  if err != nil {
		   return Evidence{}, ErrActor
	  }
	  kind = strings.ToLower(strings.TrimSpace(kind))
	  if kind == "" {
		   kind = "note"
	  }
	  body = strings.TrimSpace(body)
	   if body == "" {
		  return Evidence{}, ErrEvidence
	   }
	   if len(body) > 1<<20 {
		  return Evidence{}, fmt.Errorf("%w: too large", ErrEvidence)
	   }
	  if note != "" {
		   if _, err := validate.Reason(note); err != nil {
			  return Evidence{}, err
		  }
	   }
	   sum := sha256.Sum256([]byte(body))
	   ev := Evidence{
		   ID: uuid.NewString(), CaseID: strings.TrimSpace(caseID), Actor: act,
		  Kind: kind, Checksum: hex.EncodeToString(sum[:]), Bytes: len(body),
		   Note: note, AttachedAt: b.clock(),
	  }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	  r, ok := b.rows[ev.CaseID]
	  if !ok {
		   return Evidence{}, ErrNotFound
	   }
	  if r.Status == StatusClosed || r.Status == StatusRejected {
		  return Evidence{}, ErrClosed
	  }
	  b.ev[ev.CaseID] = append(b.ev[ev.CaseID], ev)
	  if r.Status == StatusOpen {
		   r.Status = StatusEvidence
	  }
	   b.noteLocked(ev.CaseID, act, "evidence", kind+" "+ev.Checksum)
	  return ev, nil
   }

  func (b *Book) Evidence(caseID string) []Evidence {
	   if b == nil {
		  return nil
	  }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   src := b.ev[strings.TrimSpace(caseID)]
	  out := make([]Evidence, len(src))
	   copy(out, src)
	   return out
   }

   func VerifyEvidence(ev Evidence, body string) error {
	  sum := sha256.Sum256([]byte(strings.TrimSpace(body)))
	  if hex.EncodeToString(sum[:]) != ev.Checksum {
		  return fmt.Errorf("%w: checksum", ErrEvidence)
	  }
	   return nil
  }
