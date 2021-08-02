// 公开面挂地址簿导入/异常分与 destallow 快照。属主取 requireUser；运营转移不走公开面。干跑不得入库；快照哈希不一致不得当已同步。

// Author: Rook
// Date: 2021.08.01

package kcfund

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/address"
	"github.com/gin-gonic/gin"
)

  func (a *API) importWithdrawAddresses(c *gin.Context) {
	  var body address.ImportReq
	  if err := c.ShouldBindJSON(&body); err != nil {
		  fail(c, 400, err.Error())
		   return
	   }
	   uid, okU := a.requireUser(c)
	   if !okU {
		  return
	   }
	  body.UserID = uid
	  for i := range body.Rows {
		   if strings.TrimSpace(body.Rows[i].UserID) == "" {
			   body.Rows[i].UserID = uid
		  }
		  body.Rows[i].Actor = uid
	   }
	  rep, err := a.bookSvc().ImportBook(c.Request.Context(), body)
	  if err != nil {
		   fail(c, 400, err.Error())
		  return
	  }
	   if body.Commit {
		   for _, e := range rep.Accepted {
			  a.syncBookDestAllow(e.AddressBookEntry, false)
		  }
	  }
	   ok(c, rep)
  }

   func (a *API) getAddressAnomaly(c *gin.Context) {
	   uid, okU := a.requireUser(c)
	   if !okU {
		   return
	   }
	   view, err := a.bookSvc().InspectAnomaly(c.Request.Context(), uid)
	  if err != nil {
		   fail(c, 400, err.Error())
		  return
	   }
	  ok(c, view)
   }

   func (a *API) destAllowSnapshot(c *gin.Context) {
	  uid, okU := a.requireUser(c)
	   if !okU {
		  return
	   }
	   if a.rt == nil || a.rt.DestAllow == nil {
		  ok(c, gin.H{"user_id": uid, "hash": "", "count": 0, "algo": "sha256"})
		   return
	  }
	   ok(c, a.rt.DestAllow.SnapshotHash(uid))
   }
