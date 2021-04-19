// 领域服务：业务用例编排与校验。

// Author: Cason
// Date: 2021.04.13

package disputedesk

import (
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/validate"
)

type Service struct {
	mu sync.Mutex
	cases  map[string]*Case
	ev     map[string][]Evidence
	byIdem map[string]string
	events []Event
	gens   map[string]*CaseGeneration
	clock  func() time.Time
	gateChecker GateChecker
}

  func New() *Service {
	   return &Service{
		  cases:  map[string]*Case{},
		  ev:     map[string][]Evidence{},
		  byIdem: map[string]string{},
		   gens:   map[string]*CaseGeneration{},
		   clock: func() time.Time { return time.Now().UTC() },
	  }
   }

  func (s *Service) now() time.Time {
	   if s == nil || s.clock == nil {
		   return time.Now().UTC()
	   }
	   return s.clock()
  }

  func (s *Service) Open(req OpenReq) (Case, error) {
	   if s == nil {
		   return Case{}, ErrUnavailable
	   }
	  ch, err := ParseChannel(string(req.Channel))
	  if err != nil {
		  return Case{}, err
	   }
	  uid, err := validate.UserID(req.UserID)
	  if err != nil {
		   return Case{}, ErrUser
	   }
	  asset,
		   err := validate.Asset(req.Asset)
	   if err != nil {
		  return Case{}, ErrAsset
	  }
	   amt, err := money.Parse(req.Amount)
	   if err != nil {
		   return Case{}, ErrAmount
	  }
	  actor, err := validate.Actor(req.OpenedBy)
	  if err != nil {
		   return Case{}, ErrActor
	   }
	   reason := strings.TrimSpace(req.Reason)
	   if reason == "" || len(reason) > 240 {
		  return Case{}, ErrReason
	  }
	   idem, err := validate.Idem(req.IdempotencyKey)
	  if err != nil {
		   return Case{}, ErrIdem
	   }
	  cp := strings.TrimSpace(req.Counterparty)
	  if cp != "" {
		   if _, err := validate.UserID(cp); err != nil {
			   return Case{}, ErrUser
		  }
	   }
	  s.mu.Lock()
	   if id, ok := s.byIdem[idem]; ok {
		  cur := s.cases[id]
		   s.mu.Unlock()
		  if cur.UserID != uid || cur.Amount != amt.String() || cur.Channel != ch {
			   return Case{}, ErrIdem
		   }
		  return cur.Copy(), nil
	   }
	  now := s.now()
	   row := &Case{
		  ID: uuid.NewString(), IdempotencyKey: idem, Channel: ch, UserID: uid,
		   Counterparty: cp, Asset: asset, Amount: amt.String(), Reason: reason,
		  RefID: strings.TrimSpace(req.RefID), Status: StatusOpen, OpenedBy: actor,
		  CreatedAt: now, UpdatedAt: now,
	  }
	  s.cases[row.ID] = row
	  s.byIdem[idem] = row.ID
	  s.ensureGenLocked(row.ID)
	   s.noteLocked(row.ID, actor, "open", reason)
	   s.mu.Unlock()
	   return row.Copy(), nil
  }

  func (s *Service) Get(id string) (Case, error) {
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

   func (s *Service) List(channel, userID string, limit int) []Case {
	   if s == nil {
		   return nil
	  }
	   if limit <= 0 {
		   limit = 50
	   }
	  channel = strings.TrimSpace(channel)
	  userID = strings.TrimSpace(userID)
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   out := make([]Case, 0, limit)
	   for _, c := range s.cases {
		  if channel != "" && string(c.Channel) != channel {
			  continue
		   }
		  if userID != "" && c.UserID != userID {
			   continue
		   }
		   out = append(out, c.Copy())
		  if len(out) >= limit {
			  break
		  }
	  }
	  return out
  }
