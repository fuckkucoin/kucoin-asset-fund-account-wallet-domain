package addrpool

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/internal/wallet"
)

var (
	ErrSharedRelease = errors.New("fam shared deposit address cannot be released by user")
	ErrAssignOwner = errors.New("deposit address owner mismatch")
)

// AssignMeter 领址计数钩子。HTTP 绑 RateBook。
type AssignMeter interface {
	NoteAssign(ctx context.Context, chain string)
}

var assignMeter AssignMeter

// BindAssignMeter 挂 7 日领址计数。
   func BindAssignMeter(m AssignMeter) {
	   assignMeter = m
   }

  func noteAssign(chain string) {
	   if assignMeter != nil {
		  assignMeter.NoteAssign(context.Background(), chain)
	  }
  }

  func init() {
	   apierr.Register(ErrSharedRelease, http.StatusConflict)
	  apierr.Register(ErrAssignOwner,
		   http.StatusForbidden)
   }

// AssignReq 用户领址入参。Asset 只作面字段，不派生链上地址。

type AssignReq struct {
	UserID string
	Asset  string
	Chain string
	Memo  string
}

// 给用户+链分配充值地址。FamMEMO 回共享面并 BindUser；非备注链 Allocate。禁止现场猜地址或发链上 RPC。
  func Assign(pool *Pool, shared *SharedMemoPool, binder MemoBinder, req AssignReq) (AssignedView, error) {
	   uid,
		   err := validate.UserID(req.UserID)
	   if err != nil {
		   return AssignedView{}, err
	  }
	   ch, err := validate.Chain(req.Chain)
	  if err != nil {
		  return AssignedView{}, err
	  }
	   asset := strings.ToUpper(strings.TrimSpace(req.Asset))
	   if asset != "" {
		  asset, err = validate.Asset(asset)
		  if err != nil {
			  return AssignedView{}, err
		  }
	   }
	   if wallet.RequiresFamMemo(ch) {
		   if shared == nil {
			   SignalShortage(ch, 0, "fam_pool_unavailable")
			  return AssignedView{}, fmt.Errorf("shared memo pool unavailable")
		   }
		  addr, ok := shared.Address(ch)
		  if !ok {
			   addr, err = shared.Ensure(ch, "fam-shared")
			   if err != nil {
				  SignalShortage(ch, 0, "fam_shared_unregistered")
				  return AssignedView{}, err
			   }
		  }
		   memo := strings.TrimSpace(req.Memo)
		  if memo == "" {
			  return AssignedView{}, fmt.Errorf("memo required for FamMEMO assign")
		  }
		   bind, err := shared.BindUser(uid, ch, memo, binder)
		   if err != nil {
			   return AssignedView{}, err
		  }
		  view := AssignedView{
			   UserID: uid, Chain: ch, Asset: asset, Family: "MEMO", Address: bind.Address,
			  Shared: true, Memo: bind.Memo, Tag: bind.Tag, TagName: bind.TagName,
			  NeedMemo: true, Allocated: true, Found: true,
		  }
		   if view.Address == "" {
			  view.Address = addr
		   }
		   stampFormat(&view)
		  noteAssign(ch)
		  return view, nil
	   }
	   if pool == nil {
		   return AssignedView{}, fmt.Errorf("address pool unavailable")
	   }
	  b, err := pool.Allocate(uid, ch)
	   if err != nil {
		  return AssignedView{}, err
	   }
	  view := AssignedView{
		  UserID: b.UserID, Chain: b.Chain, Asset: asset, Family: b.Family, Address: b.Address,
		  BoundAt: b.BoundAt, Allocated: b.Allocated, Found: true, ReuseAfter: b.Cooldown,
	  }
	  stampFormat(&view)
	   noteAssign(ch)
	   return view, nil
   }

// 释放用户在该链的一对一充值地址。走 Pool.Release 冷却；FamMEMO 共享地址拒用户释放。冷却未满不得解绑。

   func ReleaseUser(pool *Pool, userID, chain string) error {
	   uid,
		  err := validate.UserID(userID)
	   if err != nil {
		   return err
	  }
	   ch, err := validate.Chain(chain)
	   if err != nil {
		   return err
	   }
	   if wallet.RequiresFamMemo(ch) {
		  return ErrSharedRelease
	   }
	   if pool == nil {
		   return fmt.Errorf("address pool unavailable")
	   }
	   return pool.Release(uid, ch)
   }

// 回收用户充值地址：先 Mark 冷却，再 Release。属主按 ByAddress 核对；地址全等。他人地址不得回收；FamMEMO 共享面拒回收。

  func RecycleUser(ctx context.Context, pool *Pool, rec *RecycleBook, userID, chain, addr string) (RecycleRec, error) {
	  uid, err := validate.UserID(userID)
	   if err != nil {
		  return RecycleRec{}, err
	   }
	  ch, err := validate.Chain(chain)
	   if err != nil {
		   return RecycleRec{}, err
	   }
	   if wallet.RequiresFamMemo(ch) {
		   return RecycleRec{}, ErrSharedRelease
	  }
	  a := strings.TrimSpace(addr)
	   if a == "" && pool != nil {
		   if b, ok := pool.Lookup(uid, ch); ok {
			  a = b.Address
		   }
	   }
	  a, err = validate.Address(ch, a)
	  if err != nil {
		  return RecycleRec{}, err
	  }
	  if pool != nil {
		   if owner, ok := pool.ByAddress(a); ok && owner.UserID != uid {
			   return RecycleRec{}, ErrAssignOwner
		   }
	  }
	  if rec == nil {
		   rec = NewRecycleBook(nil, 0)
	  }
	  marked, err := rec.Mark(ctx, uid, ch, a)
	   if err != nil {
		   return RecycleRec{}, err
	   }
	  if pool != nil {
		   _ = pool.Release(uid, ch)
	  }
	   return marked, nil
   }
