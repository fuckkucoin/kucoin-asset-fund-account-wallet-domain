package amldesk

import (
	"strings"
	"time"
	"github.com/google/uuid"
)

type Event struct {
	ID string `json:"id"`
	CaseID string `json:"case_id"`
	Actor  string `json:"actor"`
	Kind string    `json:"kind"`
	Note string    `json:"note,omitempty"`
	At   time.Time `json:"at"`
}

// noteLocked 记时间线。调用方必须已持 mu，避免开案和结案各写一半。
  func (b *Book) noteLocked(caseID, actor, kind, note string) {
	  if b == nil || b.tl == nil {
		   return
	  }
	   ev := Event{
		   ID:     uuid.NewString(),
		   CaseID: caseID, Actor: actor,
		   Kind: strings.TrimSpace(kind), Note: strings.TrimSpace(note), At: b.clock(),
	   }
	   b.tl[caseID] = append(b.tl[caseID], ev)
   }

  func (b *Book) Timeline(caseID string) []Event {
	   if b == nil {
		  return nil
	  }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   src := b.tl[strings.TrimSpace(caseID)]
	  out := make([]Event, len(src))
	  copy(out, src)
	  return out
  }
