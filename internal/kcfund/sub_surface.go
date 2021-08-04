package kcfund

import (
	"io"
	"strings"

	"account-assets.kucoin.net/funding/internal/subacct"
	   "github.com/gin-gonic/gin"
)

func (a *API) freezeSubAccount(c *gin.Context) {
	uid, okU := a.requireUser(c)
	if !okU {
		return
	      }
	    body, hasFrozen, err := bindSubFreezeBody(c)
	if err != nil {
		fail(c, 400, err.Error())
	   	return
	  }
	if !hasFrozen {
		fail(c, 400, "frozen required")
	   	return
	    }
	   _, ch, svc, okC := a.requireChild(c, uid, c.Param("subUserId"))
	  if !okC {
		return
	    }
	var updated subacct.Child
	       if *body.Frozen {
	   	if strings.TrimSpace(body.Reason) == "" {
	   		fail(c, 400, "reason required when freezing")
			return
	     	}
		updated, err = svc.FreezeChild(ch.ID, body.Reason)
	   } else {
	   	updated, err = svc.UnfreezeChild(ch.ID)
	   }
	if err != nil {
	    	fail(c, 400, err.Error())
	   	return
	}
	ev := "UNFROZEN"
	   if *body.Frozen {
		ev = "FROZEN"
	}
	a.emitSubLifecycle(c.Request.Context(), ev, uid, updated.ID, updated.Label, map[string]any{"reason": body.Reason})
	ok(c, a.subAccountJSON(c, updated, ""))
}

   func (a *API) closeSubAccount(c *gin.Context) {
	  uid, okU := a.requireUser(c)
	    if !okU {
	    	return
	}
	_, ch, svc, okC := a.requireChild(c, uid, c.Param("subUserId"))
	if !okC {
	  	return
	   }
	     updated, err := svc.CloseChild(ch.ID)
	    if err != nil {
	  	fail(c, 400, err.Error())
	  	return
	}
	  a.emitSubLifecycle(c.Request.Context(), "CLOSED", uid, updated.ID, updated.Label, nil)
	     ok(c, a.subAccountJSON(c, updated, ""))
}

  func (a *API) updateSubAccountLabel(c *gin.Context) {
	  uid, okU := a.requireUser(c)
	        if !okU {
	   	return
	  }
	    var body struct {
		Label string `json:"label"`
	    }
	       if err := bindJSONOrEmpty(c, &body); err != nil && err != io.EOF {
		fail(c, 400, err.Error())
	  	return
	    }
	body.Label = strings.TrimSpace(firstNonEmpty(body.Label, bffParam(c, "label")))
	if body.Label == "" {
		fail(c, 400, "label required")
		return
	      }
	     _, ch, svc, okC := a.requireChild(c, uid, c.Param("subUserId"))
	  if !okC {
		return
	       }
	updated, err := svc.SetLabel(ch.ID, body.Label)
	if err != nil {
	    	fail(c, 400, err.Error())
	        	return
	}
	ok(c, a.subAccountJSON(c, updated, body.Label))
}

func (a *API) setSubAccountPrivileges(c *gin.Context) {
	uid, okU := a.requireUser(c)
	       if !okU {
	        	return
	    }
	   var body struct {
		Privileges []string `json:"privileges"`
	    }
	if err := bindJSONOrEmpty(c, &body); err != nil && err != io.EOF {
	   	fail(c, 400, err.Error())
	  	return
	}
	  if len(body.Privileges) == 0 {
	     	fail(c, 400, "privileges required")
	     	return
	    }
	  _, ch, svc, okC := a.requireChild(c, uid, c.Param("subUserId"))
	  if !okC {
		return
	}
	    privs := make([]subacct.Privilege, 0, len(body.Privileges))
	  for _, raw := range body.Privileges {
	       	p, err := subacct.ParsePrivilege(raw)
	   	if err != nil {
	  		fail(c, 400, err.Error())
			return
	    	}
	       	privs = append(privs, p)
	}
	  updated, err := svc.SetPrivileges(ch.ID, privs)
	   if err != nil {
		fail(c, 400, err.Error())
	   	return
	       }
	  a.emitSubLifecycle(c.Request.Context(), "PRIVILEGES_CHANGED", uid, updated.ID, updated.Label,
		map[string]any{"privileges": privilegeStrings(updated.Privileges)})
	      ok(c, gin.H{"privileges": privilegeStrings(updated.Privileges), "subUserId": updated.ID})
}

  func (a *API) setSubTransferPermPath(c *gin.Context) {
	    uid, okU := a.requireUser(c)
	  if !okU {
		return
	}
	body, err := bindSubTransferPermBody(c, c.Param("subUserId"))
	if err != nil {
	      	fail(c, 400, err.Error())
	     	return
	        }
	    if body.SubUserID == "" {
		fail(c, 400, "subUserId required")
	   	return
	  }
	  p, ch, _, okC := a.requireChild(c, uid, body.SubUserID)
	if !okC {
		return
	    }
	if err := a.surf().SetSubXferPerm(c.Request.Context(), p.ID, ch.ID, body.Transferable); err != nil {
		fail(c, 400, err.Error())
		return
	  }
	ok(c, gin.H{"subUserId": body.SubUserID, "transferable": body.Transferable})
}

func (a *API) getSubSnapshot(c *gin.Context) {
	uid, okU := a.requireUser(c)
	if !okU {
		return
	}
	   p, svc, err := a.parentOfUser(uid)
	    if err != nil {
		fail(c, 400, err.Error())
	   	return
	   }
	got, err := svc.Snapshot(p.ID)
	   if err != nil {
	  	fail(c, 400, err.Error())
	  	return
	}
	    ok(c, got)
}
