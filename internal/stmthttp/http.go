package stmthttp

import (
	"strconv"
	"time"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/stmt"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/gin-gonic/gin"
)

  func Register(v1 *gin.RouterGroup, rt *app.Runtime) {
	   v1.GET("/stmt", func(c *gin.Context) {
		   rawUID, err := callerid.Bind(c, c.Query("user_id"), callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  uid, err := validate.UserID(rawUID)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   bills, err := rt.Ledger.ListBills(c.Request.Context(), uid, validate.Limit(atoi(c.Query("limit")), 100, 500))
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   rep, err := stmt.Build(uid, bills, stmt.Filter{Asset: c.Query("asset"), Book: c.Query("book")})
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, gin.H{"report": rep, "host": "query-gateway.kucoin.net"})
	   })
	  v1.GET("/stmt/page", func(c *gin.Context) {
		   rawUID, err := callerid.Bind(c, c.Query("user_id"), callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  uid, err := validate.UserID(rawUID)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  after, err := stmt.ParseAfter(c.Query("after"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   bills, err := rt.Ledger.ListBills(c.Request.Context(), uid, 500)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   page, err := stmt.PageAfter(uid, bills, stmt.Filter{}, after, validate.Limit(atoi(c.Query("limit")), 50, 200))
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, page)
	  })
	  v1.GET("/stmt/export", func(c *gin.Context) {
		  rawUID, err := callerid.Bind(c, c.Query("user_id"), callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  uid, err := validate.UserID(rawUID)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  format, err := stmt.ParseFormat(c.DefaultQuery("format", "json"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   bills, err := rt.Ledger.ListBills(c.Request.Context(), uid, 200)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		  rep, err := stmt.Build(uid, bills, stmt.Filter{})
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   exp, err := stmt.ExportReport(rep, format)
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, exp)
	  })
	  v1.GET("/stmt/tax", func(c *gin.Context) {
		   rawUID, err := callerid.Bind(c, c.Query("user_id"), callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   uid,
			   err := validate.UserID(rawUID)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   year := time.Now().UTC().Year()
		  if y := c.Query("year"); y != "" {
			   n, err := strconv.Atoi(y)
			  if err != nil {
				  mw.WriteProblem(c, err)
				  return
			  }
			  year = n
		  }
		  bills, err := rt.Ledger.ListBills(c.Request.Context(), uid, 500)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		  rep, err := stmt.BuildTaxYear(uid, stmt.BillsWithTime(bills, time.Now().UTC()), year)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  c.JSON(200, gin.H{"tax": rep, "note": rep.Note})
	   })
	  v1.GET("/stmt/manifest", func(c *gin.Context) {
		  rawUID, err := callerid.Bind(c, c.Query("user_id"), callerid.FromRuntime(rt.Mem, rt.Cfg.SeedUserID, rt.Cfg.Auth.Kind, rt.Cfg.Profile))
		  if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		  uid, err := validate.UserID(rawUID)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   bills, err := rt.Ledger.ListBills(c.Request.Context(), uid, 200)
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		  rep, err := stmt.Build(uid, bills, stmt.Filter{})
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		   js, err := stmt.ExportReport(rep, stmt.FormatJSON)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   csv, err := stmt.ExportReport(rep, stmt.FormatCSV)
		  if err != nil {
			  mw.WriteProblem(c, err)
			   return
		  }
		  man, err := stmt.BuildManifest(uid, []stmt.Export{js, csv})
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(200, man)
	  })
  }

  func atoi(s string) int {
	  n, _ := strconv.Atoi(s)
	  return n
  }
