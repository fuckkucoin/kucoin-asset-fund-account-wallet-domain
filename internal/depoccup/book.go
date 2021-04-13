package depoccup

import (
	"fmt"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
)

type Service struct {
	mu sync.Mutex
	policies []*Policy
	byKey  map[string]*Policy
	lots   map[string]*Lot
	byXfer map[string]string
	byIdem map[string]string
	tiers map[string]*TierState
	moves map[string][]TierMove
	signals   map[string]*Signal
	sigByUser map[string][]string
	buckets   map[string]*Bucket
	audits []Audit
	src  *SourceBook
	dust DustReader
	kyc  KYCReader
	roll RollbackPort
	delay DelaySource
	db    *persist.SQLDB
	seq   uint64
	noteErr int
	saveErr string
	loadErr string
	clock   func() time.Time
}

   func New() *Service {
	  return &Service{
		   byKey: map[string]*Policy{},
		   lots:   map[string]*Lot{},
		  byXfer: map[string]string{},
		  byIdem: map[string]string{},
		  tiers:  map[string]*TierState{},
		   moves:  map[string][]TierMove{},
		  signals:   map[string]*Signal{},
		   sigByUser: map[string][]string{},
		  buckets:   map[string]*Bucket{},
		   src:   NewSourceBook(),
		  clock: func() time.Time { return time.Now().UTC() },
	   }
  }

   func (s *Service) now() time.Time {
	  if s == nil || s.clock == nil {
		  return time.Now().UTC()
	   }
	   return s.clock().UTC()
  }

   func (s *Service) nextID(prefix string) string {
	   s.seq++
	  return fmt.Sprintf("%s-%d-%d", prefix, s.now().UnixNano(), s.seq)
   }

  func normUser(id string) (string, error) {
	  uid, err := validate.UserID(id)
	  if err != nil {
		  return "", ErrUser
	  }
	  return uid, nil
   }

  func bucketKey(user, asset string) string { return user + "|" + asset }

  func (s *Service) bucketLocked(user, asset string) *Bucket {
	  k := bucketKey(user, asset)
	   b := s.buckets[k]
	   if b == nil {
		   b = &Bucket{UserID: user, Asset: asset, InTransit: "0", Available: "0", Rolled: "0"}
		   s.buckets[k] = b
	   }
	  return b
  }

   func (s *Service) BindSQL(db *persist.SQLDB) {
	   if s == nil {
		   return
	  }
	   s.mu.Lock()
	   s.db = db
	   s.mu.Unlock()
  }

   func (s *Service) BindRollback(p RollbackPort) {
	   if s == nil {
		  return
	   }
	   s.mu.Lock()
	  s.roll = p
	   s.mu.Unlock()
   }

   func (s *Service) BindKYC(r KYCReader) {
	   if s == nil {
		   return
	   }
	   s.mu.Lock()
	   s.kyc = r
	   s.mu.Unlock()
   }

  func (s *Service) BindDelay(d DelaySource) {
	  if s == nil {
		  return
	  }
	   s.mu.Lock()
	   s.delay = d
	  s.mu.Unlock()
   }

  func (s *Service) Sources() *SourceBook {
	  if s == nil {
		  return nil
	  }
	  return s.src
   }

   func (s *Service) bumpNoteErr() {
	  if s == nil {
		   return
	  }
	   s.mu.Lock()
	  s.noteErr++
	   s.mu.Unlock()
  }
