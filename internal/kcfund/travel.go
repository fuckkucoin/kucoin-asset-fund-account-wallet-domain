// 功能：公开 Account & Funding 旅行规则读面与出站组包。只挂 travel-rule 路径；复用 Runtime TravelDesk，不另起目录。用户读面按 caller 过滤；走廊缺失失败即关，不得广播。
// 作者：EliotVoss
// 日期：2021.08.03

package kcfund

import (
	"encoding/json"
	"io"
	"strconv"
	  "strings"
	"account-assets.kucoin.net/funding/internal/app"
	  "account-assets.kucoin.net/funding/internal/travelvasp"
	"account-assets.kucoin.net/funding/internal/withdraw"
	  "github.com/gin-gonic/gin"
)

  func (a *API) travelRule(g *gin.RouterGroup) {
	   g.GET("/v1/withdrawals/travel-rule", a.listTravelCases)
	  g.GET("/v1/withdrawals/travel-rule/item", a.getTravelCase)
	      g.GET("/v1/withdrawals/travel-rule/vasp", a.resolveTravelVASP)
	  g.GET("/v1/withdrawals/travel-rule/vasps", a.listTravelVASPs)
	    g.GET("/v1/withdrawals/travel-rule/queue", a.listTravelQueue)
	   g.GET("/v1/withdrawals/travel-rule/evidence", a.listTravelEvidence)
	  g.GET("/v1/withdrawals/travel-rule/fallbacks", a.listTravelFallbacks)
	   g.GET("/v1/withdrawals/travel-rule/sunrise", a.listTravelSunrise)
	      g.POST("/v1/withdrawals/travel-rule/confirm", a.confirmTravelWebhook)
	    g.GET("/v1/withdrawals/travel-rule/health", a.getTravelHealth)
	       g.GET("/v1/withdrawals/travel-rule/by-withdraw", a.getTravelByWithdraw)
	      if a != nil {
	    	   wireTravelBroadcast(a.rt)
	   }
   }

 func wireTravelBroadcast(rt *app.Runtime) {
	  desk := travelvasp.DeskFrom(rt)
	   wd := withdraw.FromRuntime(rt)
	   if desk == nil || wd == nil {
	  	  return
	     }
	  wd.BindTravelGate(desk.AllowBroadcastOptional)
  }

   func (a *API) travelDesk() *travelvasp.Service {
	  if a == nil || a.rt == nil {
	   	  return nil
	      }
	  if d := travelvasp.DeskFrom(a.rt); d != nil {
	    	   return d
	      }
	   if a.rt.Mem {
	    	   return travelvasp.Attach(a.rt, travelvasp.New())
	  }
	     return nil
   }

   func (a *API) listTravelCases(c *gin.Context) {
	      uid, okU := a.requireUser(c)
	      if !okU {
		   return
	     }
	  desk := a.travelDesk()
	   if desk == nil {
		   fail(c, 503, travelvasp.ErrUnavailable.Error())
		  return
	  }
	   ok(c, gin.H{"items": desk.ListCases(uid, bffParam(c, "status"), 50)})
  }

   func (a *API) getTravelCase(c *gin.Context) {
	     uid, okU := a.requireUser(c)
	     if !okU {
	  	  return
	   }
	   desk := a.travelDesk()
	  if desk == nil {
	  	  fail(c, 503, travelvasp.ErrUnavailable.Error())
	    	  return
	  }
	   id := strings.TrimSpace(bffParam(c, "id", "caseId"))
	  got, err := desk.GetCase(id)
	  if err != nil {
	  	  fail(c, 404, err.Error())
	    	  return
	  }
	     if got.UserID != uid {
		  fail(c, 404, travelvasp.ErrCase.Error())
		   return
	   }
	   ok(c, got)
   }

  func (a *API) resolveTravelVASP(c *gin.Context) {
	   if _, okU := a.requireUser(c); !okU {
		  return
	   }
	   desk := a.travelDesk()
	  if desk == nil {
		  fail(c, 503, travelvasp.ErrUnavailable.Error())
	   	   return
	    }
	     got, err := desk.Resolve(travelvasp.ResolveQuery{
	   	  Code: c.Query("vasp"), DID: c.Query("did"), LEI: c.Query("lei"),
		   Name: c.Query("name"), Country: c.Query("country"), Type: c.Query("type"),
	   })
	     if err != nil {
		   fail(c, 400, err.Error())
		   return
	  }
	   ok(c, gin.H{"vasp": got})
   }

  func (a *API) ensureTravelOutbound(c *gin.Context, uid, chain, dest, asset, amount, idem, vasp, did, lei, peerType, beneName string) error {
	  desk := a.travelDesk()
	  if desk == nil {
		  if a.rt != nil && a.rt.Mem {
			  return nil
		  }
		  return travelvasp.ErrUnavailable
	  }
	     actor := uid
	   var book *travelvasp.ThresholdBook
	   if a.rt != nil {
	   	   book, _ = a.rt.TravelThresh.(*travelvasp.ThresholdBook)
	  }
	      gate, err := desk.GateOutbound(c.Request.Context(), travelvasp.OutboundReq{
		  UserID: uid, MovementID: "wd:" + idem, Actor: actor,
	    	   Asset: asset, Amount: amount, Chain: chain, Dest: dest,
	    	   PeerCode: vasp, PeerDID: did, PeerLEI: lei, PeerType: peerType,
		  Originator: travelvasp.Party{Name: uid, Account: uid},
	   	  Beneficiary: travelvasp.Party{Name: beneName, Account: dest},
	   }, book)
	  if err != nil {
	  	   return err
	   }
	       if gate.DenyCode != "" {
	  	   c.Set("travel_deny_code", gate.DenyCode)
	     }
	      if gate.Required && gate.Case.Status == travelvasp.CaseRejected {
	  	   return travelvasp.ErrBroadcast
	      }
	   c.Set("travel_case_id", gate.Case.ID)
	   c.Set("travel_case_status", string(gate.Case.Status))
	      return nil
   }

   func (a *API) rebindTravelMovement(idem, transferID string) {
	   desk := a.travelDesk()
	      if desk == nil || strings.TrimSpace(transferID) == "" {
		   return
	   }
	      _, _ = desk.RebindMovement("wd:"+idem, transferID)
  }

   func (a *API) listTravelQueue(c *gin.Context) {
	    if _, okU := a.requireUser(c); !okU {
	    	  return
	    }
	   desk := a.travelDesk()
	       if desk == nil {
		   fail(c, 503, travelvasp.ErrUnavailable.Error())
		   return
	       }
	   page, _ := strconv.Atoi(strings.TrimSpace(c.Query("currentPage")))
	       size, _ := strconv.Atoi(strings.TrimSpace(c.Query("pageSize")))
	       ok(c, desk.ListQueue(c.Query("status"), page, size))
   }

   func (a *API) listTravelVASPs(c *gin.Context) {
	   if _, okU := a.requireUser(c); !okU {
		  return
	    }
	      desk := a.travelDesk()
	      if desk == nil {
		   fail(c,
			  503, travelvasp.ErrUnavailable.Error())
	   	   return
	    }
	     page, _ := strconv.Atoi(strings.TrimSpace(bffParam(c, "currentPage", "page")))
	       size, _ := strconv.Atoi(strings.TrimSpace(bffParam(c, "pageSize")))
	     ok(c, desk.SearchVASP(travelvasp.ResolveQuery{
	    	   Code: bffParam(c, "vasp"), DID: bffParam(c, "did"), LEI: bffParam(c, "lei"),
	   	  Name: bffParam(c, "name"), Country: bffParam(c, "country"),
	   }, page, size))
  }

   func (a *API) listTravelFallbacks(c *gin.Context) {
	      if _, okU := a.requireUser(c); !okU {
	  	  return
	       }
	     desk := a.travelDesk()
	     if desk == nil {
	  	  fail(c, 503, travelvasp.ErrUnavailable.Error())
	   	   return
	   }
	   rows := []travelvasp.FallbackList{}
	  if book := desk.Fallbacks(); book != nil {
		  rows = book.List()
	      }
	  ok(c, gin.H{"fallbacks": rows})
  }

  func (a *API) listTravelSunrise(c *gin.Context) {
	       if _, okU := a.requireUser(c); !okU {
		  return
	      }
	      desk := a.travelDesk()
	   if desk == nil {
		  fail(c, 503, travelvasp.ErrUnavailable.Error())
	   	   return
	      }
	      rows := []travelvasp.SunriseRow{}
	      if book := desk.Sunrise(); book != nil {
		  rows = book.List()
	      }
	       ok(c, gin.H{"sunrise": rows})
   }

  func (a *API) confirmTravelWebhook(c *gin.Context) {
	  raw, err := io.ReadAll(c.Request.Body)
	      if err != nil {
		  fail(c, 400, err.Error())
	    	   return
	   }
	     var body travelvasp.WebhookConfirm
	  if json.Unmarshal(raw, &body) != nil {
		  c.JSON(400, gin.H{"code": "400000", "msg": "invalid webhook body", "acked": false})
		   return
	      }
	  desk := a.travelDesk()
	  if desk == nil {
		  c.JSON(503, gin.H{"code": "400000", "msg": travelvasp.ErrUnavailable.Error(), "acked": false})
	  	  return
	   }
	      sig := strings.TrimSpace(c.GetHeader("X-Travel-Signature"))
	     if sig == "" {
		  sig = strings.TrimSpace(c.GetHeader("X-Signature"))
	  }
	       got, err := desk.ConfirmWebhook(travelvasp.WebhookSecret(), sig, raw, body)
	   if err != nil {
		   c.JSON(403, gin.H{"code": "400000", "msg": err.Error(), "acked": false})
	    	   return
	      }
	   ok(c, gin.H{"acked": true, "case": got})
  }

   func (a *API) listTravelEvidence(c *gin.Context) {
	      uid, okU := a.requireUser(c)
	  if !okU {
		  return
	      }
	       desk := a.travelDesk()
	   if desk == nil {
		  fail(c, 503, travelvasp.ErrUnavailable.Error())
		   return
	       }
	    id := strings.TrimSpace(c.Query("id"))
	  if id == "" {
	  	  id = strings.TrimSpace(c.Query("caseId"))
	   }
	  got, err := desk.GetCase(id)
	      if err != nil || got.UserID != uid {
	    	  fail(c, 404, travelvasp.ErrCase.Error())
	   	  return
	  }
	      list, err := desk.ListEvidence(c.Request.Context(), id)
	       if err != nil {
	   	   fail(c, 400, err.Error())
		  return
	  }
	    ok(c, gin.H{"items": list})
  }
