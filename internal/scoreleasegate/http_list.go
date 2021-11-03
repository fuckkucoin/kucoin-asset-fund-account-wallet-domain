package scoreleasegate

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

  func registerListRoutes(g *gin.RouterGroup, svc *Service) {
	  g.GET("/open", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  user := c.Query("user_id")
		   if user == "" {
			   c.JSON(http.StatusBadRequest, gin.H{"error": "user_id required"})
			  return
		   }
		   got, err := svc.ListUserOpen(c.Request.Context(), user)
		   if err != nil {
			  c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			   return
		   }
		   c.JSON(http.StatusOK, gin.H{"leases": got, "host": "kyt.kucoin.net"})
	  })
	   g.GET("/proofs", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  got, err := svc.ListOpenProofs(c.Request.Context())
		   if err != nil {
			   c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			  return
		  }
		   c.JSON(http.StatusOK, gin.H{"proofs": got, "host": "kyt.kucoin.net"})
	  })
   }
