package balproofgate

import (
	"net/http"
	"strings"
	"account-assets.kucoin.net/funding/internal/mw"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

   func registerListRoutes(g *gin.RouterGroup, svc *Service) {
	  g.GET("/leases", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		   user := strings.TrimSpace(c.Query("user_id"))
		   book := strings.TrimSpace(c.Query("book"))
		   asset := strings.TrimSpace(c.Query("asset"))
		  if user == "" || book == "" || asset == "" {
			  c.JSON(http.StatusBadRequest, gin.H{"error": "user_id, book, asset required"})
			  return
		  }
		  rows, err := svc.Store().ListLeasesBySubject(c.Request.Context(), user, book, asset)
		   if err != nil {
			   mw.WriteProblem(c, err)
			   return
		  }
		   c.JSON(http.StatusOK, gin.H{"leases": rows, "isolation_mark": IsolationMark})
	  })
	  g.GET("/drifts", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		   }
		   user := strings.TrimSpace(c.Query("user_id"))
		  book := strings.TrimSpace(c.Query("book"))
		   asset := strings.TrimSpace(c.Query("asset"))
		  rows, err := svc.Store().ListBlockingDrifts(c.Request.Context(), user, book, asset)
		   if err != nil {
			   mw.WriteProblem(c, err)
			  return
		   }
		  c.JSON(http.StatusOK, gin.H{"drifts": rows})
	  })
   }
