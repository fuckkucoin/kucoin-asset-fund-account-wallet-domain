package gateway

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/apierr"
	      "account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/ledger"
	    "account-assets.kucoin.net/funding/internal/mw"
	  "account-assets.kucoin.net/funding/internal/validate"
	"github.com/gin-gonic/gin"
)

   func registerAssetsSurface(v1 *gin.RouterGroup, rt *app.Runtime, svc *Service) {
	   registerAdjunctRails(v1, rt)
	      v1.GET("/assets/overview", func(c *gin.Context) { writeOverview(c, rt, svc) })
	  v1.GET("/assets/valuation", func(c *gin.Context) { writeValuation(c, rt, svc) })
	         v1.GET("/assets/lite", func(c *gin.Context) { writeLite(c, rt) })
	  v1.GET("/assets/settings", func(c *gin.Context) { writeSettings(c, rt) })
	      v1.GET("/assets/accounts", func(c *gin.Context) { writeAccounts(c, rt) })
   }

   func writeSettings(c *gin.Context, rt *app.Runtime) {
	      user, err := validate.UserID(callerid.Resolve(c, idOpt(rt)))
	  if err != nil {
		  mw.WriteProblem(c, err)
	   	   return
	       }
	      opts, err := resolveDisplayOpts(c.Request.Context(), rt, user, "", "")
	     if err != nil {
	    	  mw.WriteProblem(c, err)
	    	  return
	     }
	   c.JSON(200, gin.H{
	   	   "user_id": user, "quote": opts.Quote, "hide_small": opts.HideSmall,
	    	   "hide_small_enabled": opts.Hide, "preference": opts.Pref,
	        	   "host": "query-gateway.kucoin.net",
	       })
   }

   func writeAccounts(c *gin.Context, rt *app.Runtime) {
	     user, err := validate.UserID(callerid.Resolve(c, idOpt(rt)))
	  if err != nil {
		  mw.WriteProblem(c, err)
		   return
	       }
	  books, err := ledgerBooks(c.Request.Context(), rt, user, c.GetHeader("Authorization"))
	       if err != nil {
	  	  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
	  	  return
	  }
	   c.JSON(200, gin.H{"user_id": user, "accounts": groupBooks(books), "host": "query-gateway.kucoin.net"})
   }

   func writeOverview(c *gin.Context, rt *app.Runtime, svc *Service) {
	       user, err := validate.UserID(callerid.Resolve(c, idOpt(rt)))
	       if err != nil {
	       	   mw.WriteProblem(c, err)
	   	  return
	      }
	      bals, err := svc.Unified(c.Request.Context(), user)
	     if err != nil {
		   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
	  	  return
	      }
	         books, err := ledgerBooks(c.Request.Context(), rt, user, c.GetHeader("Authorization"))
	      if err != nil {
	     	   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
	    	   return
	       }
	   quoteQ, hideQ := quoteAndHideQuery(c)
	       opts, err := resolveDisplayOpts(c.Request.Context(), rt, user, quoteQ, hideQ)
	     if err != nil {
		  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
	   	  return
	  }
	     valued := valuePortfolio(c.Request.Context(),
		   newFxBook(rt.KV, convertQuoteURL(rt)), bals, opts.Quote, opts.HideSmall, opts.Hide)
	          ordered, err := applyValuedOrder(rt, c.Request.Context(), user, valued.Items)
	       if err != nil {
	     	  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
	   	   return
	     }
	     valued.Items = ordered
	     c.JSON(200, gin.H{
	   	  "user_id": user, "balances": bals, "valued": valued.Items,
	    	  "portfolio_total": valued.Total, "valuation_incomplete": valued.Incomplete,
	  	  "accounts": books, "quote": opts.Quote, "hide_small": opts.Hide,
	   	   "preference": opts.Pref, "host": "query-gateway.kucoin.net",
	   })
  }

   func writeValuation(c *gin.Context, rt *app.Runtime, svc *Service) {
	         user, err := validate.UserID(callerid.Resolve(c, idOpt(rt)))
	      if err != nil {
	   	   mw.WriteProblem(c, err)
	     	  return
	         }
	      bals, err := svc.Unified(c.Request.Context(), user)
	      if err != nil {
	  	  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
	    	  return
	        }
	    quoteQ, hideQ := quoteAndHideQuery(c)
	    opts, err := resolveDisplayOpts(c.Request.Context(), rt, user, quoteQ, hideQ)
	     if err != nil {
	   	   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
	  	  return
	     }
	  valued := valuePortfolio(c.Request.Context(), newFxBook(rt.KV, convertQuoteURL(rt)), bals, opts.Quote, opts.HideSmall, opts.Hide)
	         c.JSON(200, gin.H{
		  "user_id": user,
	  	  "portfolio_total": valued.Total, "valuation_incomplete": valued.Incomplete,
	      	  "quote": valued.Quote, "valued": valued.Items,
		  "valuation": valued,
	       	  "host": "query-gateway.kucoin.net",
	     })
  }

   func writeLite(c *gin.Context, rt *app.Runtime) {
	     user, err := validate.UserID(callerid.Resolve(c, idOpt(rt)))
	  if err != nil {
	   	   mw.WriteProblem(c, err)
		   return
	      }
	      quoteQ, _ := quoteAndHideQuery(c)
	      opts, err := resolveDisplayOpts(c.Request.Context(), rt, user, quoteQ, "")
	  if err != nil {
	    	  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
		  return
	      }
	     books, errBooks := ledgerBooks(c.Request.Context(), rt, user, c.GetHeader("Authorization"))
	   if errBooks != nil {
	        	  c.JSON(apierr.HTTPStatus(errBooks), gin.H{"error": errBooks.Error()})
	      	  return
	   }
	       var led *ledger.Service
	      if rt != nil {
	   	   led = rt.Ledger
	      }
	    view, err := liteSummary(c.Request.Context(), led, user, opts.Quote, opts.HideSmall, books)
	      if err != nil {
	      	   c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
	    	   return
	      }
	  if err := applyLiteDefault(rt, c.Request.Context(), user, &view); err != nil {
		  c.JSON(apierr.HTTPStatus(err), gin.H{"error": err.Error()})
	   	   return
	       }
	   c.JSON(200, gin.H{"lite": view, "host": "query-gateway.kucoin.net"})
  }

   func groupBooks(books []ledger.BookView) map[string][]ledger.BookView {
	    out := map[string][]ledger.BookView{}
	        for _, b := range books {
		   key := strings.ToLower(strings.TrimSpace(b.Book))
	       	  if key == "" {
	    		   key = "other"
	   	  }
		  out[key] = append(out[key], b)
	      }
	      return out
   }
