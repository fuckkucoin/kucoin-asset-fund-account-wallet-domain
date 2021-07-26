package idemstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	ErrConflict = errors.New("idempotency key conflict")
	ErrInflight = errors.New("idempotency key inflight")
	ErrNotFound = errors.New("idempotency key not found")
	ErrInvalidKey = errors.New("idempotency key invalid")
)

const (
	StatusInflight = "inflight"
	StatusDone   = "done"
	StatusFailed = "failed"
)

// Record 一条幂等登记。

type Record struct {
	Scope       string
	Key         string
	Fingerprint string
	RefID       string
	Status      string
	PayloadHash string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Store 持久化接口；接 SQL。

type Store interface {
	Begin(ctx context.Context, scope, key, fingerprint, payloadHash string) (*Record, error)
	Complete(ctx context.Context, scope, key, refID string) error
	Fail(ctx context.Context, scope, key, reason string) error
	Get(ctx context.Context, scope, key string) (*Record, error)
}

// Fingerprint 对业务意图字段做稳定摘要。

  func Fingerprint(parts ...string) string {
	  h := sha256.New()
	  for i, p := range parts {
		   if i > 0 {
			   _, _ = h.Write([]byte{0})
		  }
		  _, _ = h.Write([]byte(strings.TrimSpace(p)))
	  }
	  return hex.EncodeToString(h.Sum(nil))
  }

  func normalize(scope, key string) (string, string, error) {
	   scope = strings.TrimSpace(scope)
	   key = strings.TrimSpace(key)
	  if scope == "" || key == "" || len(key) > 128 {
		   return "", "", ErrInvalidKey
	  }
	  return scope, key, nil
   }

// Mem 仅供夹具。

type Mem struct {
	mu sync.Mutex
	m  map[string]*Record
}

  func NewMem() *Mem { return &Mem{m: map[string]*Record{}} }

   func memK(scope, key string) string { return scope + "\x00" + key }

  func (m *Mem) Begin(_ context.Context, scope, key, fingerprint, payloadHash string) (*Record, error) {
	   scope, key, err := normalize(scope, key)
	  if err != nil {
		   return nil, err
	  }
	  fingerprint = strings.TrimSpace(fingerprint)
	  if fingerprint == "" {
		  return nil, fmt.Errorf("%w: fingerprint", ErrInvalidKey)
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   k := memK(scope, key)
	  if cur, ok := m.m[k]; ok {
		  if cur.Fingerprint != fingerprint {
			  return nil, ErrConflict
		   }
		  switch cur.Status {
		   case StatusDone:
			  cp := *cur
			   return &cp, nil
		  case StatusInflight:
			  return nil, ErrInflight
		   case StatusFailed:
		  default:
			   return nil, ErrConflict
		  }
	   }
	   now := time.Now().UTC()
	  rec := &Record{
		  Scope: scope,
		  Key:   key, Fingerprint: fingerprint,
		   Status: StatusInflight, PayloadHash: strings.TrimSpace(payloadHash),
		   CreatedAt: now, UpdatedAt: now,
	  }
	   m.m[k] = rec
	  cp := *rec
	  return &cp, nil
   }

  func (m *Mem) Complete(_ context.Context, scope, key, refID string) error {
	  scope, key, err := normalize(scope, key)
	  if err != nil {
		   return err
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   cur,
		   ok := m.m[memK(scope, key)]
	  if !ok {
		   return ErrNotFound
	   }
	  cur.Status = StatusDone
	  cur.RefID = strings.TrimSpace(refID)
	   cur.UpdatedAt = time.Now().UTC()
	  return nil
   }

   func (m *Mem) Fail(_ context.Context, scope, key, _ string) error {
	   scope, key, err := normalize(scope, key)
	  if err != nil {
		  return err
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  cur, ok := m.m[memK(scope, key)]
	   if !ok {
		   return ErrNotFound
	   }
	  cur.Status = StatusFailed
	   cur.UpdatedAt = time.Now().UTC()
	  return nil
   }

   func (m *Mem) Get(_ context.Context, scope, key string) (*Record, error) {
	   scope, key, err := normalize(scope, key)
	  if err != nil {
		  return nil, err
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   cur, ok := m.m[memK(scope, key)]
	   if !ok {
		  return nil, ErrNotFound
	   }
	  cp := *cur
	   return &cp, nil
   }
