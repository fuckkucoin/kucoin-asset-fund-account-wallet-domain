// HTTP 面：对外路由与请求编排。

// Author: Kian
// Date: 2021.11.24

package subacct

import (
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/xferloop"
	"github.com/gin-gonic/gin"
)

   func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	  var db *persist.SQLDB
	  if rt != nil && !rt.Mem {
		   db = rt.SQL
	  }
	  svc := NewWithKV(rt.KV).BindSQL(db)
	  if rt != nil {
		  svc.BindBooks(rt.Ledger)
	  }
	  v1.POST("/subacct/parents", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body OpenParentReq
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		  uid, err := callerid.Bind(c, body.UserID, callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		  body.UserID = uid
		   got, err := svc.OpenParent(body)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  c.JSON(200, got)
	  })
	   v1.POST("/subacct/children", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body OpenChildReq
		  if !mw.BindJSON(c, &body) {
			   return
		  }
		  got, err := svc.OpenChild(body)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, got)
	   })
	  v1.POST("/subacct/freeze", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountFreeze); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			  ParentID string `json:"parent_id"`
			  ChildID string `json:"child_id"`
			   Frozen  *bool  `json:"frozen"`
			  Reason  string `json:"reason"`
			   Cascade bool   `json:"cascade_children"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		   }
		  if body.Frozen == nil {
			   c.JSON(400, gin.H{"error": "frozen required"})
			   return
		  }
		   actor := perm.FromRequest(c.Request, "").ID
		  if body.ParentID != "" && !*body.Frozen && body.Cascade {
			   if err := svc.RejectChildCascade(body.ParentID, actor); err != nil {
				   mw.WriteProblem(c, err)
				   return
			  }
		  }
		  if body.ParentID != "" {
			  if *body.Frozen {
				  got,
					   err := svc.FreezeParent(body.ParentID, body.Reason)
				  if err != nil {
					   mw.WriteProblem(c, err)
					  return
				  }
				   c.JSON(200, got)
				  return
			   }
			   got, err := svc.UnfreezeParent(body.ParentID)
			   if err != nil {
				  mw.WriteProblem(c, err)
				  return
			   }
			   c.JSON(200, got)
			   return
		  }
		   if *body.Frozen {
			  got,
				  err := svc.FreezeChild(body.ChildID, body.Reason)
			  if err != nil {
				  mw.WriteProblem(c, err)
				  return
			  }
			   c.JSON(200, got)
			  return
		  }
		   got, err := svc.UnfreezeChild(body.ChildID)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, got)
	   })
	   v1.POST("/subacct/credit", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   var body PostReq
		   if !mw.BindJSON(c, &body) {
			  return
		  }
		  got, err := svc.Credit(body)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, got)
	   })
	  v1.POST("/subacct/debit", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   var body PostReq
		  if !mw.BindJSON(c, &body) {
			  return
		   }
		   got, err := svc.Debit(body)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, got)
	   })
	   v1.POST("/subacct/hops", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   var body HopReq
		  if !mw.BindJSON(c, &body) {
			  return
		  }
		   got, err := svc.Hop(c.Request.Context(), body)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  c.JSON(200, got)
	  })
	  v1.GET("/subacct/snapshot", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		   got, err := svc.Snapshot(c.Query("parent_id"))
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, got)
	   })
	   v1.GET("/subacct/freeze-gate", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		   owner := c.Query("owner_id")
		  action := c.Query("action")
		  if action == "" {
			   c.JSON(200, gin.H{"actions": FreezeActions(), "audits": svc.FreezeAudits(c.Query("parent_id"), 50)})
			   return
		  }
		  got, err := svc.PreviewFreeze(owner, action)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   c.JSON(200, gin.H{"preview": got, "actions": FreezeActions()})
	  })
	   v1.GET("/subacct/freeze-audit", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		   }
		   c.JSON(200, gin.H{"audits": svc.FreezeAudits(c.Query("parent_id"), 50)})
	   })
	  v1.GET("/subacct/hops", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  hops := svc.ListHops(c.Query("owner_id"), 50)
		   items := make([]gin.H, 0, len(hops))
		  for _, h := range hops {
			   st := "SUCCESS"
			   if h.Reversed {
				  st = "FAILURE"
			  }
			  if s, ok := ledger.LoopRailKey(h.Idem); ok {
				   st = s
			   } else if s, ok := xferloop.RailKey(h.Idem); ok {
				  st = s
			   }
			  items = append(items, gin.H{
				  "id": h.ID, "idempotency_key": h.Idem, "from_owner": h.FromOwner, "to_owner": h.ToOwner,
				   "from_user": h.FromUser, "to_user": h.ToUser, "from_book": h.FromBook, "to_book": h.ToBook,
				  "asset": h.Asset, "amount": h.Amount, "kind": h.Kind, "reversed": h.Reversed,
				   "transfer_id": h.TransferID, "created_at": h.CreatedAt, "status": st,
			   })
		  }
		  c.JSON(200, gin.H{
			   "hops": items,
			  "host": "account-book.kucoin.net",
			   "dsn":  persist.DSNFor(persist.RegionSG),
		  })
	   })
	  v1.POST("/subacct/transfer-quota", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   var body struct {
			  ParentID string `json:"parent_id"`
			   Asset      string `json:"asset"`
			   DailyLimit string `json:"daily_limit"`
			  PerTxLimit string `json:"per_tx_limit"`
		  }
		   if !mw.BindJSON(c, &body) {
			  return
		   }
		   got, err := svc.SetTransferQuota(body.ParentID, body.Asset, body.DailyLimit, body.PerTxLimit)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, got)
	   })
	   v1.GET("/subacct/transfer-quota", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  got, err := svc.GetTransferQuota(c.Query("parent_id"), c.Query("asset"))
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, got)
	   })
	   v1.POST("/subacct/flex-transfer", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		  var body struct {
			   Type string `json:"type"`
			   ParentID string `json:"parent_id"`
			   SubID    string `json:"sub_id"`
			   Asset    string `json:"asset"`
			   Amount         string `json:"amount"`
			   IdempotencyKey string `json:"idempotency_key"`
		  }
		   if !mw.BindJSON(c, &body) {
			   return
		  }
		  got, err := svc.FlexTransfer(c.Request.Context(), body.Type, body.ParentID, body.SubID, body.Asset, body.Amount, body.IdempotencyKey)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, got)
	   })
	v1.POST("/subacct/children/close", func(c *gin.Context) {
		if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			c.JSON(403, gin.H{"error": err.Error()})
			return
		}
		var body struct {
			ChildID string `json:"child_id"`
		}
		if !mw.BindJSON(c, &body) {
			return
		}
		got, err := svc.CloseChild(body.ChildID)
		if err != nil {
			mw.WriteProblem(c, err)
			return
		}
		c.JSON(200, got)
	})
	v1.POST("/subacct/privileges", func(c *gin.Context) {
		if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			c.JSON(403, gin.H{"error": err.Error()})
			return
		}
		var body struct {
			ChildID    string   `json:"child_id"`
			Privileges []string `json:"privileges"`
		}
		if !mw.BindJSON(c, &body) {
			return
		}
		privs := make([]Privilege, 0, len(body.Privileges))
		for _, raw := range body.Privileges {
			p, err := ParsePrivilege(raw)
			if err != nil {
				mw.WriteProblem(c, err)
				return
			}
			privs = append(privs, p)
		}
		got, err := svc.SetPrivileges(body.ChildID, privs)
		if err != nil {
			mw.WriteProblem(c, err)
			return
		}
		c.JSON(200, got)
	})
	v1.POST("/subacct/label", func(c *gin.Context) {
		if err := perm.FromRequest(c.Request, "").Check(perm.ActAccountMove); err != nil {
			c.JSON(403, gin.H{"error": err.Error()})
			return
		}
		var body struct {
			ChildID string `json:"child_id"`
			Label   string `json:"label"`
		}
		if !mw.BindJSON(c, &body) {
			return
		}
		got, err := svc.SetLabel(body.ChildID, body.Label)
		if err != nil {
			mw.WriteProblem(c, err)
			return
		}
		c.JSON(200, got)
	})
   }
