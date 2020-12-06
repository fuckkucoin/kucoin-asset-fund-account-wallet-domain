package kcfund

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/subacct"
	"github.com/gin-gonic/gin"
)

  func (a *API) subFreeze(c *gin.Context) {
	  uid, okU := a.requireUser(c)
	   if !okU {
		   return
	  }
	  svc := a.subs()
	  p, err := svc.OpenParent(subacct.OpenParentReq{UserID: uid, Label: "primary"})
	   if err != nil {
		  fail(c, 400, err.Error())
		  return
	   }
	  subID := strings.TrimSpace(c.Param("subUserId"))
	  action := strings.ToLower(strings.TrimSpace(c.Query("action")))
	   if action == "" {
		   action = "debit"
	   }
	   kids := svc.ListChildren(p.ID)
	   var child *subacct.Child
	   for i := range kids {
		  if kids[i].ID == subID || kids[i].UserID == subID {
			  cp := kids[i]
			  child = &cp
			   break
		  }
	  }
	   if child == nil {
		  fail(c, 404, "sub user not found")
		  return
	  }
	   prev, err := svc.PreviewFreeze(child.ID, action)
	   if err != nil {
		  fail(c, 400, err.Error())
		   return
	  }
	   ok(c, gin.H{
		  "parent": p.ID, "parent_status": p.Status, "child": child.ID,
		   "child_status": child.Status, "privileges": child.Privileges,
		   "preview": prev, "actions": subacct.FreezeActions(),
	  })
  }
