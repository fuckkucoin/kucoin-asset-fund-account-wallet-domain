package collide

import (
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
)

// Book 进程内案件。折叠键是 chain|namespace|fold。
type Book struct {
	mu   sync.Mutex
	idx  map[string]string
	byID map[string]*Case
	byKey map[string]*Case
	clock func() time.Time
}

  func NewBook() *Book {
	   return &Book{
		   idx: map[string]string{},
		  byID:  map[string]*Case{},
		  byKey: map[string]*Case{},
		   clock: func() time.Time { return time.Now().UTC() },
	  }
   }

  func (b *Book) SetClock(fn func() time.Time) {
	  if b == nil || fn == nil {
		  return
	   }
	  b.mu.Lock()
	   b.clock = fn
	  b.mu.Unlock()
  }

   func keyOf(chain, ns, fold string) string {
	  return strings.ToUpper(strings.TrimSpace(chain)) + "|" + strings.TrimSpace(ns) + "|" + fold
  }

// Note 登记一个用户对折叠键的主张。
// 返回的 conflict 为真时，两边都不能入账，直到裁决释放其中一方。

   func (b *Book) Note(chain, ns, raw, fold, user, mode string) (Case, bool, error) {
	  if b == nil {
		   return Case{}, false, errBook
	  }
	  user = strings.TrimSpace(user)
	   fold = strings.TrimSpace(fold)
	   if user == "" || fold == "" {
		   return Case{}, false, errBook
	  }
	  if mode != "review" {
		   mode = "block_both"
	  }
	   k := keyOf(chain, ns, fold)
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   now := b.clock()
	  cur, ok := b.idx[k]
	   if !ok {
		   b.idx[k] = user
		  return Case{Chain: chain, Namespace: ns, Fold: fold, Origin: user, State: StClear, Mode: mode}, false, nil
	  }
	  if cur == user {
		  if c := b.byKey[k]; c != nil {
			  return *c, !c.Allows(user), nil
		  }
		  return Case{Chain: chain,
			   Namespace: ns, Fold: fold, Origin: user, State: StClear, Mode: mode}, false, nil
	  }
	   c := b.byKey[k]
	  if c == nil {
		  c = &Case{
			  ID: uuid.NewString(), Chain: strings.ToUpper(chain), Namespace: ns, Fold: fold,
			  Origin: cur, Claimants: []string{cur}, State: StClear, Mode: mode, Updated: now,
		  }
		  if raw != "" {
			  c.Raw = []string{raw}
		   }
		   b.byKey[k] = c
		  b.byID[c.ID] = c
	   }
	  if c.Allows(user) {
		   return *c, false, nil
	  }
	  if hasClaim(c.Claimants, user) || user == c.Origin {
		   return *c, !c.Allows(user), nil
	   }
	   ev := Event{Kind: "conflict", User: user, Reason: raw, At: now, Actor: "attrib-worker"}
	  next, err := Transit(*c, ev, now)
	  if err != nil {
		   return *c, true, err
	   }
	  *c = next
	   return *c, true, nil
  }
	   if err != nil {
		   return *c, err
	   }
	  *c = next
	  return *c, nil
  }

// Get 按编号读取。

  func (b *Book) Get(id string) (Case, bool) {
	  if b == nil {
		  return Case{}, false
	   }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	  c := b.byID[strings.TrimSpace(id)]
	  if c == nil {
		  return Case{}, false
	   }
	   return *c, true
  }

// ByFold 按折叠键读取。没有案件时 ok 为 false。

   func (b *Book) ByFold(chain, ns, fold string) (Case, bool) {
	   if b == nil {
		  return Case{}, false
	   }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   c := b.byKey[keyOf(chain, ns, fold)]
	  if c == nil {
		  return Case{}, false
	  }
	   return *c, true
   }

// Peek 只读。键上还没有主张时不挡住，也不写入。
// 簿为空、用户或折叠键为空时视为挡住。
   func (b *Book) Peek(chain, ns, fold, user string) bool {
	  if b == nil || strings.TrimSpace(user) == "" || strings.TrimSpace(fold) == "" {
		   return true
	   }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	  k := keyOf(chain, ns, fold)
	   if c := b.byKey[k]; c != nil {
		  return !c.Allows(user)
	  }
	   cur, ok := b.idx[k]
	  if !ok {
		   return false
	   }
	  return cur != user
   }

// Restore 把已落库的案件装回内存。同一折叠键出现两案时整键挡住。
  func (b *Book) Restore(in Case) bool {
	  if b == nil {
		  return false
	   }
	  c := NormalizeLoaded(in)
	  if c.ID == "" || c.Fold == "" || c.Chain == "" {
		   return false
	   }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  k := keyOf(c.Chain, c.Namespace, c.Fold)
	   if prev := b.byKey[k]; prev != nil && prev.ID != c.ID {
		  prev.State = StBlocked
		  prev.Winner = ""
		  prev.Reason = "duplicate fold"
		   return true
	  }
	   stored := c
	   b.byKey[k] = &stored
	   b.byID[stored.ID] = b.byKey[k]
	   if stored.Origin != "" {
		   b.idx[k] = stored.Origin
	  }
	   return true
   }

// NormalizeLoaded 校正从库读回的案件。未知状态、空胜诉人按挡住处理。
   func NormalizeLoaded(in Case) Case {
	  c := in
	  c.ID = strings.TrimSpace(c.ID)
	   c.Chain = strings.ToUpper(strings.TrimSpace(c.Chain))
	   c.Namespace = strings.TrimSpace(c.Namespace)
	  c.Fold = strings.TrimSpace(c.Fold)
	  c.Origin = strings.TrimSpace(c.Origin)
	  c.Winner = strings.TrimSpace(c.Winner)
	   if c.Origin != "" && !hasClaim(c.Claimants, c.Origin) {
		   c.Claimants = append([]string{c.Origin}, c.Claimants...)
	  }
	   switch c.State {
	   case StClear, StSuspect, StBlocked, StDesk, StEvidence, StArbitrate, StReleased, StRejected, StExpired:
	  default:
		   c.State = StBlocked
		   c.Winner = ""
	  }
	   if c.Mode != "review" {
		   c.Mode = "block_both"
	   }
	  if c.State == StReleased && (c.Winner == "" || (c.Winner != c.Origin && !hasClaim(c.Claimants, c.Winner))) {
		  c.State = StBlocked
		   c.Winner = ""
	   }
	   return c
   }

// Allows 无案时，只有索引上的用户可以通过。有案时看状态。

   func (b *Book) Allows(chain, ns, fold, user string) bool {
	   if b == nil || strings.TrimSpace(user) == "" {
		   return false
	   }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	  k := keyOf(chain, ns, fold)
	   c := b.byKey[k]
	   if c != nil {
		   return c.Allows(user)
	   }
	  cur, ok := b.idx[k]
	  if !ok {
		   return false
	   }
	   return cur == user
  }

// Expire 到期未决的案打成超时。已释放的不在这里翻案。
   func (b *Book) Expire() []Case {
	  if b == nil {
		   return nil
	   }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   now := b.clock()
	   var out []Case
	  for _, c := range b.byID {
		   if c.Deadline.IsZero() || now.Before(c.Deadline) {
			  continue
		  }
		  next, err := Transit(*c, Event{Kind: "timeout", Actor: "attrib-worker", At: now, Reason: "deadline"}, now)
		  if err != nil {
			   continue
		  }
		   *c = next
		   out = append(out, *c)
	  }
	  return out
  }

// Replay 重放一案。

   func (b *Book) Replay(id string) (Case, error) {
	  c, ok := b.Get(id)
	  if !ok {
		  return Case{}, errMiss
	   }
	  return Replay(c)
  }

// List 全部案件的副本。

   func (b *Book) List() []Case {
	   if b == nil {
		   return nil
	   }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	  out := make([]Case, 0, len(b.byID))
	  for _, c := range b.byID {
		   out = append(out, *c)
	  }
	   return out
   }
