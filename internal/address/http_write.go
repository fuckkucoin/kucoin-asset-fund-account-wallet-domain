package address

import (
	"context"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/addrpool"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/fundsurf"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

   func registerWrite(v1 *gin.RouterGroup, rt *app.Runtime, svc *Service) {
	  if v1 == nil || svc == nil {
		   return
	   }
	   id := readID(rt)
	   v1.PUT("/addresses/book", func(c *gin.Context) {
		  var body BookWrite
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  uid, err := callerid.Bind(c, body.UserID, id)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   body.UserID = uid
		   body.Actor = uid
		   e,
			   err := svc.UpdateBook(c.Request.Context(), body)
		  if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		   syncDestAllow(rt, e, e.Disabled)
		   c.JSON(200, svc.decorate(c.Request.Context(), e))
	  })
	  v1.POST("/addresses/book/restore", func(c *gin.Context) {
		  var body BookWrite
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   uid, err := callerid.Bind(c, body.UserID, id)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  e, err := svc.RestoreBook(c.Request.Context(), uid, body.Chain, body.Address, uid)
		   if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		   }
		  syncDestAllow(rt, e, false)
		   c.JSON(200, svc.decorate(c.Request.Context(), e))
	   })
	   v1.POST("/addresses/book/enable", func(c *gin.Context) {
		  var body BookWrite
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  uid,
			  err := callerid.Bind(c, body.UserID, id)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   e, err := svc.EnableBook(c.Request.Context(), uid, body.Chain, body.Address, uid)
		  if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		   }
		  syncDestAllow(rt, e, false)
		  c.JSON(200, svc.decorate(c.Request.Context(), e))
	   })
	  v1.POST("/addresses/book/disable", func(c *gin.Context) {
		   var body BookWrite
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  uid, err := callerid.Bind(c, body.UserID, id)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   e, err := svc.DisableBook(c.Request.Context(), uid, body.Chain, body.Address, uid)
		   if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		  syncDestAllow(rt, e, true)
		   c.JSON(200,
			   svc.decorate(c.Request.Context(), e))
	   })
	   v1.POST("/addresses/book/purge", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body BookWrite
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		  uid, err := callerid.Bind(c, body.UserID, id)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   if err := svc.RemoveBook(c.Request.Context(), uid, body.Chain, body.Address); err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		   if rt != nil && rt.DestAllow != nil {
			  _ = rt.DestAllow.RemoveExact(uid, body.Chain, body.Address)
		   }
		  c.JSON(200, gin.H{"ok": true, "purged": true})
	  })
	   v1.POST("/addresses/assign", func(c *gin.Context) {
		   var body struct {
			  UserID string `json:"user_id"`
			   Asset  string `json:"asset"`
			   Chain string `json:"chain"`
			   Memo string `json:"memo"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		   }
		  uid, err := callerid.Bind(c, body.UserID, id)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   memo := strings.TrimSpace(body.Memo)
		  if memo == "" {
			  memo = fundsurf.MemoFor(uid, body.Chain)
		  }
		  view, err := svc.AssignDeposit(uid, body.Asset, body.Chain, memo)
		   if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(200, view)
	   })
	  v1.POST("/addresses/release", func(c *gin.Context) {
		  var body struct {
			  UserID string `json:"user_id"`
			  Chain string `json:"chain"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		  uid, err := callerid.Bind(c, body.UserID, id)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  if err := svc.ReleaseDeposit(uid, body.Chain); err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		   }
		  c.JSON(200, gin.H{"ok": true, "user_id": uid, "chain": strings.ToUpper(strings.TrimSpace(body.Chain))})
	  })
	   v1.POST("/addresses/recycle", func(c *gin.Context) {
		   var body struct {
			  UserID string `json:"user_id"`
			   Chain  string `json:"chain"`
			  Address string `json:"address"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  uid, err := callerid.Bind(c, body.UserID, id)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   got, err := svc.RecycleDeposit(c.Request.Context(), uid, body.Chain, body.Address)
		  if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(200, got)
	   })
	  v1.POST("/addresses/reclaim", func(c *gin.Context) {
		  var body struct {
			  UserID string `json:"user_id"`
			   Chain string `json:"chain"`
			   Address string `json:"address"`
		   }
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  uid,
			  err := callerid.Bind(c, body.UserID, id)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  got, err := svc.ReclaimDeposit(c.Request.Context(), uid, body.Chain, body.Address)
		  if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		  c.JSON(200, got)
	   })
	   v1.POST("/addresses/book/import", func(c *gin.Context) {
		   var body ImportReq
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  uid, err := callerid.Bind(c, body.UserID, id)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   body.UserID = uid
		   for i := range body.Rows {
			   if strings.TrimSpace(body.Rows[i].UserID) == "" {
				  body.Rows[i].UserID = uid
			  }
			  body.Rows[i].Actor = uid
		  }
		   rep, err := svc.ImportBook(c.Request.Context(), body)
		   if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error(), "report": rep})
			   return
		  }
		  if body.Commit {
			   for _, e := range rep.Accepted {
				   syncDestAllow(rt, e.AddressBookEntry, false)
			   }
		  }
		  c.JSON(200, rep)
	  })
	   v1.POST("/addresses/book/transfer", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body TransferReq
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  if strings.TrimSpace(body.Actor) == "" {
			   body.Actor = "ops"
		   }
		  rec, err := svc.TransferBook(c.Request.Context(), body)
		  if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			  return
		  }
		  syncDestAllow(rt, rec.From, true)
		   syncDestAllow(rt, rec.To, false)
		  c.JSON(200, rec)
	   })
	   v1.POST("/addresses/book/batch", func(c *gin.Context) {
		   var body struct {
			   UserID string      `json:"user_id"`
			   Rows   []BookWrite `json:"rows"`
		  }
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		   uid, err := callerid.Bind(c, body.UserID, id)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   for i := range body.Rows {
			   if strings.TrimSpace(body.Rows[i].UserID) == "" {
				   body.Rows[i].UserID = uid
			   }
			   body.Rows[i].Actor = uid
		  }
		  rep,
			  err := svc.AddBookBatch(c.Request.Context(), body.Rows)
		  if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error(), "report": rep})
			  return
		   }
		   for _, e := range rep.Accepted {
			   syncDestAllow(rt, e.AddressBookEntry, false)
		   }
		   c.JSON(200, rep)
	   })
	   v1.POST("/addresses/book/touch", func(c *gin.Context) {
		   var body BookWrite
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		   uid, err := callerid.Bind(c, body.UserID, id)
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   rec, err := svc.TouchLastUsed(c.Request.Context(), uid, body.Chain, body.Address, body.IdemKey)
		   if err != nil {
			   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		  }
		   c.JSON(200, rec)
	  })
	  v1.POST("/addresses/assign-sticky", func(c *gin.Context) {
		   var body struct {
			  UserID string `json:"user_id"`
			   Asset string `json:"asset"`
			   Chain string `json:"chain"`
			   Memo string `json:"memo"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		   uid,
			  err := callerid.Bind(c, body.UserID, id)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  memo := strings.TrimSpace(body.Memo)
		   if memo == "" {
			   memo = fundsurf.MemoFor(uid, body.Chain)
		   }
		  view, err := svc.AssignDepositSticky(uid, body.Asset, body.Chain, memo)
		  if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		   }
		   c.JSON(200, view)
	   })
	  v1.POST("/addresses/assign-fair", func(c *gin.Context) {
		   var body struct {
			   UserID string `json:"user_id"`
			  Asset  string `json:"asset"`
			   Chain  string `json:"chain"`
			  Memo   string `json:"memo"`
			  Chains []string `json:"chains"`
		  }
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		   uid,
			  err := callerid.Bind(c, body.UserID, id)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		  memo := strings.TrimSpace(body.Memo)
		  if memo == "" {
			  memo = fundsurf.MemoFor(uid, body.Chain)
		   }
		  view, err := svc.AssignDepositFair(uid, body.Asset, body.Chain, memo, body.Chains)
		   if err != nil {
			  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
			   return
		   }
		  c.JSON(200, view)
	   })
   }

   func (s *Service) ReclaimDeposit(ctx context.Context, userID, chain, addr string) (addrpool.RecycleRec, error) {
	  rec := (*addrpool.RecycleBook)(nil)
	  if s != nil {
		   rec = s.rec
	   }
	  return addrpool.ReclaimUser(ctx, s.pool, rec, userID, chain, addr)
  }

   func (s *Service) AssignDepositSticky(userID,
	  asset, chain, memo string) (addrpool.AssignedView, error) {
	  if s == nil {
		   return addrpool.AssignedView{}, fmt.Errorf("address wallet unavailable")
	   }
	  var binder addrpool.MemoBinder
	   if s.attr != nil {
		  binder = s.attr
	  }
	  return addrpool.AssignSticky(s.pool, s.shared, binder, nil, addrpool.AssignReq{
		   UserID: userID, Asset: asset, Chain: chain, Memo: memo,
	  })
   }

  func (s *Service) AssignDepositFair(userID, asset, chain, memo string, chains []string) (addrpool.AssignedView, error) {
	  if s == nil {
		  return addrpool.AssignedView{}, fmt.Errorf("address wallet unavailable")
	   }
	   var binder addrpool.MemoBinder
	   if s.attr != nil {
		  binder = s.attr
	  }
	   return addrpool.AssignFair(s.pool, s.shared, binder, addrpool.AssignReq{
		   UserID: userID, Asset: asset, Chain: chain, Memo: memo,
	  }, chains)
  }

// 分配充值地址（一对一或 FamMEMO）。复用 addrpool.Assign；不编造链上 RPC。FamMEMO 无 memo 不得领址。

   func (s *Service) AssignDeposit(userID, asset, chain, memo string) (addrpool.AssignedView, error) {
	  if s == nil {
		   return addrpool.AssignedView{}, fmt.Errorf("address wallet unavailable")
	  }
	  var binder addrpool.MemoBinder
	  if s.attr != nil {
		  binder = s.attr
	  }
	   return addrpool.Assign(s.pool, s.shared, binder, addrpool.AssignReq{
		  UserID: userID, Asset: asset, Chain: chain, Memo: memo,
	   })
  }

// 释放一对一充值地址。走 Pool.Release 冷却。FamMEMO 共享地址拒用户释放。

   func (s *Service) ReleaseDeposit(userID, chain string) error {
	   return addrpool.ReleaseUser(s.pool, userID, chain)
  }

// 回收充值地址。Mark 冷却簿后再 Release。属主不符不得回收。

   func (s *Service) RecycleDeposit(ctx context.Context, userID, chain, addr string) (addrpool.RecycleRec, error) {
	   rec := (*addrpool.RecycleBook)(nil)
	  if s != nil {
		  rec = s.rec
	   }
	  return addrpool.RecycleUser(ctx, s.pool, rec, userID, chain, addr)
   }

   func syncDestAllow(rt *app.Runtime, e ledger.AddressBookEntry, drop bool) {
	   if rt == nil || rt.DestAllow == nil {
		  return
	   }
	  if drop || e.Disabled {
		   _ = rt.DestAllow.RemoveExact(e.UserID, e.Chain, e.Address)
		  return
	   }
	  _, _ = rt.DestAllow.AddExact(e.UserID, e.Chain, e.Address, e.Label)
   }
