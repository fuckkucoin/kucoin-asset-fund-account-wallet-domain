package amldesk

import (
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/validate"
)

type WatchKind string

const (
	WatchUser WatchKind = "user"
	WatchDest WatchKind = "dest"
	WatchVASP WatchKind = "vasp"
)

type WatchEntry struct {
	Kind WatchKind `json:"kind"`
	Subject string `json:"subject"`
	Reason  string `json:"reason"`
	Actor string    `json:"actor"`
	At    time.Time `json:"at"`
}

type WatchHit struct {
	Kind WatchKind `json:"kind"`
	Subject string `json:"subject"`
	Reason string `json:"reason"`
}

type Watchlist struct {
	mu   sync.Mutex
	rows map[string]WatchEntry
	clock func() time.Time
}

   func NewWatchlist() *Watchlist {
	   return &Watchlist{rows: map[string]WatchEntry{}, clock: func() time.Time { return time.Now().UTC() }}
  }

  func watchKey(kind WatchKind, subject string) string {
	  return string(kind) + "|" + strings.ToLower(strings.TrimSpace(subject))
  }

   func ParseWatchKind(s string) (WatchKind, error) {
	   switch WatchKind(strings.ToLower(strings.TrimSpace(s))) {
	  case WatchUser, "":
		   return WatchUser, nil
	   case WatchDest:
		  return WatchDest, nil
	   case WatchVASP:
		   return WatchVASP, nil
	   default:
		   return "", ErrWatch
	  }
  }

   func (w *Watchlist) Put(kind WatchKind, subject, reason, actor string) (WatchEntry, error) {
	   if w == nil {
		  return WatchEntry{}, ErrUnavailable
	   }
	   act, err := actorOK(actor)
	   if err != nil {
		   return WatchEntry{}, ErrActor
	  }
	   subject = strings.TrimSpace(subject)
	  if subject == "" || len(subject) > 128 {
		   return WatchEntry{}, ErrWatch
	  }
	  reason, err = validate.Reason(reason)
	  if err != nil {
		   return WatchEntry{}, err
	   }
	   if kind == "" {
		   kind = WatchUser
	  }
	   ent := WatchEntry{Kind: kind, Subject: subject, Reason: reason, Actor: act, At: w.clock()}
	   w.mu.Lock()
	   w.rows[watchKey(kind, subject)] = ent
	   w.mu.Unlock()
	   return ent, nil
   }

  func (w *Watchlist) Remove(kind WatchKind, subject string) {
	   if w == nil {
		   return
	   }
	   w.mu.Lock()
	   delete(w.rows, watchKey(kind, subject))
	  w.mu.Unlock()
   }

  func (w *Watchlist) Get(kind WatchKind, subject string) (WatchEntry, bool) {
	  if w == nil {
		  return WatchEntry{}, false
	  }
	   w.mu.Lock()
	   defer w.mu.Unlock()
	  e, ok := w.rows[watchKey(kind, subject)]
	  return e, ok
   }

  func (w *Watchlist) List(limit int) []WatchEntry {
	  if w == nil {
		   return nil
	   }
	   w.mu.Lock()
	  defer w.mu.Unlock()
	  if limit <= 0 {
		   limit = 100
	  }
	  out := make([]WatchEntry, 0, limit)
	  for _, e := range w.rows {
		  out = append(out, e)
		   if len(out) >= limit {
			  break
		   }
	   }
	   return out
   }

// Match 按用户、收款地址、VASP 匹配。命中只出 hit，不得在这里改账或关案。
  func (w *Watchlist) Match(userID, dest, vasp string) []WatchHit {
	   if w == nil {
		  return nil
	   }
	  w.mu.Lock()
	  defer w.mu.Unlock()
	  out := make([]WatchHit, 0, 3)
	  if e, ok := w.rows[watchKey(WatchUser, userID)]; ok {
		   out = append(out, WatchHit{Kind: WatchUser, Subject: e.Subject, Reason: e.Reason})
	  }
	  if dest != "" {
		  if e, ok := w.rows[watchKey(WatchDest, dest)]; ok {
			  out = append(out, WatchHit{Kind: WatchDest, Subject: e.Subject, Reason: e.Reason})
		  }
	  }
	   if vasp != "" {
		  if e, ok := w.rows[watchKey(WatchVASP, vasp)]; ok {
			   out = append(out, WatchHit{Kind: WatchVASP, Subject: e.Subject, Reason: e.Reason})
		  }
	   }
	   return out
  }

   func (w *Watchlist) MatchTravel(p Payload) []WatchHit {
	  return w.Match(p.Originator.Account, p.Beneficiary.Account, p.Beneficiary.VASP)
   }
