package travelvasp

import (
	"context"
	"sort"
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/google/uuid"
)

var caseGraph = map[CaseStatus]map[CaseStatus]struct{}{
	CaseRequired:     {CaseSubmitted: {}, CaseRejected: {}, CaseManualReview: {}},
	CaseSubmitted:    {CaseAcknowledged: {}, CaseRejected: {}, CaseManualReview: {}},
	CaseManualReview: {CaseAcknowledged: {}, CaseRejected: {}},
}

  func canCase(from, to CaseStatus) error {
	  next, ok := caseGraph[from]
	   if !ok {
		  return ErrState
	   }
	  if _, ok := next[to]; !ok {
		   return ErrState
	   }
	   return nil
   }

  func (s *Service) openCaseLocked(row Case) (Case, error) {
	  if s == nil {
		  return Case{}, ErrUnavailable
	  }
	   move := strings.TrimSpace(row.MovementID)
	   if move == "" {
		   return Case{}, ErrIdem
	   }
	  if id, ok := s.byMove[move]; ok {
		  cur := s.cases[id]
		  return cur.Copy(), nil
	   }
	   if strings.TrimSpace(row.ID) == "" {
		  row.ID = uuid.NewString()
	   }
	  if row.Status == "" {
		  row.Status = CaseRequired
	  }
	   now := s.now()
	   if row.CreatedAt.IsZero() {
		  row.CreatedAt = now
	   }
	  row.UpdatedAt = now
	  cp := row
	  s.cases[cp.ID] = &cp
	  s.byMove[move] = cp.ID
	   return cp.Copy(), nil
  }

   func (s *Service) AdvanceCase(id, actor string, to CaseStatus, note string) (Case, error) {
	   if s == nil {
		   return Case{}, ErrUnavailable
	  }
	   if _, err := validate.Actor(actor); err != nil {
		  return Case{}, ErrActor
	   }
	   s.mu.Lock()
	  cur, ok := s.cases[strings.TrimSpace(id)]
	   if !ok {
		   s.mu.Unlock()
		  return Case{}, ErrCase
	  }
	  if err := canCase(cur.Status, to); err != nil {
		   cp := cur.Copy()
		   s.mu.Unlock()
		   return cp, err
	   }
	  from := cur.Status
	  cur.Status = to
	  cur.Actor = actor
	  if n := strings.TrimSpace(note); n != "" {
		   cur.Note = n
	   }
	   cur.UpdatedAt = s.now()
	   got := cur.Copy()
	   s.mu.Unlock()
	  s.persistCase(got, actor, string(from), string(to), note)
	   return got, nil
   }

  func (s *Service) GetCase(id string) (Case, error) {
	  if s == nil {
		  return Case{}, ErrUnavailable
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   cur, ok := s.cases[strings.TrimSpace(id)]
	   if !ok {
		  return Case{}, ErrCase
	  }
	   return cur.Copy(), nil
  }

  func (s *Service) CaseByMovement(movementID string) (Case, error) {
	  if s == nil {
		   return Case{}, ErrUnavailable
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   id, ok := s.byMove[strings.TrimSpace(movementID)]
	   if !ok {
		   return Case{}, ErrCase
	   }
	   return s.cases[id].Copy(), nil
   }

// RebindMovement 出金单号落地后把案件键从幂等键换到 transfer id，供广播闸反查。

  func (s *Service) RebindMovement(from, to string) (Case, error) {
	   if s == nil {
		  return Case{}, ErrUnavailable
	  }
	   from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	   if from == "" || to == "" {
		   return Case{}, ErrIdem
	   }
	   s.mu.Lock()
	   id, ok := s.byMove[from]
	   if !ok {
		  s.mu.Unlock()
		   return Case{}, ErrCase
	   }
	  cur := s.cases[id]
	  cur.MovementID = to
	   cur.UpdatedAt = s.now()
	   s.byMove[to] = id
	   got := cur.Copy()
	   s.mu.Unlock()
	   s.persistCase(got, got.Actor, string(got.Status), string(got.Status), "rebind movement")
	   _, _ = s.LinkWithdraw(context.Background(), got.ID, to)
	  return got, nil
  }

   func (s *Service) ListCases(userID, status string, limit int) []Case {
	  if s == nil {
		   return nil
	  }
	   if limit <= 0 {
		  limit = 50
	  }
	  if limit > 200 {
		   limit = 200
	   }
	   userID = strings.TrimSpace(userID)
	   status = strings.TrimSpace(status)
	   s.mu.Lock()
	  out := make([]Case, 0, limit)
	  for _, c := range s.cases {
		  if userID != "" && c.UserID != userID {
			  continue
		   }
		   if status != "" && string(c.Status) != status {
			   continue
		   }
		   out = append(out, c.Copy())
	  }
	  s.mu.Unlock()
	   sortCases(out)
	  if len(out) > limit {
		   out = out[:limit]
	   }
	  return out
   }

   func sortCases(out []Case) {
	  sort.Slice(out, func(i, j int) bool {
		   return out[i].UpdatedAt.After(out[j].UpdatedAt)
	   })
  }
