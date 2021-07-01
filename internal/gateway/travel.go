package gateway

import (
	"strconv"
	"strings"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/travelvasp"
	"github.com/gin-gonic/gin"
)

   func registerTravel(v1 *gin.RouterGroup, rt *app.Runtime) {
	  if v1 == nil {
		  return
	  }
	  desk := travelvasp.DeskFrom(rt)
	  if desk == nil {
		   desk = travelvasp.Attach(rt, travelvasp.New())
	  }
	  v1.GET("/funding/travel/cases", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   if err2 := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err2 != nil {
				  c.JSON(403, gin.H{"error": err.Error()})
				   return
			  }
		  }
		  uid := strings.TrimSpace(c.Query("user_id"))
		  ops := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead) == nil
		   if !ops {
			  uid = callerid.Resolve(c, idOpt(rt))
		  } else if uid == "" {
			  uid = callerid.Resolve(c, idOpt(rt))
		  }
		  c.JSON(200, gin.H{"cases": desk.ListCases(uid, c.Query("status"), 50), "host": "query-gateway.kucoin.net"})
	   })
	   v1.GET("/funding/travel/cases/:id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   if err2 := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err2 != nil {
				  c.JSON(403, gin.H{"error": err.Error()})
				  return
			   }
		  }
		   got, err := desk.GetCase(c.Param("id"))
		  if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   uid := callerid.Resolve(c, idOpt(rt))
		   if got.UserID != uid && perm.FromRequest(c.Request, "").Check(perm.ActCaseRead) != nil {
			   c.JSON(403, gin.H{"error": "travel-rule case forbidden"})
			   return
		   }
		   c.JSON(200, got)
	   })
	  v1.GET("/funding/travel/vasp", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  got, err := desk.Resolve(travelvasp.ResolveQuery{
			  Code: c.Query("code"), DID: c.Query("did"), LEI: c.Query("lei"),
			  Name: c.Query("name"), Country: c.Query("country"), Type: c.Query("type"),
		  })
		   if err != nil {
			  mw.WriteProblem(c, err)
			  return
		   }
		   c.JSON(200, gin.H{"vasp": got, "host": "query-gateway.kucoin.net"})
	   })
	  v1.GET("/funding/travel/vasps", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			   return
		  }
		   page, _ := strconv.Atoi(strings.TrimSpace(c.Query("currentPage")))
		   size, _ := strconv.Atoi(strings.TrimSpace(c.Query("pageSize")))
		   c.JSON(200, gin.H{
			  "page": desk.SearchVASP(travelvasp.ResolveQuery{
				   Code: c.Query("code"), DID: c.Query("did"), LEI: c.Query("lei"),
				  Name: c.Query("name"), Country: c.Query("country"),
			  }, page, size),
			  "host": "query-gateway.kucoin.net",
		  })
	  })
	   v1.GET("/funding/travel/policies", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   if err2 := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err2 != nil {
				  c.JSON(403, gin.H{"error": err.Error()})
				  return
			  }
		   }
		   rows := []travelvasp.CorridorPolicy{}
		  if book := desk.Policies(); book != nil {
			   rows = book.List()
		  }
		   c.JSON(200, gin.H{"policies": rows, "host": "query-gateway.kucoin.net"})
	   })
	  v1.GET("/funding/travel/evidence", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			  if err2 := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err2 != nil {
				  c.JSON(403, gin.H{"error": err.Error()})
				   return
			   }
		   }
		   id := strings.TrimSpace(c.Query("id"))
		   if id == "" {
			   id = c.Query("case_id")
		   }
		   got, err := desk.GetCase(id)
		   if err != nil {
			  mw.WriteProblem(c, err)
			   return
		   }
		   uid := callerid.Resolve(c, idOpt(rt))
		  if got.UserID != uid && perm.FromRequest(c.Request, "").Check(perm.ActCaseRead) != nil {
			   c.JSON(403, gin.H{"error": "travel-rule case forbidden"})
			  return
		  }
		  list, err := desk.ListEvidence(c.Request.Context(), id)
		  if err != nil {
			   mw.WriteProblem(c, err)
			  return
		  }
		   c.JSON(200, gin.H{"items": list, "host": "query-gateway.kucoin.net"})
	  })
	   v1.GET("/funding/travel/fallbacks", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  rows := []travelvasp.FallbackList{}
		  if book := desk.Fallbacks(); book != nil {
			   rows = book.List()
		   }
		  c.JSON(200, gin.H{"fallbacks": rows, "host": "query-gateway.kucoin.net"})
	  })
	  v1.GET("/funding/travel/sunrise", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(403, gin.H{"error": err.Error()})
			  return
		  }
		  rows := []travelvasp.SunriseRow{}
		   if book := desk.Sunrise(); book != nil {
			   rows = book.List()
		   }
		  c.JSON(200, gin.H{"sunrise": rows, "host": "query-gateway.kucoin.net"})
	  })
	   v1.GET("/funding/travel/sla", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			  if err2 := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err2 != nil {
				   c.JSON(403, gin.H{"error": err.Error()})
				  return
			  }
		  }
		   if id := strings.TrimSpace(c.Query("id")); id != "" {
			   got, err := desk.InspectSLA(id)
			   if err != nil {
				   mw.WriteProblem(c, err)
				   return
			  }
			   c.JSON(200, got)
			   return
		   }
		  c.JSON(200, gin.H{"items": desk.ListSLA(c.Query("status"), 50), "host": "query-gateway.kucoin.net"})
	  })
	   v1.GET("/funding/travel/health", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(403, gin.H{"error": err.Error()})
			  return
		   }
		  c.JSON(200, gin.H{"health": desk.InspectPeerHealth(c.Query("code")), "host": "query-gateway.kucoin.net"})
	  })
	  v1.GET("/funding/travel/by-withdraw", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			  if err2 := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err2 != nil {
				   c.JSON(403, gin.H{"error": err.Error()})
				  return
			   }
		   }
		  got, err := desk.CaseByWithdraw(c.Request.Context(), c.Query("withdraw_id"))
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		   }
		   uid := callerid.Resolve(c, idOpt(rt))
		   if got.UserID != uid && perm.FromRequest(c.Request, "").Check(perm.ActCaseRead) != nil {
			   c.JSON(403, gin.H{"error": "travel-rule case forbidden"})
			   return
		  }
		   c.JSON(200, gin.H{"case": got, "host": "query-gateway.kucoin.net"})
	  })
	  v1.GET("/funding/travel/queue", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActCaseRead); err != nil {
			  if err2 := perm.FromRequest(c.Request, "").Check(perm.ActOpsAdmin); err2 != nil {
				  c.JSON(403, gin.H{"error": err.Error()})
				   return
			  }
		   }
		  page, _ := strconv.Atoi(strings.TrimSpace(c.Query("currentPage")))
		  size, _ := strconv.Atoi(strings.TrimSpace(c.Query("pageSize")))
		  c.JSON(200, gin.H{"queue": desk.ListQueue(c.Query("status"), page, size), "host": "query-gateway.kucoin.net"})
	   })
  }
