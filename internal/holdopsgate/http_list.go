package holdopsgate

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

  func registerListRoutes(g *gin.RouterGroup, svc *Service) {
	   g.GET("/lease/:id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  got, err := svc.GetLease(c.Request.Context(), c.Param("id"))
		   if err != nil {
			  c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			  return
		  }
		   c.JSON(http.StatusOK, got)
	   })
	  g.GET("/proof/:id", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  got, err := svc.GetProof(c.Request.Context(), c.Param("id"))
		  if err != nil {
			  c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			   return
		   }
		   c.JSON(http.StatusOK, got)
	  })
	  g.GET("/generation", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		   got, err := svc.GetGeneration(c.Request.Context())
		   if err != nil {
			  c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			   return
		  }
		   c.JSON(http.StatusOK, got)
	   })
	   g.GET("/export/:id", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			  return
		  }
		  got, err := ExportProof(svc, c.Request.Context(), c.Param("id"))
		  if err != nil {
			   c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			  return
		   }
		  c.JSON(http.StatusOK, got)
	  })
	  g.GET("/stats", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		  }
		  c.JSON(http.StatusOK, svc.Stats(c.Request.Context()))
	  })
	  g.GET("/token/:id", func(c *gin.Context) {
		  if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			  c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  got, err := svc.GetToken(c.Request.Context(), c.Param("id"))
		  if err != nil {
			  c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			  return
		   }
		  c.JSON(http.StatusOK, got)
	   })
	   g.GET("/gap/:id", func(c *gin.Context) {
		   if err := perm.FromRequest(c.Request, "").Check(perm.ActQueryRead); err != nil {
			   c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			   return
		   }
		  got, err := svc.GetGap(c.Request.Context(), c.Param("id"))
		   if err != nil {
			  c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			  return
		   }
		   c.JSON(http.StatusOK, got)
	  })
   }
