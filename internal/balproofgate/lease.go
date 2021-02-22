package balproofgate

import (
	"context"
	"strings"
	"time"
	"github.com/google/uuid"
)

// LeaseFSM 租约状态机。
type LeaseFSM struct {
	store Store
	gen *GenerationEngine
	clock func() time.Time
}

   func newLeaseFSM(store Store, gen *GenerationEngine, clock func() time.Time) *LeaseFSM {
	  if clock == nil {
		   clock = func() time.Time { return time.Now().UTC() }
	}
	return &LeaseFSM{store: store, gen: gen, clock: clock}
}

// FindIdempotent 查找同 nonce 未终态租约。

   func (f *LeaseFSM) FindIdempotent(ctx context.Context, userID, book, asset, nonce string) (*ProofLease, error) {
	   if f == nil || f.store == nil {
		   return nil, ErrUnavailable
	   }
	   nonce = strings.TrimSpace(nonce)
	   if nonce == "" {
		   return nil, ErrNonceInvalid
	  }
	  userID,
		  book, asset, err := normKey(userID, book, asset)
	  if err != nil {
		   return nil, err
	   }
	  row, err := f.store.GetLeaseByNonce(ctx, userID, book, asset, nonce)
	  if err != nil {
		   return nil, err
	   }
	   if row == nil {
		  return nil, nil
	  }
	   now := f.clock()
	   if row.Terminal() {
		   return nil, nil
	   }
	   if !row.ExpiresAt.IsZero() && !now.Before(row.ExpiresAt) {
		  return nil, nil
	   }
	  cp := row.Copy()
	  return &cp, nil
  }

// IssueNew 抬世代并落 issued→bound 租约。调用方须先做幂等查找。
  func (f *LeaseFSM) IssueNew(ctx context.Context, req IssueReq, snap LiveSnapshotRoot, secret string) (*ProofLease, error) {
	  if f == nil || f.store == nil || f.gen == nil {
		   return nil, ErrUnavailable
	  }
	   if strings.TrimSpace(secret) == "" {
		  return nil, ErrSecret
	  }
	  userID, book, asset, err := normKey(req.UserID, req.Book, req.Asset)
	  if err != nil {
		  return nil, err
	   }
	   nonce := strings.TrimSpace(req.Nonce)
	  if nonce == "" || len(nonce) > 128 {
		  return nil, ErrNonceInvalid
	  }
	  if err := RejectPorOrStmtField(nonce); err != nil {
		   return nil, err
	   }
	  cur,
		  err := f.gen.Current(ctx, userID, book, asset)
	   if err != nil {
		   return nil, err
	   }
	   next, err := f.gen.Bump(ctx, userID, book, asset, cur)
	   if err != nil {
		   if err == ErrStaleGen {
			  return nil, ErrConcurrentIssue
		   }
		   return nil, err
	   }
	  now := f.clock()
	  ttl := DefaultLeaseTTL
	  if req.TTLSec > 0 {
		   ttl = ClampTTL(time.Duration(req.TTLSec) * time.Second)
	  }
	  can := LeaseCanonical(userID, book, asset, nonce, snap, now, next)
	  lease := &ProofLease{
		   ID: uuid.NewString(),
		   UserID: userID,
		  Book: book,
		  Asset:      asset,
		   Nonce:      nonce,
		  Generation: next,
		  Status:     LeaseIssued,
		   Snapshot:  snap,
		   Canonical: can,
		  Signature: SignHMAC(secret, can),
		   IssuedAt:      now,
		  ExpiresAt:     now.Add(ttl),
		  IsolationMark: IsolationMark,
	   }
	   lease.ProofLeaseID = lease.ID
	  if err := f.store.SaveLease(ctx, lease); err != nil {
		  return nil, err
	  }
	   bound := now
	   lease.Status = LeaseBound
	  lease.BoundAt = &bound
	   if err := f.store.SaveLease(ctx, lease); err != nil {
		  return nil, err
	  }
	   cp := lease.Copy()
	   return &cp, nil
  }

// BindIssued 将残留 issued 推进 bound（worker 补偿）。

   func (f *LeaseFSM) BindIssued(ctx context.Context, leaseID string) (*ProofLease, error) {
	  l, err := f.load(ctx, leaseID)
	   if err != nil {
		  return nil, err
	   }
	  if l.Status != LeaseIssued {
		  return nil, ErrLeaseNotBound
	   }
	   now := f.clock()
	   l.Status = LeaseBound
	  l.BoundAt = &now
	   if err := f.store.SaveLease(ctx, l); err != nil {
		  return nil, err
	   }
	   cp := l.Copy()
	   return &cp, nil
  }

// Consume 闸消费：仅 bound 且未过期。

   func (f *LeaseFSM) Consume(ctx context.Context, leaseID string) (*ProofLease, error) {
	  l, err := f.load(ctx, leaseID)
	   if err != nil {
		  return nil, err
	   }
	   now := f.clock()
	  if l.Status == LeaseExpired {
		   return nil, ErrLeaseExpired
	   }
	  if l.Status == LeaseRevoked {
		  return nil, ErrRevoked
	   }
	   if l.Terminal() {
		  return nil, ErrLeaseTerminal
	  }
	   if !l.Consumable(now) {
		   if l.Status != LeaseBound {
			  return nil, ErrLeaseNotBound
		   }
		  return nil, ErrLeaseExpired
	  }
	   l.Status = LeaseConsumed
	   l.ConsumedAt = &now
	  if err := f.store.SaveLease(ctx, l); err != nil {
		  return nil, err
	   }
	  cp := l.Copy()
	   return &cp, nil
   }

// Expire 扫描推进过期。
   func (f *LeaseFSM) Expire(ctx context.Context, leaseID string) (*ProofLease, error) {
	   l, err := f.load(ctx, leaseID)
	  if err != nil {
		  return nil, err
	  }
	  if l.Terminal() {
		   cp := l.Copy()
		   return &cp, nil
	   }
	  now := f.clock()
	  if now.Before(l.ExpiresAt) {
		   cp := l.Copy()
		   return &cp, nil
	   }
	  l.Status = LeaseExpired
	  if err := f.store.SaveLease(ctx, l); err != nil {
		   return nil, err
	  }
	   cp := l.Copy()
	  return &cp, nil
   }

// Revoke 显式吊销。
  func (f *LeaseFSM) Revoke(ctx context.Context, leaseID, reason string) (*ProofLease, error) {
	   l, err := f.load(ctx, leaseID)
	   if err != nil {
		   return nil, err
	  }
	  if l.Status == LeaseConsumed {
		   return nil, ErrLeaseTerminal
	   }
	  if l.Status == LeaseRevoked {
		  cp := l.Copy()
		  return &cp, nil
	  }
	  now := f.clock()
	  l.Status = LeaseRevoked
	  l.RevokedAt = &now
	   l.RevokeReason = strings.TrimSpace(reason)
	  if err := f.store.SaveLease(ctx, l); err != nil {
		  return nil, err
	  }
	   cp := l.Copy()
	   return &cp, nil
  }

  func (f *LeaseFSM) load(ctx context.Context, leaseID string) (*ProofLease, error) {
	  if f == nil || f.store == nil {
		   return nil, ErrUnavailable
	   }
	   leaseID = strings.TrimSpace(leaseID)
	   if leaseID == "" {
		  return nil, ErrLeaseNotFound
	  }
	   l, err := f.store.GetLease(ctx, leaseID)
	  if err != nil {
		  return nil, err
	   }
	   if l == nil {
		  return nil, ErrLeaseNotFound
	  }
	  cp := l.Copy()
	   return &cp, nil
   }
