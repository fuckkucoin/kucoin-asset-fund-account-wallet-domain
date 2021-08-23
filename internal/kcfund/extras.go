package kcfund

import (
	"net/http"
	    "strconv"
	   "strings"
	"account-assets.kucoin.net/funding/internal/cvfund"
	   "account-assets.kucoin.net/funding/internal/earn"
	  "account-assets.kucoin.net/funding/internal/holdcat"
	   "account-assets.kucoin.net/funding/internal/notify"
	"account-assets.kucoin.net/funding/internal/perm"
	   "account-assets.kucoin.net/funding/internal/subacct"
	        "account-assets.kucoin.net/funding/internal/sqlrepo"
	  "account-assets.kucoin.net/funding/internal/subkey"
	    "account-assets.kucoin.net/funding/internal/utabook"
	  "account-assets.kucoin.net/funding/internal/vipfee"
	  "github.com/gin-gonic/gin"
)

   func (a *API) sub(g *gin.RouterGroup) {
	   g.GET("/v1/sub-accounts", a.listSubAccounts)
	         g.GET("/v2/sub/user", a.listSubAccounts)
	        g.GET("/v1/sub-accounts/snapshot", a.getSubSnapshot)
	      g.GET("/v1/sub-accounts/:subUserId", a.getSubAccount)
	        g.GET("/v1/sub-accounts/:subUserId/freeze", a.subFreeze)
	     g.POST("/v1/sub-accounts/:subUserId/freeze", a.freezeSubAccount)
	    g.POST("/v1/sub-accounts/:subUserId/close", a.closeSubAccount)
	     g.POST("/v1/sub-accounts/:subUserId/label", a.updateSubAccountLabel)
	     g.PUT("/v1/sub-accounts/:subUserId/label", a.updateSubAccountLabel)
	     g.POST("/v1/sub-accounts/:subUserId/privileges", a.setSubAccountPrivileges)
	         g.PUT("/v1/sub-accounts/:subUserId/privileges", a.setSubAccountPrivileges)
	  g.PUT("/v1/sub-accounts/:subUserId/transfer-permission", a.setSubTransferPermPath)
	   g.POST("/v2/sub/user", a.addSubAccount)
	         g.GET("/v1/user/api-key", a.listMasterAPIKeys)
	     mesh := a.meshSubKey
	     g.GET("/v1/sub/api-key", mesh(a.listSubAPIKeys))
	  g.GET("/v1/sub/api-key/detail", mesh(a.detailSubAPIKey))
	       g.GET("/v1/sub/api-key/quota", mesh(a.quotaSubAPIKeys))
	      g.GET("/v1/sub/api-key/audit", mesh(a.auditSubAPIKeys))
	   g.GET("/v1/sub/api-key/usage", mesh(a.usageSubAPIKey))
	  g.GET("/v1/sub/api-key/probe", mesh(a.probeSubAPIKey))
	  g.GET("/v1/sub/api-key/stats", mesh(a.statsSubAPIKeys))
	       g.GET("/v1/sub/api-key/expiring", mesh(a.expiringSubAPIKeys))
	  g.GET("/v1/sub/api-key/locked", mesh(a.lockedSubAPIKeys))
	        g.GET("/v1/sub/api-key/by-fp", mesh(a.fingerprintSubAPIKey))
	   g.POST("/v1/sub/api-key", mesh(a.addSubAPIKey))
	   g.POST("/v1/sub/api-key/update", mesh(a.modifySubAPIKey))
	  g.POST("/v1/sub/api-key/status", mesh(a.statusSubAPIKey))
	   g.POST("/v1/sub/api-key/disable-all", mesh(a.disableAllSubAPIKeys))
	   g.POST("/v1/sub/api-key/enable-all", mesh(a.enableAllSubAPIKeys))
	        g.POST("/v1/sub/api-key/rotate", mesh(a.rotateSubAPIKey))
	  g.POST("/v1/sub/api-key/passphrase", mesh(a.passphraseSubAPIKey))
	       g.POST("/v1/sub/api-key/sweep-expired", mesh(a.sweepExpiredSubAPIKeys))
	   g.POST("/v1/sub/api-key/sweep-idle", mesh(a.sweepIdleSubAPIKeys))
	   g.POST("/v1/sub/api-key/sweep-grace", mesh(a.sweepGraceSubAPIKeys))
	   g.POST("/v1/sub/api-key/sweep-audit", mesh(a.sweepAuditSubAPIKeys))
	     g.POST("/v1/sub/api-key/unlock", mesh(a.unlockSubAPIKey))
	      g.POST("/v1/sub/api-key/revoke", mesh(a.revokeSubAPIKey))
	         g.POST("/v1/sub/api-key/revoke-all", mesh(a.revokeAllSubAPIKeys))
	  g.POST("/v1/sub/api-key/restore", mesh(a.restoreSubAPIKey))
	       g.POST("/v1/sub/api-key/purge-revoked", mesh(a.purgeRevokedSubAPIKeys))
	       g.POST("/v1/sub/api-key/clone", mesh(a.cloneSubAPIKey))
	      g.POST("/v1/sub/api-key/reset-usage", mesh(a.resetUsageSubAPIKey))
	           g.POST("/v1/sub/api-key/revoke-grace", mesh(a.revokeGraceSubAPIKey))
	  g.DELETE("/v1/sub/api-key", mesh(a.deleteSubAPIKey))
	   g.POST("/v1/sub/transfer-permission", a.setSubTransferPerm)
	      g.GET("/v1/sub-accounts/:subUserId/balance", a.getSubBalance)
	   g.GET("/v2/sub-accounts/futures", a.getSubFutures)
	          g.GET("/v2/sub-accounts/spot", a.getSubSpot)
   }

  func (a *API) subs() *subacct.Service {
	   if a != nil && a.subSvc != nil {
	   	  return a.subSvc
	      }
	        return subacct.NewWithKV(a.rt.KV)
   }

 func (a *API) listSubAccounts(c *gin.Context) {
	       uid, okU := a.requireUser(c)
	  if !okU {
	   	   return
	         }
	     p, svc, err := a.parentOfUser(uid)
	  if err != nil {
	      	   fail(c, 400, err.Error())
	    	   return
	   }
	   kids := svc.ListChildren(p.ID)
	         items := make([]gin.H, 0, len(kids))
	      for _, k := range kids {
	   	   items = append(items, a.subAccountJSON(c, k, k.Label))
	      }
	   page, _ := parsePositiveInt(bffParam(c, "currentPage"), bffParam(c, "page"), 1)
	   pageSize, _ := parsePositiveInt(bffParam(c, "pageSize"), "", 50)
	         if pageSize > 200 {
	    	   pageSize = 200
	      }
	   total := len(items)
	   start := (page - 1) * pageSize
	   if start > total {
		   start = total
	      }
	   end := start + pageSize
	      if end > total {
	    	   end = total
	      }
	   totalPage := 1
	        if pageSize > 0 && total > 0 {
		   totalPage = (total + pageSize - 1) / pageSize
	   }
	   if strings.Contains(c.Request.URL.Path, "/v2/") {
	   	  ok(c, gin.H{"currentPage": page, "pageSize": pageSize, "totalNum": total, "totalPage": totalPage, "items": items[start:end]})
	    	  return
	      }
	    ok(c, gin.H{"items": items[start:end], "totalNum": total})
   }

  func (a *API) getSubAccount(c *gin.Context) {
	    uid, okU := a.requireUser(c)
	   if !okU {
		   return
	    }
	   _, ch, _, okC := a.requireChild(c, uid, c.Param("subUserId"))
	   if !okC {
		   return
	   }
	   ok(c, a.subAccountJSON(c, ch, ch.Label))
  }

  func (a *API) addSubAccount(c *gin.Context) {
	     body, err := bindCreateSubBody(c)
	  if err != nil {
	       	  fail(c, 400, err.Error())
	   	   return
	         }
	      pwd := strings.TrimSpace(body.Password)
	          if pwd != "" && len(pwd) < 8 {
	       	  fail(c, 400, "password must be at least 8 characters when provided")
		  return
	    }
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
	       childUID := firstNonEmpty(body.SubName, body.Remarks)
	    if childUID == "" {
	   	  fail(c, 400, "subName required")
	  	  return
	      }
	   label := firstNonEmpty(body.SubName, childUID)
	   ch, err := svc.OpenChild(subacct.OpenChildReq{ParentID: p.ID, UserID: uid + "-sub-" + childUID, Label: label})
	   if err != nil {
	   	   fail(c, 400, err.Error())
	   	   return
	     }
	   privs, err := subacct.ParseAccess(body.Access)
	   if err != nil {
	   	   fail(c, 400, err.Error())
		   return
	     }
	       if len(privs) > 0 {
	   	   updated, err := svc.SetPrivileges(ch.ID, privs)
	   	   if err != nil {
	   		   fail(c, 400, err.Error())
			   return
		   }
	    	   ch = updated
	       }
	  a.emitSubLifecycle(c.Request.Context(), "CREATED", uid, ch.ID, ch.Label, map[string]any{"access": body.Access})
	     ok(c, a.subCreateJSON(c, ch, body.Access))
  }

  func (a *API) keys() *subkey.Store {
	     if a != nil && a.rt != nil && a.rt.SubKeys != nil {
	    	  return a.rt.SubKeys
	  }
	       return nil
  }

  func (a *API) meshSubKey(h gin.HandlerFunc) gin.HandlerFunc {
	     return func(c *gin.Context) {
		   if c != nil && c.Request != nil && a.forwardMeshSubKey(c, c.Request.Method, c.Request.URL.Path, nil) {
	  		  return
		   }
	  	  h(c)
	       }
  }

// bindSubKeyOwner 母户鉴权：已登记子户时 subName 必须归属该母户且为 active。

  func (a *API) bindSubKeyOwner(store *subkey.Store) {
	       if store == nil {
	      	   return
	     }
	   store.BindOwner(subkey.SubOwnerFunc(func(ownerUID, subName string) bool {
		   ownerUID = strings.TrimSpace(ownerUID)
		  subName = strings.TrimSpace(subName)
	       	   if ownerUID == "" || subName == "" {
			  return false
	    	  }
	   	  svc := a.subs()
		   p, err := svc.OpenParent(subacct.OpenParentReq{UserID: ownerUID, Label: "primary"})
	       	   if err != nil {
			   return false
		  }
	  	  kids := svc.ListChildren(p.ID)
		  for _, k := range kids {
			   if (k.Label == subName || k.UserID == subName || k.ID == subName) && k.Status == subacct.StatusActive {
	   			  return true
	    		  }
	    	   }
	    	  return false
	      }))
  }

 func subKeyFail(c *gin.Context, err error) {
	   st := 400
	     switch {
	    case err == subkey.ErrGone:
		  st = 404
	    case err == subkey.ErrAuth, err == subkey.ErrOff, err == subkey.ErrSubOwn, err == subkey.ErrExpired, err == subkey.ErrIdle, err == subkey.ErrNotBefore, err == subkey.ErrPermNeed, err == subkey.ErrIPDeny, err == subkey.ErrStickyIP, err == subkey.ErrPathDeny, err == subkey.ErrMethodDeny, err == subkey.ErrSign, err == subkey.ErrLocked, err == subkey.ErrSkew, err == subkey.ErrReplay, err == subkey.ErrPassHdr, err == subkey.ErrSiteDeny, err == subkey.ErrSignVer, err == subkey.ErrRevoked:
		   st = 403
	       case err == subkey.ErrRate, err == subkey.ErrDay, err == subkey.ErrRotateCool, err == subkey.ErrOwnerRate, err == subkey.ErrMinute, err == subkey.ErrRotateMax, err == subkey.ErrMgmtRate:
		   st = 429
	         case err == subkey.ErrOwner:
	  	   st = 401
	   case err == subkey.ErrEnc, err == subkey.ErrSecret:
	   	  st = 503
	  }
	   fail(c, st, err.Error())
   }

  func (a *API) listSubAPIKeys(c *gin.Context) {
	       uid, okU := a.requireUser(c)
	  if !okU {
	   	  return
	    }
	      store := a.keys()
	        if store == nil {
	    	  ok(c, []gin.H{})
		   return
	  }
	     a.bindSubKeyOwner(store)
	      status := strings.TrimSpace(c.Query("status"))
	  label := strings.TrimSpace(c.Query("label"))
	  page, _ := strconv.Atoi(c.Query("page"))
	  pageSize, _ := strconv.Atoi(c.Query("pageSize"))
	   if status != "" || label != "" || page > 0 || pageSize > 0 {
	      	  ok(c, store.ListPage(subkey.ListFilter{
	  		   Owner: uid, SubName: c.Query("subName"), APIKey: c.Query("apiKey"),
	   		   Status: status, Label: label, Page: page, PageSize: pageSize,
		   }))
	        	  return
	   }
	     ok(c, store.List(uid, c.Query("subName"), c.Query("apiKey")))
   }

  func (a *API) addSubAPIKey(c *gin.Context) {
	       var body struct {
	   	   SubName string `json:"subName"`
	  	  Passphrase string `json:"passphrase"`
		  Remark     string `json:"remark"`
		  Permission string `json:"permission"`
	  	   IPWhitelist string `json:"ipWhitelist"`
	    	  ExpireAt    int64  `json:"expireAt"`
	    	   PathAllow   string `json:"pathAllow"`
		  IdleTTL     int64  `json:"idleTTL"`
	  	   MethodAllow string `json:"methodAllow"`
	   	   NotBefore int64 `json:"notBefore"`
		  StickyIP  bool  `json:"stickyIp"`
		   RateMax   int   `json:"rateMax"`
	    	  DayCap    int    `json:"dayCap"`
	    	  SiteAllow string `json:"siteAllow"`
	    	  SignVer int    `json:"signVer"`
		  Label   string `json:"label"`
	        }
	   if err := c.ShouldBindJSON(&body); err != nil {
	    	  fail(c, 400, err.Error())
	   	  return
	     }
	        uid, okU := a.requireUser(c)
	        if !okU {
	   	  return
	       }
	  store := a.keys()
	          if store == nil {
	    	   fail(c, 400, subkey.ErrEnc.Error())
		   return
	   }
	      a.bindSubKeyOwner(store)
	  created, err := store.Create(subkey.CreateReq{
	    	   Owner: uid,
		  SubName: body.SubName, Passphrase: body.Passphrase,
		  Remark: body.Remark, Permission: body.Permission, IPWhitelist: body.IPWhitelist,
	      	   ExpireAt: body.ExpireAt, CreatedIP: c.ClientIP(), PathAllow: body.PathAllow, IdleTTL: body.IdleTTL,
		  MethodAllow: body.MethodAllow, NotBefore: body.NotBefore, StickyIP: body.StickyIP,
		   RateMax: body.RateMax, DayCap: body.DayCap,
	    	   SiteAllow: body.SiteAllow, SignVer: body.SignVer, Label: body.Label, ActorIP: c.ClientIP(),
	      })
	       if err != nil {
	       	   subKeyFail(c, err)
	    	   return
	    }
	   ok(c, created)
   }

  func (a *API) modifySubAPIKey(c *gin.Context) {
	      var body struct {
	      	  SubName    string `json:"subName"`
	        	   APIKey     string `json:"apiKey"`
	     	  Passphrase string `json:"passphrase"`
	   	  Permission string `json:"permission"`
	   	   IPWhitelist *string `json:"ipWhitelist"`
		   Remark   *string `json:"remark"`
	  	   ExpireAt *int64  `json:"expireAt"`
	   	   RateMax  *int    `json:"rateMax"`
		  DayCap   *int    `json:"dayCap"`
	  	   PathAllow *string `json:"pathAllow"`
	        	  IdleTTL *int64 `json:"idleTTL"`
	  	  MethodAllow *string `json:"methodAllow"`
	     	   NotBefore *int64  `json:"notBefore"`
		  StickyIP  *bool   `json:"stickyIp"`
		  SiteAllow *string `json:"siteAllow"`
	      	  SignVer   *int    `json:"signVer"`
	   	  Label     *string `json:"label"`
	          }
	      if err := c.ShouldBindJSON(&body); err != nil {
	   	   fail(c, 400, err.Error())
	  	  return
	      }
	    uid, okU := a.requireUser(c)
	  if !okU {
		  return
	  }
	          store := a.keys()
	         if store == nil {
	    	   fail(c, 400, subkey.ErrEnc.Error())
	   	  return
	     }
	       a.bindSubKeyOwner(store)
	  req := subkey.ModifyReq{
	    	   Owner: uid, SubName: body.SubName, APIKey: body.APIKey, Passphrase: body.Passphrase,
	      	  Permission: body.Permission,
	   	   Remark:     body.Remark, ActorIP: c.ClientIP(),
	       }
	        if body.IPWhitelist != nil {
	  	   req.SetIP = true
	  	  req.IPWhitelist = *body.IPWhitelist
	      }
	     if body.ExpireAt != nil {
	    	   req.SetExpire = true
	       	   req.ExpireAt = *body.ExpireAt
	  }
	       if body.RateMax != nil {
		   req.SetRate = true
	  	  req.RateMax = *body.RateMax
	       }
	        if body.DayCap != nil {
	     	  req.SetDayCap = true
	   	   req.DayCap = *body.DayCap
	  }
	      if body.PathAllow != nil {
		   req.SetPath = true
	    	  req.PathAllow = *body.PathAllow
	         }
	   if body.IdleTTL != nil {
		  req.SetIdle = true
	    	  req.IdleTTL = *body.IdleTTL
	   }
	   if body.MethodAllow != nil {
	    	  req.SetMethod = true
	    	  req.MethodAllow = *body.MethodAllow
	     }
	       if body.NotBefore != nil {
	  	  req.SetNotBefore = true
		  req.NotBefore = *body.NotBefore
	   }
	       if body.StickyIP != nil {
		  req.SetSticky = true
	   	   req.StickyIP = *body.StickyIP
	      }
	  if body.SiteAllow != nil {
	  	   req.SetSite = true
		   req.SiteAllow = *body.SiteAllow
	   }
	    if body.SignVer != nil {
		  req.SetSignVer = true
	  	  req.SignVer = *body.SignVer
	      }
	   if body.Label != nil {
	   	  req.SetLabel = true
	    	   req.Label = *body.Label
	  }
	        rec, err := store.Modify(req)
	      if err != nil {
		   subKeyFail(c, err)
		  return
	  }
	  ok(c, gin.H{
	      	  "subName": rec.SubName, "apiKey": rec.APIKey, "remark": rec.Remark,
	       	  "permission": rec.Permission, "ipWhitelist": rec.IPWhitelist, "status": rec.Status,
	   	  "expireAt": rec.ExpireAt, "lastUsedAt": rec.LastUsedAt, "lastUsedIp": rec.LastUsedIP,
		  "rotatedAt": rec.RotatedAt, "rateMax": rec.RateMax, "dayCap": rec.DayCap,
	  	   "secretFp": rec.SecretFP, "graceUntil": rec.GraceUntil, "createdIp": rec.CreatedIP,
	  	   "pathAllow": rec.PathAllow, "idleTTL": rec.IdleTTL, "methodAllow": rec.MethodAllow,
	    	   "notBefore": rec.NotBefore, "stickyIp": rec.StickyIP, "siteAllow": rec.SiteAllow,
	    	  "signVer": rec.SignVer, "label": rec.Label, "rotateCount": rec.RotateCount,
		   "rotatedIp": rec.RotatedIP, "apiVersion": rec.APIVersion,
		   "updatedAt": rec.UpdatedAt,
	  })
  }

  func (a *API) statusSubAPIKey(c *gin.Context) {
	   var body struct {
	   	   SubName string `json:"subName"`
	       	  APIKey string `json:"apiKey"`
		   Passphrase string `json:"passphrase"`
	      	   Status string `json:"status"`
	    }
	  if err := c.ShouldBindJSON(&body); err != nil {
	       	   fail(c, 400, err.Error())
	  	   return
	        }
	      uid, okU := a.requireUser(c)
	  if !okU {
	     	   return
	    }
	  store := a.keys()
	      if store == nil {
		   fail(c, 400, subkey.ErrEnc.Error())
		  return
	     }
	      a.bindSubKeyOwner(store)
	    rec,
	  	   err := store.SetStatus(uid, body.SubName, body.APIKey, body.Passphrase, body.Status, c.ClientIP())
	     if err != nil {
	  	   subKeyFail(c, err)
		   return
	    }
	     ok(c, gin.H{"subName": rec.SubName, "apiKey": rec.APIKey, "status": rec.Status, "updatedAt": rec.UpdatedAt})
  }

   func (a *API) disableAllSubAPIKeys(c *gin.Context) {
	  var body struct {
	   	  SubName string `json:"subName"`
	       }
	       if err := c.ShouldBindJSON(&body); err != nil {
	   	   fail(c, 400, err.Error())
		  return
	    }
	   uid, okU := a.requireUser(c)
	    if !okU {
	  	  return
	      }
	         store := a.keys()
	    if store == nil {
	    	   fail(c, 400, subkey.ErrEnc.Error())
	  	   return
	  }
	        a.bindSubKeyOwner(store)
	   n, err := store.DisableAll(uid, body.SubName, c.ClientIP())
	      if err != nil {
	   	   subKeyFail(c, err)
		   return
	  }
	   ok(c, gin.H{"subName": strings.TrimSpace(body.SubName), "disabled": n})
  }

  func (a *API) deleteSubAPIKey(c *gin.Context) {
	       uid, okU := a.requireUser(c)
	  if !okU {
	    	  return
	  }
	     store := a.keys()
	        if store == nil {
	      	  fail(c, 400, subkey.ErrEnc.Error())
		  return
	        }
	        a.bindSubKeyOwner(store)
	     subName := c.Query("subName")
	     apiKey := c.Query("apiKey")
	     pass := c.Query("passphrase")
	     if subName == "" || apiKey == "" || pass == "" {
		   var body struct {
	    		   SubName string `json:"subName"`
	      		   APIKey     string `json:"apiKey"`
	      		   Passphrase string `json:"passphrase"`
	    	   }
		   _ = c.ShouldBindJSON(&body)
		  if subName == "" {
	   		  subName = body.SubName
	   	  }
	     	   if apiKey == "" {
	   		  apiKey = body.APIKey
	   	  }
		   if pass == "" {
	   		  pass = body.Passphrase
		  }
	   }
	       rec, err := store.Delete(uid, subName, apiKey, pass, c.ClientIP())
	          if err != nil {
	    	  subKeyFail(c, err)
	    	  return
	   }
	       ok(c, gin.H{"subName": rec.SubName, "apiKey": rec.APIKey})
  }

  func (a *API) rotateSubAPIKey(c *gin.Context) {
	      var body struct {
	    	  SubName string `json:"subName"`
		   APIKey string `json:"apiKey"`
	  	  Passphrase string `json:"passphrase"`
	        }
	    if err := c.ShouldBindJSON(&body); err != nil {
	   	  fail(c, 400, err.Error())
	  	  return
	     }
	      uid, okU := a.requireUser(c)
	     if !okU {
		  return
	      }
	     store := a.keys()
	     if store == nil {
	  	  fail(c, 400, subkey.ErrEnc.Error())
	  	   return
	  }
	     a.bindSubKeyOwner(store)
	   out, err := store.Rotate(uid, body.SubName, body.APIKey, body.Passphrase, c.ClientIP())
	      if err != nil {
	    	  subKeyFail(c, err)
		   return
	   }
	       ok(c, out)
  }

  func (a *API) passphraseSubAPIKey(c *gin.Context) {
	  var body struct {
	  	   SubName string `json:"subName"`
	   	   APIKey  string `json:"apiKey"`
		   Passphrase    string `json:"passphrase"`
	    	   NewPassphrase string `json:"newPassphrase"`
	          }
	      if err := c.ShouldBindJSON(&body); err != nil {
	     	   fail(c, 400, err.Error())
		   return
	   }
	      uid, okU := a.requireUser(c)
	        if !okU {
		  return
	     }
	       store := a.keys()
	   if store == nil {
		   fail(c, 400, subkey.ErrEnc.Error())
	     	  return
	      }
	   a.bindSubKeyOwner(store)
	   rec, err := store.ChangePassphrase(uid, body.SubName, body.APIKey, body.Passphrase, body.NewPassphrase, c.ClientIP())
	     if err != nil {
		   subKeyFail(c, err)
	  	   return
	      }
	   ok(c, gin.H{"subName": rec.SubName, "apiKey": rec.APIKey, "updatedAt": rec.UpdatedAt})
  }

  func (a *API) sweepExpiredSubAPIKeys(c *gin.Context) {
	      var body struct {
	     	   SubName string `json:"subName"`
	  }
	      if err := c.ShouldBindJSON(&body); err != nil {
	  	  fail(c, 400, err.Error())
	   	   return
	   }
	           uid, okU := a.requireUser(c)
	      if !okU {
	  	   return
	   }
	   store := a.keys()
	      if store == nil {
		   fail(c, 400, subkey.ErrEnc.Error())
		  return
	         }
	      a.bindSubKeyOwner(store)
	  n, err := store.SweepExpired(uid, body.SubName, c.ClientIP())
	  if err != nil {
	   	   subKeyFail(c, err)
	    	  return
	      }
	    ok(c, gin.H{"subName": strings.TrimSpace(body.SubName), "disabled": n})
  }

  func (a *API) sweepIdleSubAPIKeys(c *gin.Context) {
	      var body struct {
	   	   SubName string `json:"subName"`
	        }
	       if err := c.ShouldBindJSON(&body); err != nil {
		   fail(c, 400, err.Error())
		   return
	   }
	    uid, okU := a.requireUser(c)
	     if !okU {
	  	  return
	  }
	     store := a.keys()
	    if store == nil {
		   fail(c, 400, subkey.ErrEnc.Error())
	     	   return
	   }
	        a.bindSubKeyOwner(store)
	   n, err := store.SweepIdle(uid, body.SubName, c.ClientIP())
	      if err != nil {
	  	   subKeyFail(c, err)
	    	   return
	          }
	         ok(c, gin.H{"subName": strings.TrimSpace(body.SubName), "disabled": n})
   }

   func (a *API) sweepGraceSubAPIKeys(c *gin.Context) {
	  var body struct {
	   	  SubName string `json:"subName"`
	    }
	     if err := c.ShouldBindJSON(&body); err != nil {
	  	  fail(c, 400, err.Error())
	  	  return
	        }
	     uid, okU := a.requireUser(c)
	      if !okU {
	    	  return
	   }
	         store := a.keys()
	  if store == nil {
	    	   fail(c, 400, subkey.ErrEnc.Error())
	    	  return
	       }
	  a.bindSubKeyOwner(store)
	  n,
	  	  err := store.SweepGrace(uid, body.SubName, c.ClientIP())
	  if err != nil {
		   subKeyFail(c, err)
	  	   return
	  }
	      ok(c, gin.H{"subName": strings.TrimSpace(body.SubName), "cleared": n})
   }

   func (a *API) revokeGraceSubAPIKey(c *gin.Context) {
	  var body struct {
		   SubName string `json:"subName"`
		  APIKey     string `json:"apiKey"`
		  Passphrase string `json:"passphrase"`
	       }
	        if err := c.ShouldBindJSON(&body); err != nil {
	    	  fail(c, 400, err.Error())
		  return
	   }
	     uid, okU := a.requireUser(c)
	   if !okU {
	    	   return
	   }
	      store := a.keys()
	     if store == nil {
		  fail(c, 400, subkey.ErrEnc.Error())
	    	   return
	   }
	      a.bindSubKeyOwner(store)
	   rec, err := store.RevokeGrace(uid, body.SubName, body.APIKey, body.Passphrase, c.ClientIP())
	        if err != nil {
	   	   subKeyFail(c, err)
	     	  return
	      }
	     ok(c, gin.H{"subName": rec.SubName, "apiKey": rec.APIKey, "graceUntil": rec.GraceUntil, "updatedAt": rec.UpdatedAt})
   }

  func (a *API) unlockSubAPIKey(c *gin.Context) {
	       var body struct {
	    	   SubName string `json:"subName"`
	    	  APIKey  string `json:"apiKey"`
	   }
	     if err := c.ShouldBindJSON(&body); err != nil {
		  fail(c, 400, err.Error())
	    	  return
	       }
	     uid, okU := a.requireUser(c)
	    if !okU {
	   	  return
	   }
	  store := a.keys()
	       if store == nil {
	  	   fail(c, 400, subkey.ErrEnc.Error())
	       	   return
	      }
	     a.bindSubKeyOwner(store)
	       rec, err := store.Unlock(uid, body.SubName, body.APIKey, c.ClientIP())
	   if err != nil {
		   subKeyFail(c, err)
		   return
	    }
	    ok(c, gin.H{"subName": rec.SubName, "apiKey": rec.APIKey, "failCount": rec.FailCount, "lockedUntil": rec.LockedUntil, "updatedAt": rec.UpdatedAt})
  }

  func (a *API) quotaSubAPIKeys(c *gin.Context) {
	     uid, okU := a.requireUser(c)
	          if !okU {
		   return
	       }
	      store := a.keys()
	   if store == nil {
	    	   lim := subkey.MaxKeysPerSub
	    	   if strings.TrimSpace(c.Query("subName")) == "" {
	   		  lim = subkey.MaxKeysPerOwner
		  }
	   	  ok(c, gin.H{"owner": uid, "subName": c.Query("subName"), "used": 0, "limit": lim})
		  return
	  }
	       a.bindSubKeyOwner(store)
	   q, err := store.CountQuota(uid, c.Query("subName"))
	       if err != nil {
		   subKeyFail(c, err)
		  return
	     }
	       ok(c, q)
  }

 func (a *API) auditSubAPIKeys(c *gin.Context) {
	     uid, okU := a.requireUser(c)
	         if !okU {
	  	   return
	  }
	   store := a.keys()
	    if store == nil {
	    	   ok(c, []subkey.AuditEvent{})
	   	  return
	      }
	      a.bindSubKeyOwner(store)
	     limit, _ := strconv.Atoi(c.Query("limit"))
	   since, _ := strconv.ParseInt(c.Query("since"), 10, 64)
	       until, _ := strconv.ParseInt(c.Query("until"), 10, 64)
	  ok(c, store.ListAuditFilter(subkey.AuditFilter{
	  	  Owner: uid, SubName: c.Query("subName"), APIKey: c.Query("apiKey"),
	   	  Action: c.Query("action"), Limit: limit, Since: since, Until: until,
	  }))
   }

   func (a *API) usageSubAPIKey(c *gin.Context) {
	   uid, okU := a.requireUser(c)
	       if !okU {
	  	   return
	       }
	   store := a.keys()
	       if store == nil {
	    	  fail(c, 400, subkey.ErrEnc.Error())
	  	   return
	   }
	   a.bindSubKeyOwner(store)
	     rec, pol, day, err := store.Usage(uid, c.Query("subName"), c.Query("apiKey"))
	       if err != nil {
	  	   subKeyFail(c, err)
	    	   return
	     }
	       ok(c, gin.H{
	    	   "subName": rec.SubName, "apiKey": rec.APIKey, "callCount": rec.CallCount,
	   	  "dayCount": rec.DayCount, "dayStamp": rec.DayStamp,
	   	   "lastUsedAt": rec.LastUsedAt, "lastUsedIp": rec.LastUsedIP,
	    	   "rateMax": pol.Max, "rateWindowMs": pol.Window.Milliseconds(),
	  	  "dayCap": day.Max, "secretFp": rec.SecretFP, "graceUntil": rec.GraceUntil,
	  	   "createdIp": rec.CreatedIP, "pathAllow": rec.PathAllow, "idleTTL": rec.IdleTTL,
	  	   "methodAllow": rec.MethodAllow, "notBefore": rec.NotBefore, "stickyIp": rec.StickyIP,
	   	   "siteAllow": rec.SiteAllow, "signVer": rec.SignVer, "label": rec.Label,
	  	   "rotateCount": rec.RotateCount, "rotatedIp": rec.RotatedIP,
		  "lastFailAt": rec.LastFailAt, "lastFailIp": rec.LastFailIP, "denyCount": rec.DenyCount,
	     	   "minuteCap": store.MinutePolicy(rec.RateMax), "ownerRate": store.OwnerRatePolicy(),
	     	  "apiVersion": rec.APIVersion,
	     })
  }

   func (a *API) detailSubAPIKey(c *gin.Context) {
	      uid, okU := a.requireUser(c)
	  if !okU {
	        	  return
	   }
	         store := a.keys()
	   if store == nil {
	  	   fail(c, 400, subkey.ErrEnc.Error())
	    	   return
	      }
	  a.bindSubKeyOwner(store)
	       rec, err := store.Get(uid, c.Query("apiKey"))
	     if err != nil {
		   subKeyFail(c, err)
		   return
	      }
	   ok(c, rec)
  }

  func (a *API) probeSubAPIKey(c *gin.Context) {
	         uid, okU := a.requireUser(c)
	       if !okU {
	    	  return
	   }
	       store := a.keys()
	       if store == nil {
	   	  fail(c, 400, subkey.ErrEnc.Error())
	   	  return
	      }
	     a.bindSubKeyOwner(store)
	  rec, err := store.Probe(uid, c.Query("subName"), c.Query("apiKey"), c.Query("clientIp"), c.Query("site"), c.Query("method"), c.Query("path"), c.Query("permission"))
	  if err != nil {
		   subKeyFail(c, err)
		  return
	      }
	  ok(c, gin.H{
	    	  "subName": rec.SubName, "apiKey": rec.APIKey, "status": rec.Status,
	    	   "permission": rec.Permission, "siteAllow": rec.SiteAllow, "signVer": rec.SignVer,
	   	  "ok": true,
	      })
   }

 func (a *API) enableAllSubAPIKeys(c *gin.Context) {
	   var body struct {
	   	  SubName string `json:"subName"`
	     }
	      if err := c.ShouldBindJSON(&body); err != nil {
		  fail(c, 400, err.Error())
	    	   return
	     }
	  uid, okU := a.requireUser(c)
	    if !okU {
	    	  return
	     }
	     store := a.keys()
	      if store == nil {
	  	   fail(c, 400, subkey.ErrEnc.Error())
	    	   return
	          }
	      a.bindSubKeyOwner(store)
	    n, err := store.EnableAll(uid, body.SubName, c.ClientIP())
	     if err != nil {
		  subKeyFail(c, err)
		   return
	    }
	     ok(c, gin.H{"subName": strings.TrimSpace(body.SubName), "enabled": n})
  }

  func (a *API) revokeSubAPIKey(c *gin.Context) {
	  var body struct {
	  	  SubName string `json:"subName"`
	  	   APIKey string `json:"apiKey"`
	    	  Passphrase string `json:"passphrase"`
	     }
	         if err := c.ShouldBindJSON(&body); err != nil {
	    	   fail(c, 400, err.Error())
	  	   return
	  }
	  uid, okU := a.requireUser(c)
	      if !okU {
		  return
	     }
	   store := a.keys()
	   if store == nil {
	        	   fail(c, 400, subkey.ErrEnc.Error())
	    	   return
	  }
	  a.bindSubKeyOwner(store)
	      rec, err := store.Revoke(uid, body.SubName, body.APIKey, body.Passphrase, c.ClientIP())
	     if err != nil {
	   	  subKeyFail(c, err)
		   return
	       }
	     ok(c, gin.H{"subName": rec.SubName, "apiKey": rec.APIKey, "status": rec.Status, "updatedAt": rec.UpdatedAt})
  }

  func (a *API) restoreSubAPIKey(c *gin.Context) {
	      var body struct {
	      	   SubName string `json:"subName"`
	    	  APIKey  string `json:"apiKey"`
	   	   Passphrase string `json:"passphrase"`
	  }
	      if err := c.ShouldBindJSON(&body); err != nil {
		  fail(c, 400, err.Error())
	        	  return
	           }
	     uid, okU := a.requireUser(c)
	     if !okU {
		  return
	       }
	  store := a.keys()
	      if store == nil {
	  	   fail(c, 400, subkey.ErrEnc.Error())
	   	   return
	          }
	   a.bindSubKeyOwner(store)
	  rec, err := store.Restore(uid, body.SubName, body.APIKey, body.Passphrase, c.ClientIP())
	       if err != nil {
		  subKeyFail(c, err)
	    	   return
	  }
	     ok(c, gin.H{"subName": rec.SubName, "apiKey": rec.APIKey, "status": rec.Status, "updatedAt": rec.UpdatedAt})
  }

   func (a *API) sweepAuditSubAPIKeys(c *gin.Context) {
	     var body struct {
	   	   OlderThan int64 `json:"olderThan"`
	     }
	    if err := c.ShouldBindJSON(&body); err != nil {
	    	  fail(c, 400, err.Error())
		   return
	      }
	   uid, okU := a.requireUser(c)
	       if !okU {
		  return
	     }
	      store := a.keys()
	      if store == nil {
	   	  fail(c, 400, subkey.ErrEnc.Error())
		  return
	       }
	  a.bindSubKeyOwner(store)
	      n, err := store.SweepAudit(uid, body.OlderThan, c.ClientIP())
	  if err != nil {
		   subKeyFail(c, err)
		  return
	  }
	      ok(c, gin.H{"cleared": n})
  }

   func (a *API) statsSubAPIKeys(c *gin.Context) {
	  uid, okU := a.requireUser(c)
	     if !okU {
	   	   return
	       }
	      store := a.keys()
	       if store == nil {
	   	  ok(c, subkey.Stats{Owner: uid, SubName: c.Query("subName")})
		   return
	     }
	      a.bindSubKeyOwner(store)
	   st, err := store.CountStats(uid, c.Query("subName"))
	       if err != nil {
		   subKeyFail(c, err)
	   	  return
	  }
	    ok(c, st)
  }

   func (a *API) expiringSubAPIKeys(c *gin.Context) {
	   uid, okU := a.requireUser(c)
	  if !okU {
		  return
	     }
	          store := a.keys()
	    if store == nil {
	  	  ok(c, []subkey.Record{})
	   	  return
	        }
	      a.bindSubKeyOwner(store)
	      horizon, _ := strconv.ParseInt(c.Query("horizonMs"), 10, 64)
	     rows, err := store.ListExpiring(uid, c.Query("subName"), horizon)
	        if err != nil {
		   subKeyFail(c, err)
		  return
	       }
	    ok(c, rows)
   }

   func (a *API) lockedSubAPIKeys(c *gin.Context) {
	   uid, okU := a.requireUser(c)
	    if !okU {
	   	   return
	       }
	   store := a.keys()
	    if store == nil {
		  ok(c, []subkey.Record{})
	  	  return
	   }
	        a.bindSubKeyOwner(store)
	        rows, err := store.ListLocked(uid, c.Query("subName"))
	   if err != nil {
	  	   subKeyFail(c, err)
		  return
	      }
	      ok(c, rows)
   }

  func (a *API) fingerprintSubAPIKey(c *gin.Context) {
	    uid, okU := a.requireUser(c)
	      if !okU {
	        	   return
	     }
	   store := a.keys()
	        if store == nil {
		   fail(c, 400, subkey.ErrEnc.Error())
	    	   return
	   }
	  a.bindSubKeyOwner(store)
	       rec, err := store.FindByFP(uid, c.Query("secretFp"))
	  if err != nil {
		  subKeyFail(c, err)
		  return
	         }
	         ok(c, rec)
   }

  func (a *API) revokeAllSubAPIKeys(c *gin.Context) {
	    var body struct {
	     	  SubName string `json:"subName"`
	   }
	   if err := c.ShouldBindJSON(&body); err != nil {
	    	  fail(c, 400, err.Error())
	      	  return
	       }
	      uid, okU := a.requireUser(c)
	    if !okU {
	   	  return
	  }
	        store := a.keys()
	      if store == nil {
	   	  fail(c,
	      		  400, subkey.ErrEnc.Error())
	    	  return
	      }
	    a.bindSubKeyOwner(store)
	  n, err := store.RevokeAll(uid, body.SubName, c.ClientIP())
	      if err != nil {
		   subKeyFail(c, err)
	    	   return
	       }
	      ok(c, gin.H{"subName": strings.TrimSpace(body.SubName), "revoked": n})
  }

  func (a *API) purgeRevokedSubAPIKeys(c *gin.Context) {
	    var body struct {
		  SubName string `json:"subName"`
	   	   OlderThan int64 `json:"olderThan"`
	   }
	     if err := c.ShouldBindJSON(&body); err != nil {
	       	  fail(c, 400, err.Error())
	   	   return
	      }
	      uid, okU := a.requireUser(c)
	    if !okU {
		  return
	  }
	        store := a.keys()
	   if store == nil {
	  	   fail(c, 400, subkey.ErrEnc.Error())
		  return
	   }
	      a.bindSubKeyOwner(store)
	      n, err := store.PurgeRevoked(uid, body.SubName, body.OlderThan, c.ClientIP())
	  if err != nil {
	    	   subKeyFail(c, err)
		  return
	     }
	      ok(c, gin.H{"purged": n})
   }

  func (a *API) cloneSubAPIKey(c *gin.Context) {
	       var body struct {
		  SubName string `json:"subName"`
		   APIKey     string `json:"apiKey"`
	    	   Passphrase string `json:"passphrase"`
	     }
	      if err := c.ShouldBindJSON(&body); err != nil {
		  fail(c, 400, err.Error())
		   return
	          }
	  uid, okU := a.requireUser(c)
	     if !okU {
	    	   return
	  }
	       store := a.keys()
	  if store == nil {
	   	   fail(c,
	        		  400, subkey.ErrEnc.Error())
	    	  return
	  }
	         a.bindSubKeyOwner(store)
	    out, err := store.Clone(uid, body.SubName, body.APIKey, body.Passphrase, c.ClientIP())
	        if err != nil {
		   subKeyFail(c, err)
	   	  return
	   }
	  ok(c, out)
  }

   func (a *API) resetUsageSubAPIKey(c *gin.Context) {
	         var body struct {
		  SubName string `json:"subName"`
		  APIKey  string `json:"apiKey"`
	         }
	       if err := c.ShouldBindJSON(&body); err != nil {
	   	  fail(c, 400, err.Error())
		   return
	  }
	       uid, okU := a.requireUser(c)
	     if !okU {
	  	   return
	         }
	  store := a.keys()
	     if store == nil {
		   fail(c, 400, subkey.ErrEnc.Error())
	  	  return
	     }
	      a.bindSubKeyOwner(store)
	   rec, err := store.ResetUsage(uid, body.SubName, body.APIKey, c.ClientIP())
	     if err != nil {
	    	   subKeyFail(c, err)
	        	   return
	     }
	  ok(c, gin.H{"subName": rec.SubName, "apiKey": rec.APIKey, "dayCount": rec.DayCount, "updatedAt": rec.UpdatedAt})
   }

   func (a *API) setSubTransferPerm(c *gin.Context) {
	   body, err := bindSubTransferPermBody(c, "")
	         if err != nil {
	   	  fail(c, 400, err.Error())
	  	   return
	          }
	   uid, okU := a.requireUser(c)
	  if !okU {
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
	          ok(c, gin.H{"subUserId": ch.ID, "transferable": body.Transferable})
   }

  func (a *API) getSubBalance(c *gin.Context) {
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
	       id := strings.TrimSpace(c.Param("subUserId"))
	     owner := resolveHopOwner(svc, p, id)
	       if owner == p.ID && id != "" && id != p.ID && id != p.UserID {
		  fail(c, 404, "sub-account not found")
		  return
	  }
	      rows := svc.ListOwnerBooks(owner)
	      accs := make([]gin.H, 0, len(rows))
	   for _, r := range rows {
	       	  accs = append(accs, gin.H{
	  		   "currency": r.Asset, "type": "main", "balance": addDecStr(r.Available, r.Frozen),
	    		   "available": r.Available, "holds": r.Frozen, "userId": r.UserID,
	    	  })
	       }
	  items := subBalanceItemsJSON(rows)
	  ok(c, gin.H{"subUserId": id, "items": items, "accounts": accs})
   }

  func (a *API) getSubFutures(c *gin.Context) {
	   c.Request.URL.RawQuery = "type=contract&" + c.Request.URL.RawQuery
	       a.getSubBalance(c)
  }

 func (a *API) getSubSpot(c *gin.Context) {
	         c.Request.URL.RawQuery = "type=main&" + c.Request.URL.RawQuery
	          a.getSubBalance(c)
   }

   func (a *API) fee(g *gin.RouterGroup) {
	      g.GET("/v1/base-fee", a.getBaseFee)
	   g.GET("/v1/trade-fees", a.getTradeFees)
	        g.GET("/v1/withdrawals/fee", a.withdrawFeeQuote)
  }

   func (a *API) getBaseFee(c *gin.Context) {
	      uid, okU := a.requireUser(c)
	     if !okU {
	     	   return
	    }
	      svc := a.spotFeeSvc()
	  taker, maker, found, err := a.quoteSpotFee(svc, uid, c.Query("symbol"))
	  if err != nil {
	   	  fail(c, 400, err.Error())
	    	  return
	  }
	  if !found {
	  	  fail(c, 404, "fee schedule not configured")
	    	  return
	    }
	      ok(c, gin.H{
	    	  "takerFeeRate": taker, "makerFeeRate": maker,
	  	  "currencyType": c.Query("currencyType"), "kcsDeduction": false,
	  })
   }

   func (a *API) getTradeFees(c *gin.Context) {
	  uid, okU := a.requireUser(c)
	        if !okU {
		  return
	  }
	  symbols := splitFeeSymbols(c.Query("symbols"), c.Query("symbol"))
	    if len(symbols) == 0 {
		  fail(c, 400, "symbols required")
	  	  return
	     }
	     svc := a.spotFeeSvc()
	      rows := make([]gin.H, 0, len(symbols))
	     for _, sym := range symbols {
	  	  taker, maker, found, err := a.quoteSpotFee(svc, uid, sym)
	  	  if err != nil {
	    		  fail(c, 400, err.Error())
	   		  return
	   	  }
		  if !found {
	   		  fail(c, 404, "fee schedule not configured")
			  return
	        	  }
	    	  rows = append(rows, gin.H{"symbol": sym, "takerFeeRate": taker, "makerFeeRate": maker})
	  }
	  ok(c, rows)
   }

  func (a *API) withdrawFeeQuote(c *gin.Context) {
	   cur := strings.ToUpper(strings.TrimSpace(c.Query("currency")))
	         chain := strings.ToUpper(strings.TrimSpace(c.Query("chain")))
	  rate, minFee, found := a.withdrawFeeParts(c, cur, chain)
	      if !found {
	   	  fail(c, 404, "withdraw fee not configured")
		  return
	      }
	  ok(c, gin.H{
		  "currency": cur, "chain": chain,
		  "withdrawMinFee": minFee, "withdrawFeeRate": rate,
	      	  "withdrawMinSize": a.withdrawMinSize(c, cur),
	      })
   }

   func (a *API) spotFeeSvc() *vipfee.Service {
	     svc := vipfee.New()
	      if a != nil && a.rt != nil && !a.rt.Mem && a.rt.SQL != nil {
	  	  svc.BindSQL(a.rt.SQL)
	     }
	  svc.ClearPromo()
	  return svc
  }

  func (a *API) quoteSpotFee(svc *vipfee.Service, uid, symbol string) (taker, maker string, found bool, err error) {
	      asset := feeAsset(symbol)
	      if svc != nil && svc.StoreBacked() {
		  eff, err := svc.Resolve(vipfee.ResolveReq{UserID: uid, Asset: asset})
	   	  if err != nil {
			  return "", "", false, err
		  }
	  	  return bpsRate(eff.TakerBPS), bpsRate(eff.MakerBPS), true, nil
	  }
	    if a != nil && a.rt != nil && a.rt.Cfg != nil && a.rt.Cfg.Spot.FeeBPS > 0 {
	   	  return bpsRate(a.rt.Cfg.Spot.FeeBPS), "", true, nil
	  }
	    return "", "", false, nil
  }

  func (a *API) withdrawFeeParts(c *gin.Context, cur, chain string) (rate, minFee string, found bool) {
	  if sch, okS := a.withdrawSchedule(c, cur, chain); okS {
		  rate = "0"
	  	  if sch.BPS > 0 {
			  rate = bpsRate(sch.BPS)
	  	  }
		  minFee = "0"
		  if v := strings.TrimSpace(sch.MinFee); v != "" {
			  minFee = v
	    	  } else if v := strings.TrimSpace(sch.Flat); v != "" {
	   		  minFee = v
	       	  }
		  return rate, minFee, true
	  }
	      if a != nil && a.rt != nil && a.rt.Cfg != nil && a.rt.Cfg.Transfer.WithdrawFeeBPS > 0 {
	   	  return bpsRate(a.rt.Cfg.Transfer.WithdrawFeeBPS), "0", true
	    }
	  return "", "", false
  }

  func (a *API) withdrawSchedule(c *gin.Context, cur, chain string) (sqlrepo.FeeSchedule, bool) {
	    if a == nil || a.rt == nil || a.rt.Mem || a.rt.SQL == nil || c == nil || c.Request == nil {
		  return sqlrepo.FeeSchedule{}, false
	  }
	  repo := sqlrepo.NewFeeSched(a.rt.SQL)
	    keys := make([]string, 0, 3)
	     if cur != "" && chain != "" {
	  	  keys = append(keys, cur+"-"+chain)
	      }
	  if cur != "" {
	   	  keys = append(keys, cur)
	  }
	      keys = append(keys, "*")
	  for _, key := range keys {
		  row, err := repo.Get(c.Request.Context(), "withdraw", key)
	     	  if err == nil && row.Active {
			  return row, true
	  	  }
	    }
	     return sqlrepo.FeeSchedule{}, false
  }

  func (a *API) withdrawMinSize(c *gin.Context, cur string) string {
	      if cur != "" && c != nil && c.Request != nil {
	    	  if row, okL := a.listingBySymbol(c.Request.Context())[cur]; okL && strings.TrimSpace(row.MinWithdraw) != "" {
			  return strings.TrimSpace(row.MinWithdraw)
	      	  }
	      }
	     if a != nil && a.rt != nil && a.rt.Cfg != nil && strings.TrimSpace(a.rt.Cfg.Risk.MinAmount) != "" {
	   	  return strings.TrimSpace(a.rt.Cfg.Risk.MinAmount)
	         }
	  return "0"
  }

  func feeAsset(symbol string) string {
	    asset := strings.ToUpper(strings.TrimSpace(symbol))
	      if i := strings.Index(asset, "-"); i > 0 {
	  	  asset = asset[:i]
	     }
	  if asset == "" {
		  return "USDT"
	     }
	     return asset
  }

  func splitFeeSymbols(symbols, symbol string) []string {
	  raw := strings.TrimSpace(symbols)
	        if raw == "" {
	    	  raw = strings.TrimSpace(symbol)
	     }
	  if raw == "" {
	     	  return nil
	    }
	  parts := strings.Split(raw, ",")
	        out := make([]string, 0, len(parts))
	      seen := map[string]struct{}{}
	      for _, p := range parts {
	  	  sym := strings.ToUpper(strings.TrimSpace(p))
		  if sym == "" {
	    		  continue
	  	  }
	  	  if _, ok := seen[sym]; ok {
	    		  continue
	  	  }
		  seen[sym] = struct{}{}
		  out = append(out, sym)
	      }
	     return out
  }

   func bpsRate(bps int) string {
	      return strconv.FormatFloat(float64(bps)/10000.0, 'f', 4, 64)
   }

  func (a *API) convert(g *gin.RouterGroup) {
	     g.POST("/v3/convert/quote", a.quoteConvert)
	     g.POST("/v3/convert", a.placeConvert)
	   g.GET("/v3/convert", a.listConvert)
	      g.GET("/v3/convert/:id", a.getConvert)
	      g.GET("/v1/convert/limit/quote", a.limitQuote)
	     g.POST("/v1/convert/limit/order", a.placeLimit)
	       g.GET("/v1/convert/limit/order/detail", a.getLimit)
	   g.GET("/v1/convert/limit/orders", a.listLimits)
	   g.DELETE("/v1/convert/limit/order", a.cancelLimit)
	   g.GET("/v1/convert/currencies", a.listConvertCurrencies)
	         g.POST("/v1/convert/limit/order/fill", a.partialFillLimit)
	        g.GET("/v1/convert/limit/order/freeze-integrity", a.freezeIntegrityLimit)
   }

   func (a *API) placeConvert(c *gin.Context) {
	   if a.forwardMeshConvert(c, http.MethodPost, "/api/v3/convert", nil) {
	  	  return
	     }
	   body, err := bindConvertPlaceBody(c)
	   if err != nil {
		  fail(c, 400, err.Error())
	      	  return
	      }
	         if strings.EqualFold(body.ConvertType, "STAKING") {
	   	  a.placeStakingConvert(c, body.FromCurrency, body.FromAmount, firstNonEmpty(body.ClientOid, body.QuoteID))
	    	   return
	        }
	  if a.rt.Convert == nil {
	  	  a.rt.Convert = cvfund.New()
	   }
	   book := convertAccount(body.AccountType)
	  uid, okU := a.requireUser(c)
	    if !okU {
		  return
	          }
	       ord, err := a.rt.Convert.PlaceQuoted(uid, body.FromCurrency, body.ToCurrency, body.FromAmount, body.Rate, book, body.QuoteID, a.rt.Mem)
	    if err != nil {
	  	   fail(c, 400, err.Error())
		   return
	     }
	          a.note(c.Request.Context(), "cv:"+ord.ID, notify.KindConvertFilled, ord.ID)
	    a.emitConvertEvent(c.Request.Context(), uid, ord, "FILLED")
	    ok(c, convertOrderJSON(ord))
  }

  func (a *API) quoteConvert(c *gin.Context) {
	   if a.forwardMeshConvert(c, http.MethodPost, "/api/v3/convert/quote", nil) {
	       	   return
	   }
	   body, err := bindConvertQuoteBody(c)
	      if err != nil {
	   	   fail(c, 400, err.Error())
		  return
	          }
	      if a.rt.Convert == nil {
		   fail(c, 503, cvfund.ErrQuoteSrc.Error())
		   return
	      }
	         book := convertAccount(body.AccountType)
	     uid, okU := a.requireUser(c)
	        if !okU {
	    	  return
	  }
	    amt := body.FromAmount
	      if amt == "" {
	     	  amt = body.ToAmount
	      }
	  q,
		   err := a.rt.Convert.Quote(c.Request.Context(), uid, body.FromCurrency, body.ToCurrency, amt, book)
	      if err != nil {
	   	  fail(c, 400, err.Error())
	       	  return
	      }
	       ok(c, quoteJSON(q))
  }

   func (a *API) listConvert(c *gin.Context) {
	    if a.forwardMeshConvert(c, http.MethodGet, "/api/v3/convert", nil) {
	   	  return
	          }
	  page, pageSize := convertListQuery(c)
	      if a.rt.Convert == nil {
	    	  ok(c, convertListJSON(nil, page, pageSize))
		   return
	    }
	      uid, okU := a.requireUser(c)
	    if !okU {
		   return
	    }
	   ok(c, convertListJSON(a.rt.Convert.List(uid, 100), page, pageSize))
  }

   func (a *API) getConvert(c *gin.Context) {
	        if a.forwardMeshConvert(c, http.MethodGet, "/api/v3/convert/"+c.Param("id"), nil) {
	    	   return
	  }
	      if a.rt.Convert == nil {
		  fail(c, 404, "convert order not found")
	  	  return
	        }
	      uid, okU := a.requireUser(c)
	    if !okU {
	     	   return
	     }
	    ord, err := a.rt.Convert.GetFor(uid, c.Param("id"))
	  if err != nil {
		  fail(c, 404, err.Error())
		   return
	        }
	    ok(c, convertOrderJSON(ord))
   }

   func (a *API) limitQuote(c *gin.Context) {
	  if a.forwardMeshConvert(c, http.MethodGet, "/api/v1/convert/limit/quote", nil) {
		  return
	   }
	       if a.rt.Convert == nil {
	   	  fail(c,
	  		   503, cvfund.ErrQuoteSrc.Error())
	   	  return
	  }
	       from, to, fromSize, toSize := limitQuoteParams(c)
	    price, until, err := a.rt.Convert.LimitQuote(c.Request.Context(), from, to, fromSize, toSize)
	       if err != nil {
	    	   fail(c, 400, err.Error())
		   return
	     }
	     ok(c, gin.H{"price": price.String(), "validUntill": until})
  }

  func (a *API) placeLimit(c *gin.Context) {
	   if a.forwardMeshConvert(c, http.MethodPost, "/api/v1/convert/limit/order", nil) {
	    	   return
	     }
	     var body struct {
		   ClientOrderID string `json:"clientOrderId"`
	   	  FromCurrency  string `json:"fromCurrency"`
	    	   ToCurrency string `json:"toCurrency"`
		   FromCurrencySize string `json:"fromCurrencySize"`
	    	  ToCurrencySize string `json:"toCurrencySize"`
		   AccountType    string `json:"accountType"`
	    }
	    if err := c.ShouldBindJSON(&body); err != nil {
		   fail(c, 400, err.Error())
		  return
	         }
	     uid, okU := a.requireUser(c)
	      if !okU {
	    	   return
	       }
	    if a.rt.Convert == nil {
	   	   fail(c, 503, cvfund.ErrQuoteSrc.Error())
	  	   return
	     }
	   ord,
		  err := a.rt.Convert.PlaceLimit(c.Request.Context(), uid, body.ClientOrderID, body.FromCurrency, body.ToCurrency, body.FromCurrencySize, body.ToCurrencySize, body.AccountType)
	        if err != nil {
	    	  fail(c, 400, err.Error())
	  	  return
	  }
	  ok(c, gin.H{"orderId": ord.ID, "clientOrderId": ord.ClientOrderID, "status": ord.Status})
   }

  func (a *API) getLimit(c *gin.Context) {
	   if a.forwardMeshConvert(c, http.MethodGet, "/api/v1/convert/limit/order/detail", nil) {
		  return
	       }
	       uid, okU := a.requireUser(c)
	      if !okU {
		  return
	    }
	    if a.rt.Convert == nil {
	      	  fail(c, 404, cvfund.ErrGone.Error())
	   	   return
	   }
	  ord, err := a.rt.Convert.GetLimit(c.Request.Context(), uid, c.Query("orderId"), c.Query("clientOrderId"))
	  if err != nil {
	    	  fail(c, 404, err.Error())
	       	   return
	        }
	   ok(c, ord)
   }

  func (a *API) listLimits(c *gin.Context) {
	     if a.forwardMeshConvert(c, http.MethodGet, "/api/v1/convert/limit/orders", nil) {
		  return
	      }
	       uid, okU := a.requireUser(c)
	     if !okU {
	    	  return
	       }
	   if a.rt.Convert == nil {
		  ok(c, []cvfund.LimitOrder{})
	   	   return
	       }
	     ok(c, a.rt.Convert.ListLimits(c.Request.Context(), uid, c.Query("status"), 20))
  }

  func (a *API) cancelLimit(c *gin.Context) {
	  if a.forwardMeshConvert(c, http.MethodDelete, "/api/v1/convert/limit/order", nil) {
	    	   return
	   }
	      uid, okU := a.requireUser(c)
	      if !okU {
	     	  return
	     }
	     if a.rt.Convert == nil {
		   fail(c, 404, cvfund.ErrGone.Error())
	  	   return
	      }
	      ord, err := a.rt.Convert.CancelLimit(c.Request.Context(), uid, c.Query("orderId"), firstNonEmpty(c.Query("clientOrderId"), c.Query("clientOid")))
	    if err != nil {
	     	  fail(c, 400, err.Error())
	   	   return
	       }
	    ok(c, gin.H{"orderId": ord.ID, "status": ord.Status})
  }

// partialFillLimit 限价闪兑部分成交 HTTP 面。
// 角色闸 + HMAC/mTLS 身份（超越 X-Role）；撮合回调写入成交量；幂等 fillSeq。
// 终端用户不得自助填单；须配置 CONVERT_FILL_HMAC_SECRET 或 mTLS CN。

   func (a *API) partialFillLimit(c *gin.Context) {
	      actor := perm.FromRequest(c.Request, "")
	         if err := actor.Check(perm.ActConvertFill); err != nil {
	       	  fail(c, 403, err.Error())
	   	   return
	     }
	       raw, err := cvfund.ReadBody(c.Request)
	      if err != nil {
	    	   fail(c, 400, err.Error())
	   	  return
	      }
	      ident := a.fillIdent()
	      identMethod,
		   ierr := ident.VerifyRequest(c.Request, raw)
	   if ierr != nil {
	   	  fail(c, 401, ierr.Error())
	  	   return
	        }
	   if a.rt.Convert == nil {
	       	  fail(c, 503, cvfund.ErrQuoteSrc.Error())
	    	   return
	  }
	  var body struct {
	  	  UserID string `json:"userId"`
		  OrderID string `json:"orderId"`
		   FillSize string `json:"fillSize"`
		  FillSeq  string `json:"fillSeq"`
	  }
	    if err := c.ShouldBindJSON(&body); err != nil {
	    	   fail(c, 400, err.Error())
		   return
	  }
	     uid := strings.TrimSpace(body.UserID)
	   if uid == "" {
		  var okU bool
	   	   uid, okU = a.requireUser(c)
	    	  if !okU {
	     		   return
	    	   }
	       }
	  ord, err := a.rt.Convert.PartialFill(c.Request.Context(), uid, body.OrderID, body.FillSize, body.FillSeq)
	   if err != nil {
	  	  fail(c, 400, err.Error())
	  	   return
	       }
	         ok(c, gin.H{
		  "orderId": ord.ID, "status": ord.Status,
	      	   "filledSize": ord.FilledSize, "filledToSize": ord.FilledToSize,
		   "actor": actor.ID, "role": string(actor.Role), "ident": identMethod,
	       })
  }

// freezeIntegrityLimit 核对限价单 holdcat 与剩余未成交。
// 只读检查，发现漂移返回错误；不得补铸。

 func (a *API) freezeIntegrityLimit(c *gin.Context) {
	  uid, okU := a.requireUser(c)
	         if !okU {
	    	  return
	       }
	   if a.rt.Convert == nil {
		   fail(c, 404, cvfund.ErrGone.Error())
	    	   return
	      }
	   if err := a.rt.Convert.FreezeIntegrity(c.Request.Context(), uid, c.Query("orderId")); err != nil {
		   fail(c, 409, err.Error())
	  	   return
	  }
	     ok(c, gin.H{"ok": true, "orderId": c.Query("orderId")})
  }

   func (a *API) placeStakingConvert(c *gin.Context, asset, amount, idem string) {
	    uid, okU := a.requireUser(c)
	  if !okU {
	    	   return
	   }
	   if strings.TrimSpace(idem) == "" {
	   	   fail(c, 400, "clientOid required")
	     	   return
	   }
	  prodID := ""
	      for _, p := range a.earns().ProductsBy(earn.CatStaking) {
		   if strings.EqualFold(p.Asset, asset) {
	   		   prodID = p.ID
	  		  break
	        	  }
	   }
	     if prodID == "" {
	  	  fail(c, 400, "staking product not listed")
		   return
	      }
	     pos, err := a.earns().Subscribe(c.Request.Context(), earn.SubscribeReq{
		  UserID: uid, ProductID: prodID, Asset: asset, Amount: amount, IdempotencyKey: idem,
	   })
	     if err != nil {
		   fail(c, 400, err.Error())
	   	   return
	     }
	     ok(c, gin.H{"orderId": pos.ID, "status": "filled", "convertType": "STAKING", "productId": prodID})
   }

   func (a *API) earn(g *gin.RouterGroup) {
	      g.GET("/v1/earn/products", a.earnProducts)
	      g.GET("/v1/earn/hold-assets", a.earnHold)
	     g.POST("/v1/earn/orders", a.earnPurchase)
	  g.DELETE("/v1/earn/orders", a.earnRedeem)
	  g.POST("/v1/earn/orders/redeem-early", a.earnRedeemEarly)
	        g.PUT("/v1/earn/orders/auto-subscribe", a.earnAuto)
  }

  func (a *API) earns() *earn.Service {
	      if a != nil && a.earnSvc != nil {
	    	   return a.earnSvc
	     }
	     if a == nil || a.rt == nil {
		  return earn.New(nil)
	     }
	       return earn.New(a.rt.Ledger).BindKV(a.rt.KV).BindHolds(holdcat.New(a.rt.KV))
   }

  func (a *API) earnProducts(c *gin.Context) {
	   if a.forwardMeshEarn(c, http.MethodGet, "/api/v1/earn/products", nil) {
	    	   return
	      }
	   cat := earnProductCategory(c)
	       var rows []earn.Product
	        if cat == "" {
	  	   rows = a.earns().Products()
	   } else {
		   rows = a.earns().ProductsBy(cat)
	      }
	      ok(c, earnProductsJSON(rows))
  }

  func (a *API) earnHold(c *gin.Context) {
	   if a.forwardMeshEarn(c, http.MethodGet, "/api/v1/earn/hold-assets", nil) {
	      	   return
	     }
	      uid, okU := a.requireUser(c)
	   if !okU {
	    	  return
	     }
	       ok(c, earnHoldAssetsJSON(a.earns().List(uid)))
  }

  func (a *API) earnPurchase(c *gin.Context) {
	       if a.forwardMeshEarn(c, http.MethodPost, "/api/v1/earn/orders", nil) {
	    	  return
	   }
	         body, err := bindEarnPurchaseBody(c)
	     if err != nil {
		   fail(c, 400, err.Error())
		  return
	       }
	  uid, okU := a.requireUser(c)
	       if !okU {
		  return
	     }
	    pos, err := a.earns().Subscribe(c.Request.Context(), earn.SubscribeReq{
	   	  UserID: uid, ProductID: body.ProductID, Asset: body.Currency,
		   Amount: body.Amount, IdempotencyKey: body.ClientOid, AutoSubscribe: body.AutoSubscribe,
	       })
	      if err != nil {
	    	  fail(c, 400, err.Error())
	  	   return
	   }
	  a.emitEarnEvent(c.Request.Context(), uid, pos, "SUBSCRIBED", "")
	  ok(c, earnPositionJSON(pos))
   }

  func (a *API) earnRedeem(c *gin.Context) {
	  if a.forwardMeshEarn(c, http.MethodDelete, "/api/v1/earn/orders", nil) {
		   return
	          }
	  body, err := bindEarnRedeemBody(c)
	        if err != nil {
	   	   fail(c, 400, err.Error())
		  return
	   }
	   uid, okU := a.requireUser(c)
	   if !okU {
	   	  return
	     }
	       pos, err := a.earns().Redeem(c.Request.Context(), earn.RedeemReq{
	     	  UserID:     uid,
	      	   PositionID: body.OrderID, IdempotencyKey: body.ClientOid,
	  })
	   if err != nil {
		  fail(c, 400, err.Error())
	   	  return
	   }
	      a.emitEarnEvent(c.Request.Context(), uid, pos, "REDEEMED", "")
	     ok(c, earnPositionJSON(pos))
  }

  func (a *API) earnRedeemEarly(c *gin.Context) {
	      if a.forwardMeshEarn(c, http.MethodPost, "/api/v1/earn/orders/redeem-early", nil) {
	   	   return
	   }
	  body, err := bindEarnRedeemBody(c)
	  if err != nil {
	    	   fail(c, 400, err.Error())
	     	  return
	       }
	     uid, okU := a.requireUser(c)
	   if !okU {
		  return
	   }
	    pos, pen, err := a.earns().RedeemEarly(c.Request.Context(), earn.EarlyReq{
	      	  UserID: uid, PositionID: body.OrderID, IdempotencyKey: body.ClientOid,
	  })
	      if err != nil {
	      	  fail(c, 400, err.Error())
	   	  return
	           }
	      a.emitEarnEvent(c.Request.Context(), uid, pos, "REDEEMED", pen.Fee)
	       ok(c, earnRedeemEarlyJSON(pos, pen))
  }

  func (a *API) earnAuto(c *gin.Context) {
	      if a.forwardMeshEarn(c, http.MethodPut, "/api/v1/earn/orders/auto-subscribe", nil) {
		  return
	      }
	      body, err := bindEarnAutoBody(c)
	     if err != nil {
	    	  fail(c, 400, err.Error())
		  return
	     }
	  uid, okU := a.requireUser(c)
	        if !okU {
	      	  return
	       }
	        pos, err := a.earns().SetAutoSubscribe(uid, body.OrderID, body.AutoSubscribe)
	       if err != nil {
	    	   fail(c, 400, err.Error())
		   return
	        }
	  ok(c, earnPositionJSON(pos))
   }

   func (a *API) uta(g *gin.RouterGroup) {
	     g.GET("/ua/v1/account", a.utaOverview)
	       g.GET("/ua/v2/unified/account/balance", a.utaBalance)
	     g.GET("/ua/v1/unified/account/balance", a.utaBalance)
	   g.GET("/ua/v2/account/balance", a.classicBalance)
	   g.GET("/ua/v1/account/ledger", a.getLedgers)
	  g.POST("/ua/v1/account/transfer", a.flexTransfer)
	      g.POST("/ua/v2/account/transfer", a.flexTransfer)
	    g.GET("/ua/v1/account/transfer-quotas", a.getTransferQuotas)
	   g.GET("/ua/v2/account/transfer-quota", a.getTransferQuotas)
	  g.GET("/ua/v2/account/transfer-quotas", a.getTransferQuotas)
	     g.GET("/ua/v1/account/mode", a.utaMode)
	    g.POST("/ua/v1/account/mode", a.utaSetMode)
	      g.GET("/ua/v1/unified/account/leverage", a.utaLeverage)
	     g.POST("/ua/v1/unified/account/leverage", a.utaModifyLeverage)
	   g.POST("/ua/v1/unified/account/modify-leverage", a.utaModifyLeverage)
	  g.POST("/ua/v1/unified/account/modify-cross-leverage", a.utaModifyLeverage)
	      g.GET("/ua/v1/account/interest-limits", a.utaInterestLimits)
	   g.GET("/ua/v1/account/interest-history", a.utaInterestHistory)
	         g.POST("/ua/v2/user/create-sub-api-key", a.meshSubKey(a.addSubAPIKey))
	    g.POST("/ua/v2/user/modify-sub-api-key", a.meshSubKey(a.modifySubAPIKey))
	  g.POST("/ua/v2/user/sub-api-key/status", a.meshSubKey(a.statusSubAPIKey))
	   g.POST("/ua/v2/user/sub-api-key/disable-all", a.meshSubKey(a.disableAllSubAPIKeys))
	     g.POST("/ua/v2/user/sub-api-key/enable-all", a.meshSubKey(a.enableAllSubAPIKeys))
	      g.POST("/ua/v2/user/sub-api-key/rotate", a.meshSubKey(a.rotateSubAPIKey))
	      g.POST("/ua/v2/user/sub-api-key/passphrase", a.meshSubKey(a.passphraseSubAPIKey))
	       g.POST("/ua/v2/user/sub-api-key/sweep-expired", a.meshSubKey(a.sweepExpiredSubAPIKeys))
	  g.POST("/ua/v2/user/sub-api-key/sweep-idle", a.meshSubKey(a.sweepIdleSubAPIKeys))
	        g.POST("/ua/v2/user/sub-api-key/sweep-grace", a.meshSubKey(a.sweepGraceSubAPIKeys))
	      g.POST("/ua/v2/user/sub-api-key/sweep-audit", a.meshSubKey(a.sweepAuditSubAPIKeys))
	  g.POST("/ua/v2/user/sub-api-key/unlock", a.meshSubKey(a.unlockSubAPIKey))
	   g.POST("/ua/v2/user/sub-api-key/revoke", a.meshSubKey(a.revokeSubAPIKey))
	           g.POST("/ua/v2/user/sub-api-key/revoke-all", a.meshSubKey(a.revokeAllSubAPIKeys))
	         g.POST("/ua/v2/user/sub-api-key/restore", a.meshSubKey(a.restoreSubAPIKey))
	      g.POST("/ua/v2/user/sub-api-key/purge-revoked", a.meshSubKey(a.purgeRevokedSubAPIKeys))
	     g.POST("/ua/v2/user/sub-api-key/clone", a.meshSubKey(a.cloneSubAPIKey))
	  g.POST("/ua/v2/user/sub-api-key/reset-usage", a.meshSubKey(a.resetUsageSubAPIKey))
	    g.POST("/ua/v2/user/sub-api-key/revoke-grace", a.meshSubKey(a.revokeGraceSubAPIKey))
	      g.DELETE("/ua/v2/user/sub-api-key", a.meshSubKey(a.deleteSubAPIKey))
	   g.GET("/ua/v2/user/sub-api-key", a.meshSubKey(a.listSubAPIKeys))
	   g.GET("/ua/v2/user/get-sub-api-key", a.meshSubKey(a.listSubAPIKeys))
	   g.GET("/ua/v2/user/sub-api-key/detail", a.meshSubKey(a.detailSubAPIKey))
	     g.GET("/ua/v2/user/sub-api-key/quota", a.meshSubKey(a.quotaSubAPIKeys))
	         g.GET("/ua/v2/user/sub-api-key/audit", a.meshSubKey(a.auditSubAPIKeys))
	  g.GET("/ua/v2/user/sub-api-key/usage", a.meshSubKey(a.usageSubAPIKey))
	      g.GET("/ua/v2/user/sub-api-key/probe", a.meshSubKey(a.probeSubAPIKey))
	       g.GET("/ua/v2/user/sub-api-key/stats", a.meshSubKey(a.statsSubAPIKeys))
	     g.GET("/ua/v2/user/sub-api-key/expiring", a.meshSubKey(a.expiringSubAPIKeys))
	      g.GET("/ua/v2/user/sub-api-key/locked", a.meshSubKey(a.lockedSubAPIKeys))
	      g.GET("/ua/v2/user/sub-api-key/by-fp", a.meshSubKey(a.fingerprintSubAPIKey))
	      g.POST("/ua/v1/user/create-sub-api-key", a.meshSubKey(a.addSubAPIKey))
	  g.POST("/ua/v1/user/modify-sub-api-key", a.meshSubKey(a.modifySubAPIKey))
	         g.POST("/ua/v1/user/sub-api-key/status", a.meshSubKey(a.statusSubAPIKey))
	           g.POST("/ua/v1/user/sub-api-key/disable-all", a.meshSubKey(a.disableAllSubAPIKeys))
	     g.POST("/ua/v1/user/sub-api-key/enable-all", a.meshSubKey(a.enableAllSubAPIKeys))
	       g.POST("/ua/v1/user/sub-api-key/rotate", a.meshSubKey(a.rotateSubAPIKey))
	  g.POST("/ua/v1/user/sub-api-key/passphrase", a.meshSubKey(a.passphraseSubAPIKey))
	   g.POST("/ua/v1/user/sub-api-key/sweep-expired", a.meshSubKey(a.sweepExpiredSubAPIKeys))
	  g.POST("/ua/v1/user/sub-api-key/sweep-idle", a.meshSubKey(a.sweepIdleSubAPIKeys))
	       g.POST("/ua/v1/user/sub-api-key/sweep-grace", a.meshSubKey(a.sweepGraceSubAPIKeys))
	      g.POST("/ua/v1/user/sub-api-key/sweep-audit", a.meshSubKey(a.sweepAuditSubAPIKeys))
	   g.POST("/ua/v1/user/sub-api-key/unlock", a.meshSubKey(a.unlockSubAPIKey))
	   g.POST("/ua/v1/user/sub-api-key/revoke", a.meshSubKey(a.revokeSubAPIKey))
	  g.POST("/ua/v1/user/sub-api-key/revoke-all", a.meshSubKey(a.revokeAllSubAPIKeys))
	      g.POST("/ua/v1/user/sub-api-key/restore", a.meshSubKey(a.restoreSubAPIKey))
	   g.POST("/ua/v1/user/sub-api-key/purge-revoked", a.meshSubKey(a.purgeRevokedSubAPIKeys))
	           g.POST("/ua/v1/user/sub-api-key/clone", a.meshSubKey(a.cloneSubAPIKey))
	      g.POST("/ua/v1/user/sub-api-key/reset-usage", a.meshSubKey(a.resetUsageSubAPIKey))
	      g.POST("/ua/v1/user/sub-api-key/revoke-grace", a.meshSubKey(a.revokeGraceSubAPIKey))
	   g.DELETE("/ua/v1/user/sub-api-key", a.meshSubKey(a.deleteSubAPIKey))
	     g.GET("/ua/v1/sub/api-key", a.meshSubKey(a.listSubAPIKeys))
	  g.GET("/ua/v1/user/get-sub-api-key", a.meshSubKey(a.listSubAPIKeys))
	           g.GET("/ua/v1/user/sub-api-key/detail", a.meshSubKey(a.detailSubAPIKey))
	    g.GET("/ua/v1/user/sub-api-key/quota", a.meshSubKey(a.quotaSubAPIKeys))
	       g.GET("/ua/v1/user/sub-api-key/audit", a.meshSubKey(a.auditSubAPIKeys))
	  g.GET("/ua/v1/user/sub-api-key/usage", a.meshSubKey(a.usageSubAPIKey))
	    g.GET("/ua/v1/user/sub-api-key/probe", a.meshSubKey(a.probeSubAPIKey))
	      g.GET("/ua/v1/user/sub-api-key/stats", a.meshSubKey(a.statsSubAPIKeys))
	        g.GET("/ua/v1/user/sub-api-key/expiring", a.meshSubKey(a.expiringSubAPIKeys))
	   g.GET("/ua/v1/user/sub-api-key/locked", a.meshSubKey(a.lockedSubAPIKeys))
	     g.GET("/ua/v1/user/sub-api-key/by-fp", a.meshSubKey(a.fingerprintSubAPIKey))
	         g.POST("/ua/v2/unified/account/modify-leverage", a.utaModifyLeverage)
   }

   func (a *API) utaOverview(c *gin.Context) {
	   out, okB := a.booksOrFail(c)
	         if !okB {
	    	   return
	   }
	      ok(c, gin.H{"accountType": "UTA", "accounts": out})
  }

  func (a *API) utaBalance(c *gin.Context) {
	        out, okB := a.booksOrFail(c)
	  if !okB {
		   return
	      }
	  uid, okU := a.requireUser(c)
	     if !okU {
		   return
	   }
	     book := a.utaBook()
	   raw := make([]map[string]string, 0, len(out))
	     for _, r := range out {
		   raw = append(raw, map[string]string{
	    		   "currency": r.Currency, "equity": r.Balance, "hold": r.Holds,
	  		  "balance": r.Balance, "available": r.Available,
		  })
	   }
	        rows := book.EnrichBalances(uid, raw)
	   ok(c, gin.H{"accountType": "UNIFIED", "accounts": []gin.H{{"currencies": rows}}})
   }

   func (a *API) utaBook() *utabook.Book {
	      if a != nil && a.rt != nil && a.rt.UTA != nil {
		  return a.rt.UTA
	       }
	     return utabook.New()
  }

  func (a *API) utaMode(c *gin.Context) {
	     uid, okU := a.requireUser(c)
	   if !okU {
	    	   return
	   }
	     mode, err := a.utaBook().RequireMode(uid)
	     if err != nil {
	        	  fail(c, 503, err.Error())
		  return
	        }
	  ok(c, gin.H{"accountMode": mode})
  }

   func (a *API) utaSetMode(c *gin.Context) {
	    var body struct {
	  	  AccountMode string `json:"accountMode"`
	   }
	         if err := c.ShouldBindJSON(&body); err != nil {
	   	  fail(c, 400, err.Error())
		  return
	     }
	      uid, okU := a.requireUser(c)
	   if !okU {
		  return
	       }
	     mode, err := a.utaBook().SetMode(uid, body.AccountMode)
	       if err != nil {
		  fail(c, 503, err.Error())
		  return
	  }
	    ok(c, gin.H{"accountMode": mode})
   }

  func (a *API) utaLeverage(c *gin.Context) {
	     uid, okU := a.requireUser(c)
	   if !okU {
		  return
	    }
	     rows, err := a.utaBook().RequireLeverage(uid, c.Query("symbol"), c.Query("currency"), c.Query("tradeType"), c.Query("marginMode"))
	  if err != nil {
	      	  fail(c, 503, err.Error())
	   	  return
	  }
	   ok(c, rows)
  }

   func (a *API) utaModifyLeverage(c *gin.Context) {
	         var body struct {
	  	   Symbol     string `json:"symbol"`
		  Currency   string `json:"currency"`
	   	   Leverage   string `json:"leverage"`
	    	  MarginMode string `json:"marginMode"`
		  TradeType string `json:"tradeType"`
	     }
	  if err := c.ShouldBindJSON(&body); err != nil {
		   fail(c, 400, err.Error())
	    	   return
	       }
	      uid, okU := a.requireUser(c)
	         if !okU {
		   return
	     }
	       got, err := a.utaBook().SetLeverage(uid, utabook.Leverage{
	  	   Symbol: body.Symbol, Currency: body.Currency, Leverage: body.Leverage,
		   MarginMode: body.MarginMode, TradeType: body.TradeType,
	      })
	  if err != nil {
	  	  fail(c, 503, err.Error())
	      	  return
	        }
	  ok(c, gin.H{"leverage": got.Leverage, "symbol": got.Symbol, "marginMode": got.MarginMode})
   }

  func (a *API) utaInterestLimits(c *gin.Context) {
	  lim, err := a.utaBook().RequireInterestLimits(c.Query("currency"))
	  if err != nil {
		  fail(c, 503, err.Error())
	   	  return
	         }
	    ok(c, lim)
  }

  func (a *API) utaInterestHistory(c *gin.Context) {
	       uid, okU := a.requireUser(c)
	      if !okU {
	   	   return
	     }
	     ok(c, a.utaBook().InterestHistory(uid, c.Query("currency")))
   }
