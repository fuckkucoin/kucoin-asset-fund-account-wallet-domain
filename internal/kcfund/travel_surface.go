// 公开旅行规则 REST：门槛查询、出站报文开立/提交；出金写路径见 travel_withdraw。

// Author: TateVoss
// Date: 2021.08.03

package kcfund

import (
	"net/http"
	"strings"
	  "account-assets.kucoin.net/funding/internal/travelvasp"
	"github.com/gin-gonic/gin"
)

   func (a *API) registerTravel(g *gin.RouterGroup) {
	      g.GET("/v1/travel/requirements", a.travelRequirements)
	   g.POST("/v1/travel/outbound", a.travelOpenOutbound)
	       g.GET("/v1/travel/outbound/:id", a.travelGetOutbound)
	     g.POST("/v1/travel/outbound/:id/submit", a.travelSubmitOutbound)
	   g.GET("/v1/travel/vasps/:code", a.travelGetVASP)
  }

  func (a *API) travelRequirements(c *gin.Context) {
	     if a.forwardMeshTravel(c, http.MethodGet, "/api/v1/travel/requirements", nil) {
	  	  return
	   }
	         uid, okU := a.requireUser(c)
	   if !okU {
	    	  return
	       }
	   view, err := a.travelGate().Requirements(travelvasp.RequirementsReq{
	      	  Asset: bffParam(c, "currency"),
	  	   Amount:          bffParam(c, "amount"),
	        	   BeneficiaryVASP: bffParam(c, "beneficiaryVasp"),
	   	   OriginCountry:   bffParam(c, "originCountry"),
	      	   PeerCountry:     bffParam(c, "peerCountry"),
	  })
	        if err != nil {
		  fail(c, 400, err.Error())
	    	  return
	         }
	      ok(c, gin.H{"user_id": uid, "requirements": view})
  }

   func (a *API) travelOpenOutbound(c *gin.Context) {
	    if a.forwardMeshTravel(c, http.MethodPost, "/api/v1/travel/outbound", nil) {
	    	   return
	       }
	    var body struct {
	  	   Currency string `json:"currency"`
		   Amount   string `json:"amount"`
	   	  Chain           string `json:"chain"`
	  	  ToAddress       string `json:"toAddress"`
	  	  BeneficiaryName string `json:"beneficiaryName"`
	      	   BeneficiaryVasp string `json:"beneficiaryVasp"`
	  	  ClientOid       string `json:"clientOid"`
	     }
	   if err := c.ShouldBindJSON(&body); err != nil {
	    	  fail(c, 400, err.Error())
	       	   return
	         }
	     uid, okU := a.requireUser(c)
	       if !okU {
		  return
	    }
	  msg, err := a.travelGate().OpenOutbound(c.Request.Context(), travelvasp.OpenOutboundReq{
	       	  UserID: uid, ClientOid: body.ClientOid,
		  Asset:  body.Currency,
	    	   Amount: body.Amount, Chain: body.Chain,
		   ToAddress: body.ToAddress, BeneficiaryName: body.BeneficiaryName,
	  	   BeneficiaryVASP: body.BeneficiaryVasp, Actor: "user:" + uid,
	   })
	       if err != nil {
		  fail(c, 400, err.Error())
	  	   return
	  }
	   ok(c, gin.H{"message": msg})
  }

  func (a *API) travelGetOutbound(c *gin.Context) {
	   id := strings.TrimSpace(c.Param("id"))
	       if a.forwardMeshTravel(c, http.MethodGet, "/api/v1/travel/outbound/"+id, nil) {
	    	  return
	  }
	    _, okU := a.requireUser(c)
	     if !okU {
	    	  return
	        }
	       msg, err := a.travelGate().GetOutbound(id)
	   if err != nil {
		  fail(c, 404, err.Error())
	  	   return
	      }
	       ok(c, gin.H{"message": msg})
   }

   func (a *API) travelSubmitOutbound(c *gin.Context) {
	  id := strings.TrimSpace(c.Param("id"))
	          if a.forwardMeshTravel(c, http.MethodPost, "/api/v1/travel/outbound/"+id+"/submit", nil) {
		  return
	  }
	      uid, okU := a.requireUser(c)
	  if !okU {
		   return
	         }
	     msg, err := a.travelGate().SubmitOutbound(id, "user:"+uid)
	      if err != nil {
		   fail(c, 400, err.Error())
	   	   return
	  }
	   ok(c, gin.H{"message": msg})
   }

  func (a *API) travelGetVASP(c *gin.Context) {
	      code := strings.TrimSpace(c.Param("code"))
	    v, err := a.travelGate().LookupVASP(code)
	   if err != nil {
	   	   fail(c, 404, err.Error())
		  return
	      }
	       ok(c, v)
   }
