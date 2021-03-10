package cases

import (
	"errors"
	"sync"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/persist"
)

type Store struct {
	mu   sync.Mutex
	byID map[string]*Case
	byMove map[string]string
	audit []AuditEvent
	fence *persist.Table
}

   func NewStore() *Store {
	  return &Store{byID: map[string]*Case{}, byMove: map[string]string{}, fence: persist.NewTable()}
   }

  func (s *Store) appendAuditLocked(caseID, actor string, ev Event, from, to State, note string) {
	   s.audit = append(s.audit, AuditEvent{
		   ID: uuid.NewString(), CaseID: caseID, Actor: actor,
		   Event: string(ev), From: from, To: to, Note: note, At: time.Now().UTC(),
	  })
   }

   func (s *Store) Open(movementID, userID, kind, reason string, policyVer int) (*Case, error) {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   if id, ok := s.byMove[movementID]; ok {
		   cur := s.byID[id]
		   if cur != nil && cur.Open() {
			  cp := *cur
			  return &cp, ErrExists
		   }
	   }
	   now := time.Now().UTC()
	  c := &Case{
		  ID: uuid.NewString(), MovementID: movementID, UserID: userID, Kind: kind,
		  Reason: reason,
		  State:  StateOpen, PolicyVer: policyVer, CreatedAt: now, UpdatedAt: now,
	   }
	   s.byID[c.ID] = c
	   s.byMove[movementID] = c.ID
	   if f, err := s.fence.Bump(c.ID, persist.NewToken()); err == nil {
		  c.FenceEpoch = f.Epoch
		   c.FenceToken = f.Token
	  }
	  s.appendAuditLocked(c.ID, "system", Event(""), State(""), StateOpen, reason)
	  cp := *c
	   return &cp, nil
   }

  func (s *Store) Get(id string) (*Case, error) {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   c, ok := s.byID[id]
	   if !ok {
		   return nil, ErrNotFound
	   }
	  cp := *c
	  return &cp, nil
   }

   func (s *Store) ByMovement(movementID string) (*Case, error) {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   id, ok := s.byMove[movementID]
	  if !ok {
		  return nil, ErrNotFound
	   }
	  c := s.byID[id]
	  cp := *c
	   return &cp, nil
   }

   func (s *Store) ListOpen(limit int) []Case {
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   if limit <= 0 {
		  limit = 50
	  }
	   out := make([]Case, 0, limit)
	  for _, c := range s.byID {
		  if !c.Open() {
			  continue
		  }
		  out = append(out, *c)
		   if len(out) >= limit {
			   break
		  }
	  }
	  return out
  }

  func (s *Store) Transition(id, actor string, ev Event) (*Case, error) {
	  return s.transitionLocked(id, actor, ev, 0, "", false)
   }

// TransitionFenced 结案必须带当前 epoch/token。两个复核人同时批同一案，只有先到的能过。

  func (s *Store) TransitionFenced(id, actor string, ev Event, epoch int64, token string) (*Case, error) {
	  return s.transitionLocked(id, actor, ev, epoch, token, true)
  }

   func (s *Store) transitionLocked(id, actor string, ev Event, epoch int64, token string, fenced bool) (*Case, error) {
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  c, ok := s.byID[id]
	  if !ok {
		   return nil, ErrNotFound
	   }
	   if fenced {
		  if err := s.fence.Check(id, epoch, token); err != nil {
			  if errors.Is(err, persist.ErrStaleFence) {
				   return nil, ErrStaleFence
			  }
			  return nil, err
		   }
	  }
	   if !c.Open() {
		  cp := *c
		   return &cp, ErrClosed
	  }
	   next, err := Apply(c.State, ev)
	  if err != nil {
		   return nil, err
	   }
	  prev := c.State
	   c.State = next
	  c.UpdatedAt = time.Now().UTC()
	   switch ev {
	  case EventAssign:
		   c.Assignee = actor
	  case EventApprove, EventDeny:
		  c.DecidedBy = actor
	   }
	   if f, err := s.fence.Bump(c.ID, persist.NewToken()); err == nil {
		   c.FenceEpoch = f.Epoch
		   c.FenceToken = f.Token
	   }
	  s.appendAuditLocked(c.ID, actor, ev, prev, next, "")
	  cp := *c
	  return &cp, nil
   }

// ExpireOlderThan 关掉超时仍 open/assigned 的案。过期不是放行，出金必须停在冻结或走失败回滚。
   func (s *Store) ExpireOlderThan(age time.Duration) []Case {
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  if age <= 0 {
		   age = 24 * time.Hour
	  }
	   now := time.Now().UTC()
	   out := make([]Case, 0)
	   for _, c := range s.byID {
		  if !c.Open() {
			  continue
		   }
		  if now.Sub(c.CreatedAt) < age {
			   continue
		   }
		   prev := c.State
		  c.State = StateExpired
		  c.UpdatedAt = now
		   if f, err := s.fence.Bump(c.ID, persist.NewToken()); err == nil {
			   c.FenceEpoch = f.Epoch
			   c.FenceToken = f.Token
		   }
		   s.appendAuditLocked(c.ID, "system", EventExpire, prev, StateExpired, "stale review; fail-closed")
		  out = append(out, *c)
	  }
	  return out
  }

  func (s *Store) Audit(caseID string, limit int) []AuditEvent {
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   if limit <= 0 {
		  limit = 100
	  }
	   out := make([]AuditEvent, 0, limit)
	   for i := len(s.audit) - 1; i >= 0 && len(out) < limit; i-- {
		  if caseID != "" && s.audit[i].CaseID != caseID {
			   continue
		   }
		   out = append(out, s.audit[i])
	   }
	   return out
   }
