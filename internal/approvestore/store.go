package approvestore

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/custody"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
	"github.com/google/uuid"
)

type Rec struct {
	Approval custody.Approval `json:"approval"`
}

type Store struct {
	kv  persist.KV
	sql *persist.SQLDB
}

  func New(kv persist.KV) *Store {
	  if kv == nil {
		   kv = persist.NewMemKV()
	  }
	   return &Store{kv: kv}
   }

  func NewMem() *Store { return New(nil) }

const kindAppr = "approval"

   func (s *Store) load(ctx context.Context, id string) (Rec, persist.Fence, error) {
	  if s.sqlOn() {
		   got, fence, err := s.loadSQL(ctx, id)
		  if err == nil {
			  return got, fence, nil
		   }
		   if !errors.Is(err, sqlrepo.ErrNotFound) && !errors.Is(err, sqlrepo.ErrUnavailable) {
			  return Rec{}, persist.Fence{}, err
		   }
	  }
	   row, err := s.kv.Get(ctx, kindAppr, id)
	   if err != nil {
		   return Rec{}, persist.Fence{}, err
	  }
	  var r Rec
	  if err := json.Unmarshal(row.Body, &r); err != nil {
		   return Rec{}, persist.Fence{}, err
	  }
	   if s.sqlOn() {
		   _ = s.saveSQL(ctx,
			  id, r, row.Fence)
	  }
	   return r, row.Fence, nil
   }

   func (s *Store) save(ctx context.Context, id string, r Rec, expected persist.Fence) error {
	   r.Approval.UpdatedAt = time.Now().UTC()
	  body, err := json.Marshal(r)
	   if err != nil {
		   return err
	   }
	   if s.sqlOn() {
		   if err := s.saveSQL(ctx, id, r, expected); err != nil {
			   return err
		   }
	   }
	   return s.kv.Put(ctx, persist.Row{Kind: kindAppr, Key: id, Body: body}, expected)
   }

// Open 开一条 sweep/sign/ceremony。system / 空 actor 拒。

   func (s *Store) Open(ctx context.Context, kind custody.ApprovalKind, actor, movementID, chain, to, amount, note string) (*custody.Approval, error) {
	   actor = strings.TrimSpace(actor)
	   if actor == "" || strings.EqualFold(actor, "system") {
		  return nil, custody.ErrSameActor
	  }
	  if kind != custody.KindSweep && kind != custody.KindSign && kind != custody.KindCeremony && kind != custody.KindRotate {
		   return nil, custody.ErrIllegalApproval
	  }
	   now := time.Now().UTC()
	  a := custody.Approval{
		  ID: uuid.NewString(),
		  Kind: kind, MovementID: movementID, Chain: chain, To: to, Amount: amount,
		  RequestedBy: actor, State: custody.ApprovalOpen, Note: note, CreatedAt: now, UpdatedAt: now,
	  }
	   if err := s.save(ctx, a.ID, Rec{Approval: a}, persist.Fence{}); err != nil {
		  return nil, err
	  }
	  got, _, err := s.load(ctx, a.ID)
	  if err != nil {
		   return nil, err
	   }
	   cp := got.Approval
	  return &cp, nil
   }

// ApproveFenced 复核。申请人不能批自己；token 对不上不得改状态。
  func (s *Store) ApproveFenced(ctx context.Context, id, actor string, allow bool, epoch int64, token string) (*custody.Approval, error) {
	  actor = strings.TrimSpace(actor)
	   if actor == "" || strings.EqualFold(actor, "system") {
		  return nil, custody.ErrSameActor
	   }
	   var out *custody.Approval
	   err := s.kv.InTx(ctx, func(ctx context.Context) error {
		   cur, fence, err := s.load(ctx, id)
		  if err != nil {
			   if errors.Is(err, persist.ErrNotFound) {
				   return custody.ErrApprovalNotFound
			  }
			   return err
		  }
		   if !fence.Match(epoch, token) {
			  return persist.ErrStaleFence
		   }
		  if cur.Approval.State != custody.ApprovalOpen {
			  return custody.ErrApprovalClosed
		   }
		   if strings.EqualFold(actor, cur.Approval.RequestedBy) {
			  return custody.ErrSameActor
		   }
		  cur.Approval.ApprovedBy = actor
		  if allow {
			   cur.Approval.State = custody.ApprovalApproved
		   } else {
			  cur.Approval.State = custody.ApprovalDenied
		  }
		  if err := s.save(ctx, id, cur, fence); err != nil {
			   return err
		  }
		  got, _, err := s.load(ctx, id)
		  if err != nil {
			  return err
		   }
		  cp := got.Approval
		   out = &cp
		   return nil
	   })
	  return out, err
  }

// MarkExecuted 必须已批准，执行人不是申请人，围栏对得上。
   func (s *Store) MarkExecuted(ctx context.Context, id, actor string, epoch int64, token string) (*custody.Approval, error) {
	   var out *custody.Approval
	   err := s.kv.InTx(ctx, func(ctx context.Context) error {
		  cur, fence, err := s.load(ctx, id)
		  if err != nil {
			   if errors.Is(err, persist.ErrNotFound) {
				  return custody.ErrApprovalNotFound
			   }
			   return err
		   }
		   if !fence.Match(epoch, token) {
			  return persist.ErrStaleFence
		  }
		   if cur.Approval.State != custody.ApprovalApproved {
			   return custody.ErrNotApproved
		  }
		  if strings.EqualFold(strings.TrimSpace(actor), cur.Approval.RequestedBy) {
			   return custody.ErrSameActor
		   }
		  cur.Approval.State = custody.ApprovalExecuted
		   if err := s.save(ctx, id, cur, fence); err != nil {
			  return err
		  }
		   got, _, err := s.load(ctx, id)
		  if err != nil {
			   return err
		   }
		  cp := got.Approval
		  out = &cp
		  return nil
	  })
	   return out, err
   }

  func (s *Store) Get(ctx context.Context, id string) (*custody.Approval, persist.Fence, error) {
	   cur, fence, err := s.load(ctx, id)
	   if err != nil {
		   if errors.Is(err, persist.ErrNotFound) {
			   return nil, persist.Fence{}, custody.ErrApprovalNotFound
		  }
		  return nil, persist.Fence{}, err
	  }
	  cp := cur.Approval
	   return &cp, fence, nil
  }
