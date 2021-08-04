package kcfund

import (
	  "strings"

	   "account-assets.kucoin.net/funding/internal/subacct"
	   "github.com/gin-gonic/gin"
)

func (a *API) parentOfUser(uid string) (subacct.Parent, *subacct.Service, error) {
	  svc := a.subs()
	   p, err := svc.OpenParent(subacct.OpenParentReq{UserID: uid, Label: "primary"})
	   if err != nil {
	    	return subacct.Parent{}, svc, err
	    }
	  return p, svc, nil
}

  func childUnderParent(svc *subacct.Service, p subacct.Parent, subUserID string) (subacct.Child, bool) {
	    subUserID = strings.TrimSpace(subUserID)
	if svc == nil || subUserID == "" {
		return subacct.Child{}, false
	}
	   for _, k := range svc.ListChildren(p.ID) {
	  	if k.ID == subUserID || k.UserID == subUserID || k.Label == subUserID {
			return k, true
	    	}
	      }
	  return subacct.Child{}, false
}

  func (a *API) requireChild(c *gin.Context, uid, subUserID string) (subacct.Parent, subacct.Child, *subacct.Service, bool) {
	    p, svc, err := a.parentOfUser(uid)
	      if err != nil {
		fail(c, 400, err.Error())
		return p, subacct.Child{}, svc, false
	   }
	   ch, ok := childUnderParent(svc, p, subUserID)
	   if !ok {
	       	fail(c, 404, "sub-account not found")
	  	return p, subacct.Child{}, svc, false
	}
	    return p, ch, svc, true
}
