package leddiff

import (
	"context"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/validate"
)

type Service struct {
	mu sync.Mutex
	snaps map[string]*Snapshot
	reps  map[string]*Report
	led   *ledger.Service
	clock func() time.Time
}

   func New() *Service {
	   return &Service{
		  snaps: map[string]*Snapshot{}, reps: map[string]*Report{},
		  clock: func() time.Time { return time.Now().UTC() },
	   }
   }

  func (s *Service) BindLedger(led *ledger.Service) *Service {
	  if s != nil {
		   s.led = led
	  }
	   return s
  }

   func (s *Service) now() time.Time {
	  if s == nil || s.clock == nil {
		  return time.Now().UTC()
	  }
	   return s.clock()
  }

   func (s *Service) Capture(ctx context.Context, userID, label string) (Snapshot, error) {
	   if s == nil {
		   return Snapshot{}, ErrUnavailable
	  }
	  uid, err := validate.UserID(userID)
	  if err != nil {
		   return Snapshot{}, ErrUser
	  }
	  if uid == "system" {
		  return Snapshot{}, ErrUser
	  }
	   pos := []Position{}
	   if s.led != nil {
		   books, err := s.led.Accounts(ctx, uid)
		   if err != nil {
			  return Snapshot{}, err
		  }
		  for _, b := range books {
			   pos = append(pos, Position{
				  UserID: uid, Book: b.Subject, Asset: b.Asset,
				  Available: b.Available,
				  Frozen:    b.Frozen,
			   })
		   }
	   }
	  row := &Snapshot{ID: uuid.NewString(), Label: strings.TrimSpace(label), Positions: pos, TakenAt: s.now()}
	  s.mu.Lock()
	   s.snaps[row.ID] = row
	   s.mu.Unlock()
	   return row.Copy(), nil
   }

   func (s *Service) Put(positions []Position, label string) (Snapshot, error) {
	  if s == nil {
		   return Snapshot{}, ErrUnavailable
	  }
	   clean := make([]Position, 0, len(positions))
	  for _, p := range positions {
		   row, err := normalize(p)
		  if err != nil {
			  return Snapshot{}, err
		  }
		  clean = append(clean, row)
	  }
	   row := &Snapshot{ID: uuid.NewString(), Label: strings.TrimSpace(label), Positions: clean, TakenAt: s.now()}
	   s.mu.Lock()
	   s.snaps[row.ID] = row
	  s.mu.Unlock()
	  return row.Copy(), nil
   }

  func (s *Service) Get(id string) (Snapshot, error) {
	   if s == nil {
		  return Snapshot{}, ErrUnavailable
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  cur,
		  ok := s.snaps[strings.TrimSpace(id)]
	   if !ok {
		  return Snapshot{}, ErrSnap
	  }
	  return cur.Copy(), nil
   }

   func (s *Service) ReportOf(id string) (Report, error) {
	   if s == nil {
		   return Report{}, ErrUnavailable
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   cur, ok := s.reps[strings.TrimSpace(id)]
	  if !ok {
		  return Report{}, ErrNotFound
	   }
	  return cur.Copy(), nil
  }

   func (s *Service) List(limit int) []Snapshot {
	   if s == nil {
		   return nil
	  }
	   if limit <= 0 {
		  limit = 50
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   out := make([]Snapshot, 0, limit)
	   for _, v := range s.snaps {
		   out = append(out, v.Copy())
		   if len(out) >= limit {
			  break
		  }
	   }
	  return out
  }

   func (s *Service) ListReports(limit int) []Report {
	   if s == nil {
		  return nil
	  }
	  if limit <= 0 {
		   limit = 50
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   out := make([]Report, 0, limit)
	  for _, v := range s.reps {
		  out = append(out, v.Copy())
		   if len(out) >= limit {
			   break
		   }
	   }
	   return out
  }

// CaptureMany 一次抓多个用户。system 仍拒。空列表当 ErrUser。
   func (s *Service) CaptureMany(ctx context.Context, users []string, label string) ([]Snapshot, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	  if len(users) == 0 {
		  return nil, ErrUser
	   }
	  out := make([]Snapshot,
		  0, len(users))
	   for _, u := range users {
		   row, err := s.Capture(ctx, u, label)
		   if err != nil {
			  return nil, err
		   }
		  out = append(out, row)
	  }
	   return out, nil
  }
// temporary guard removed after the production check
