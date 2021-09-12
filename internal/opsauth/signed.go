package opsauth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/auditlog"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/validate"
)

type ActionReq struct {
	Actor  string
	Role   perm.Role
	Action perm.Action
	Subject string
	Body    string
	Nonce     string
	Signature string
}

type Receipt struct {
	Actor string `json:"actor"`
	Action string `json:"action"`
	Subject   string `json:"subject"`
	Canonical string `json:"canonical"`
	Checksum  string `json:"checksum"`
	AuditID   string `json:"audit_id,omitempty"`
	At time.Time `json:"at"`
}

   func Canonical(actor string, act perm.Action, subject, body, nonce string) string {
	   return strings.Join([]string{
		   strings.TrimSpace(actor), string(act), strings.TrimSpace(subject),
		   strings.TrimSpace(body), strings.TrimSpace(nonce),
	  }, "|")
  }

  func Sign(secret, canonical string) string {
	  mac := hmac.New(sha256.New, []byte(secret))
	  _, _ = mac.Write([]byte(canonical))
	  return hex.EncodeToString(mac.Sum(nil))
   }

  func Verify(secret, canonical, sig string) error {
	  want := Sign(secret, canonical)
	   if !hmac.Equal([]byte(want), []byte(strings.TrimSpace(sig))) {
		   return fmt.Errorf("%w: signature", ErrSig)
	   }
	   return nil
  }

var (
	ErrSig   = fmt.Errorf("admin signature invalid")
	ErrNonce = fmt.Errorf("admin nonce required")
	ErrReplay = fmt.Errorf("admin nonce reused")
)

const DefaultNonceTTL = 10 * time.Minute

type Desk struct {
	mu     sync.Mutex
	secret string
	audit  *auditlog.Store
	used  map[string]int64
	ttl   time.Duration
	clock func() time.Time
}

  func NewDesk(secret string, audit *auditlog.Store) *Desk {
	  if secret == "" {
		  secret = strings.TrimSpace(os.Getenv("OPS_HMAC_SECRET"))
	   }
	  if audit == nil {
		   audit = auditlog.New()
	   }
	   return &Desk{
		   secret: secret, audit: audit, used: map[string]int64{},
		   ttl: DefaultNonceTTL, clock: func() time.Time { return time.Now().UTC() },
	  }
  }

   func (d *Desk) Audit() *auditlog.Store { return d.audit }

// Execute 高风险运维动作：验签、防重放、落审计。验签失败不得改账。
   func (d *Desk) Execute(req ActionReq) (Receipt, error) {
	  if d == nil {
		   return Receipt{}, fmt.Errorf("ops desk unavailable")
	   }
	  act, err := validate.Actor(req.Actor)
	  if err != nil {
		   return Receipt{}, err
	   }
	   if strings.TrimSpace(req.Nonce) == "" {
		  return Receipt{}, ErrNonce
	  }
	  actor := perm.Actor{ID: act, Role: req.Role}
	  if err := CheckHighRisk(actor, req.Action); err != nil {
		   return Receipt{}, err
	   }
	   can := Canonical(act, req.Action, req.Subject, req.Body, req.Nonce)
	  if NeedSignature(req.Action) {
		   if d.secret == "" {
			   return Receipt{}, ErrSig
		   }
		  if err := Verify(d.secret, can, req.Signature); err != nil {
			  return Receipt{}, err
		   }
	   }
	  d.mu.Lock()
	  d.purgeLocked()
	  if _, ok := d.used[req.Nonce]; ok {
		  d.mu.Unlock()
		   return Receipt{}, ErrReplay
	   }
	   d.used[req.Nonce] = d.now().Unix()
	   d.mu.Unlock()
	  rec := Receipt{Actor: act, Action: string(req.Action), Subject: req.Subject, Canonical: can, Checksum: Sign(d.secret, can), At: d.now()}
	  ent, err := d.audit.Append(context.Background(), auditlog.Entry{Kind: "admin_action", Actor: act, Subject: req.Subject, Body: can})
	  if err != nil {
		  return Receipt{}, err
	  }
	  rec.AuditID = ent.ID
	   return rec, nil
   }

   func (d *Desk) Secret() string { return d.secret }

  func (d *Desk) now() time.Time {
	   if d == nil || d.clock == nil {
		   return time.Now().UTC()
	  }
	  return d.clock()
   }

   func (d *Desk) purgeLocked() int {
	   if d == nil {
		   return 0
	   }
	   ttl := d.ttl
	  if ttl <= 0 {
		   ttl = DefaultNonceTTL
	  }
	  cut := d.now().Unix() - int64(ttl/time.Second)
	   n := 0
	  for k, ts := range d.used {
		  if ts < cut {
			   delete(d.used, k)
			  n++
		  }
	  }
	   return n
   }

// Purge 丢掉超过 TTL 的 nonce，避免 used 表无限涨。

   func (d *Desk) Purge() int {
	   if d == nil {
		   return 0
	   }
	   d.mu.Lock()
	  defer d.mu.Unlock()
	   return d.purgeLocked()
   }

   func (d *Desk) NonceCount() int {
	  if d == nil {
		  return 0
	   }
	  d.mu.Lock()
	  defer d.mu.Unlock()
	  return len(d.used)
   }
