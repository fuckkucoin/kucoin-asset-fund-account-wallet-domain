package balproof

import (
	"context"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/google/uuid"
)

type Service struct {
	mu sync.Mutex
	secret string
	proofs map[string]Proof
	nonces map[string]time.Time
	ttl time.Duration
	led *ledger.Service
	clock func() time.Time
}

  func New(secret string) *Service {
	  secret = strings.TrimSpace(secret)
	   return &Service{
		  secret: secret, proofs: map[string]Proof{}, nonces: map[string]time.Time{},
		   ttl:   15 * time.Minute,
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

  func (s *Service) SetClock(fn func() time.Time) {
	   if s != nil && fn != nil {
		  s.clock = fn
	  }
  }

   func (s *Service) SetTTL(d time.Duration) error {
	  if s == nil {
		   return ErrUnavailable
	  }
	  if d <= 0 {
		  return ErrExpired
	  }
	   s.mu.Lock()
	  s.ttl = d
	  s.mu.Unlock()
	   return nil
  }

   func (s *Service) Issue(ctx context.Context, req IssueReq) (Proof, error) {
	  if s == nil {
		   return Proof{}, ErrUnavailable
	  }
	  if strings.TrimSpace(s.secret) == "" {
		  return Proof{}, ErrSecret
	   }
	   uid, err := validate.UserID(req.UserID)
	   if err != nil {
		   return Proof{}, ErrUser
	   }
	   if uid == "system" {
		   return Proof{}, ErrUser
	   }
	   book, err := validate.Book(req.Book)
	   if err != nil {
		  return Proof{}, ErrBook
	  }
	   as, err := validate.Asset(req.Asset)
	   if err != nil {
		   return Proof{}, ErrAsset
	  }
	  avail, frozen := "0", "0"
	  if s.led != nil {
		   books, err := s.led.Accounts(ctx, uid)
		   if err != nil {
			   return Proof{}, err
		   }
		  found := false
		   for _, b := range books {
			   if b.Subject == book && b.Asset == as {
				   avail, frozen = b.Available, b.Frozen
				   found = true
				  break
			   }
		  }
		   if !found {
			   // 没有该科目时仍出 0/0 快照，调用方能核验签名。
			   avail, frozen = "0", "0"
		  }
	  }
	  tot, err := totalOf(avail, frozen)
	   if err != nil {
		   return Proof{}, err
	  }
	   s.mu.Lock()
	   if err := s.takeNonce(req.Nonce); err != nil {
		  s.mu.Unlock()
		   return Proof{}, err
	   }
	  s.mu.Unlock()
	  now := s.now()
	   can := Canonical(uid,
		  book, as, avail, frozen, req.Nonce, now)
	  row := Proof{
		   ID: uuid.NewString(), UserID: uid, Book: book, Asset: as,
		   Available: avail, Frozen: frozen, Total: tot, Nonce: strings.TrimSpace(req.Nonce),
		   At: now, Canonical: can, Signature: Sign(s.secret, can), Disclaimer: Disclaimer,
		   Deprecated: true, Solvency: false, Class: EngineeringClass,
	   }
	  s.mu.Lock()
	   s.proofs[row.ID] = row
	  s.mu.Unlock()
	  return row.Copy(), nil
   }

  func (s *Service) Check(req VerifyReq) error {
	  if s == nil {
		  return ErrUnavailable
	   }
	  uid, err := validate.UserID(req.UserID)
	  if err != nil {
		   return ErrUser
	   }
	   book,
		   err := validate.Book(req.Book)
	   if err != nil {
		  return ErrBook
	   }
	  as, err := validate.Asset(req.Asset)
	  if err != nil {
		   return ErrAsset
	  }
	  if _, err := totalOf(req.Available, req.Frozen); err != nil {
		  return err
	   }
	  at,
		   err := time.Parse(time.RFC3339Nano, strings.TrimSpace(req.At))
	  if err != nil {
		   at, err = time.Parse(time.RFC3339, strings.TrimSpace(req.At))
		   if err != nil {
			   return ErrNonce
		   }
	   }
	  can := Canonical(uid, book, as, req.Available, req.Frozen, req.Nonce, at)
	  return Verify(s.secret, can, req.Signature)
   }

  func (s *Service) Get(id string) (Proof, error) {
	   if s == nil {
		  return Proof{}, ErrUnavailable
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   p, ok := s.proofs[strings.TrimSpace(id)]
	   if !ok {
		  return Proof{}, ErrNotFound
	  }
	   return p.Copy(), nil
  }
