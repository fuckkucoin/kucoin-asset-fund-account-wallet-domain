package investigate

import (
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
)

type Status string

const (
	StatusOpen     Status = "open"
	StatusWatching Status = "watching"
	StatusResolved Status = "resolved"
	StatusRejected Status = "rejected"
)

type Case struct {
	ID string `json:"id"`
	UserID  string `json:"user_id"`
	Subject string `json:"subject"`
	Reason  string `json:"reason"`
	Actor   string `json:"actor"`
	Status   Status        `json:"status"`
	OpenedAt time.Time     `json:"opened_at"`
	Fence    persist.Fence `json:"fence"`
}

type Note struct {
	ID string `json:"id"`
	CaseID string    `json:"case_id"`
	Actor  string    `json:"actor"`
	Body   string    `json:"body"`
	At     time.Time `json:"at"`
}

type Book struct {
	mu sync.Mutex
	cases map[string]*Case
	notes map[string][]Note
	byUser map[string][]string
}

  func NewBook() *Book {
	  return &Book{cases: map[string]*Case{}, notes: map[string][]Note{}, byUser: map[string][]string{}}
  }

   func (b *Book) Open(userID, subject, reason, actor string) (Case, error) {
	  uid, err := validate.UserID(userID)
	  if err != nil {
		  return Case{}, err
	   }
	  act, err := validate.Actor(actor)
	   if err != nil {
		  return Case{}, err
	   }
	   reason, err = validate.Reason(reason)
	  if err != nil {
		  return Case{}, err
	  }
	  subject = strings.TrimSpace(subject)
	   if subject == "" {
		   subject = "account"
	   }
	   c := Case{
		   ID: uuid.NewString(), UserID: uid, Subject: subject, Reason: reason,
		   Actor: act, Status: StatusOpen, OpenedAt: time.Now().UTC(),
	  }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	  cp := c
	  b.cases[c.ID] = &cp
	  b.byUser[uid] = append(b.byUser[uid], c.ID)
	  return c, nil
   }

   func (b *Book) Get(id string) (Case, error) {
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   c,
		  ok := b.cases[strings.TrimSpace(id)]
	  if !ok {
		  return Case{}, ErrNotFound
	   }
	  return *c, nil
  }

  func (b *Book) SetStatus(id, actor string, st Status) (Case, error) {
	  if _, err := validate.Actor(actor); err != nil {
		  return Case{}, err
	  }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	  c, ok := b.cases[strings.TrimSpace(id)]
	   if !ok {
		   return Case{}, ErrNotFound
	  }
	  if c.Status == StatusResolved || c.Status == StatusRejected {
		  return *c, ErrClosed
	   }
	   c.Status = st
	  return *c, nil
  }

  func (b *Book) ByUser(userID string) []Case {
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   out := make([]Case, 0)
	   for _, id := range b.byUser[strings.TrimSpace(userID)] {
		  if c := b.cases[id]; c != nil {
			  out = append(out, *c)
		  }
	  }
	  return out
   }

   func (b *Book) AddNote(caseID, actor, body string) (Note, error) {
	  if _, err := validate.Actor(actor); err != nil {
		   return Note{}, err
	   }
	  body = strings.TrimSpace(body)
	  if body == "" {
		   return Note{}, validate.ErrReason
	   }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   if _, ok := b.cases[strings.TrimSpace(caseID)]; !ok {
		  return Note{}, ErrNotFound
	   }
	   n := Note{ID: uuid.NewString(), CaseID: caseID, Actor: actor, Body: body, At: time.Now().UTC()}
	   b.notes[caseID] = append(b.notes[caseID], n)
	   return n, nil
  }

  func (b *Book) Notes(caseID string) []Note {
	  b.mu.Lock()
	  defer b.mu.Unlock()
	  src := b.notes[caseID]
	  out := make([]Note, len(src))
	   copy(out, src)
	   return out
  }
