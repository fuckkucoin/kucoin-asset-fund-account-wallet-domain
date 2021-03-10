package casestore

import (
	"context"
	"encoding/json"
	"errors"
	"time"
	"account-assets.kucoin.net/funding/internal/cases"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
	"github.com/google/uuid"
)

type Repo struct {
	kv  persist.KV
	sql *persist.SQLDB
}

   func New(kv persist.KV) *Repo {
	  if kv == nil {
		   kv = persist.NewMemKV()
	   }
	  return &Repo{kv: kv}
   }

  func NewMem() *Repo { return New(persist.NewMemKV()) }

const kindCase = "case"

type rec struct {
	Case  cases.Case         `json:"case"`
	Audit []cases.AuditEvent `json:"audit"`
}

  func (r *Repo) load(ctx context.Context, movementID string) (rec, persist.Fence, error) {
	  if r.sqlOn() {
		   got,
			  fence, err := r.loadSQL(ctx, movementID)
		   if err == nil {
			  return got, fence, nil
		  }
		  if !errors.Is(err, sqlrepo.ErrNotFound) && !errors.Is(err, sqlrepo.ErrUnavailable) {
			   return rec{},
				  persist.Fence{}, err
		   }
	  }
	   row, err := r.kv.Get(ctx, kindCase, movementID)
	   if err != nil {
		   return rec{}, persist.Fence{}, err
	  }
	  var out rec
	   if err := json.Unmarshal(row.Body, &out); err != nil {
		  return rec{},
			   persist.Fence{}, err
	  }
	   out.Case.FenceEpoch = row.Fence.Epoch
	  out.Case.FenceToken = row.Fence.Token
	  if r.sqlOn() {
		   _ = r.saveSQL(ctx, out, row.Fence)
	  }
	   return out, row.Fence, nil
   }

   func (r *Repo) save(ctx context.Context, movementID string, cur rec, expected persist.Fence) error {
	  cur.Case.UpdatedAt = time.Now().UTC()
	  body, err := json.Marshal(cur)
	  if err != nil {
		  return err
	   }
	   if r.sqlOn() {
		  f := persist.Fence{Epoch: cur.Case.FenceEpoch,
			   Token: cur.Case.FenceToken}
		   if f.Epoch == 0 {
			   f = expected
		  }
		   if err := r.saveSQL(ctx, cur, f); err != nil {
			  return err
		   }
	  }
	  return r.kv.Put(ctx, persist.Row{Kind: kindCase, Key: movementID, Body: body}, expected)
   }

// Open 同一 movement 已有未结案则返回原案 + ErrExists。新建盖 epoch=1。

  func (r *Repo) Open(ctx context.Context, movementID, userID, kind, reason string, policyVer int) (*cases.Case, error) {
	   if r == nil || r.kv == nil {
		   return nil, persist.ErrTxClosed
	   }
	  var out *cases.Case
	  err := r.kv.InTx(ctx, func(ctx context.Context) error {
		  cur,
			   fence, err := r.load(ctx, movementID)
		  if err == nil && cur.Case.Open() {
			   cp := cur.Case
			   out = &cp
			  return cases.ErrExists
		   }
		   if err != nil && !errors.Is(err, persist.ErrNotFound) {
			  return err
		  }
		   now := time.Now().UTC()
		  c := cases.Case{
			  ID: uuid.NewString(), MovementID: movementID, UserID: userID, Kind: kind,
			  Reason: reason,
			   State:  cases.StateOpen, PolicyVer: policyVer, CreatedAt: now, UpdatedAt: now,
		   }
		   rec := rec{Case: c, Audit: []cases.AuditEvent{{
			  ID: uuid.NewString(), CaseID: c.ID, Actor: "system", To: cases.StateOpen, Note: reason, At: now,
		   }}}
		  expected := persist.Fence{}
		   if err == nil {
			  expected = fence
		  }
		  if err := r.save(ctx, movementID, rec, expected); err != nil {
			  return err
		   }
		   got, _, err := r.load(ctx, movementID)
		  if err != nil {
			   return err
		  }
		   cp := got.Case
		   out = &cp
		   return nil
	   })
	   return out, err
  }

   func (r *Repo) Get(ctx context.Context, movementID string) (*cases.Case, error) {
	  cur, _, err := r.load(ctx, movementID)
	  if err != nil {
		   if errors.Is(err, persist.ErrNotFound) {
			  return nil, cases.ErrNotFound
		  }
		  return nil, err
	   }
	  cp := cur.Case
	  return &cp, nil
  }

// TransitionFenced 必须带当前围栏。失败整笔回滚，审计也不落。

   func (r *Repo) TransitionFenced(ctx context.Context, movementID, actor string, ev cases.Event, epoch int64, token string) (*cases.Case, error) {
	   var out *cases.Case
	  err := r.kv.InTx(ctx, func(ctx context.Context) error {
		   cur, fence, err := r.load(ctx, movementID)
		  if err != nil {
			  if errors.Is(err, persist.ErrNotFound) {
				  return cases.ErrNotFound
			  }
			   return err
		   }
		  if !fence.Match(epoch, token) {
			  return cases.ErrStaleFence
		  }
		   if !cur.Case.Open() {
			  return cases.ErrClosed
		  }
		  next, err := cases.Apply(cur.Case.State, ev)
		  if err != nil {
			   return err
		   }
		   prev := cur.Case.State
		  cur.Case.State = next
		   switch ev {
		  case cases.EventAssign:
			  cur.Case.Assignee = actor
		   case cases.EventApprove, cases.EventDeny:
			  cur.Case.DecidedBy = actor
		  }
		  cur.Audit = append(cur.Audit,
			   cases.AuditEvent{
				  ID:     uuid.NewString(),
				  CaseID: cur.Case.ID, Actor: actor,
				  Event: string(ev), From: prev, To: next, At: time.Now().UTC(),
			   })
		   if err := r.save(ctx, movementID, cur, fence); err != nil {
			   return err
		  }
		  got, _, err := r.load(ctx, movementID)
		  if err != nil {
			   return err
		  }
		   cp := got.Case
		   out = &cp
		  return nil
	  })
	  return out, err
   }

   func (r *Repo) Audit(ctx context.Context, movementID string) ([]cases.AuditEvent, error) {
	  cur,
		  _, err := r.load(ctx, movementID)
	  if err != nil {
		   return nil, err
	   }
	   return cur.Audit, nil
   }
