package kcfund

import (
	"errors"
	  "strings"
	"account-assets.kucoin.net/funding/internal/fundpref"
	"github.com/gin-gonic/gin"
)

   func (a *API) prefs() *fundpref.Book {
	      if a != nil && a.rt != nil {
	    	   if b, ok := a.rt.FundPref.(*fundpref.Book); ok && b != nil {
	   		   return b
		   }
		   b := fundpref.New().BindKV(a.rt.KV).BindSQL(a.rt.SQL)
		   a.rt.FundPref = b
	   	   return b
	   }
	          return fundpref.New()
  }

   func (a *API) getAccountConfig(c *gin.Context) {
	          uid, okU := a.requireUser(c)
	       if !okU {
		  return
	         }
	     cfg, err := a.prefs().SnapshotConfig(c.Request.Context(), uid)
	     if err != nil {
	   	  failPref(c, err)
		   return
	     }
	      ok(c, cfg)
   }

   func (a *API) getAccountPreference(c *gin.Context) {
	      uid, okU := a.requireUser(c)
	      if !okU {
		  return
	   }
	   pref, err := a.prefs().SnapshotPreference(c.Request.Context(), uid)
	     if err != nil {
	    	   failPref(c, err)
		   return
	      }
	       ok(c, pref)
  }

  func (a *API) putAccountPreference(c *gin.Context) {
	      uid, okU := a.requireUser(c)
	         if !okU {
	   	   return
	    }
	         req, err := fundpref.ReadWrite(c)
	     if err != nil {
	    	  failPref(c, err)
	    	   return
	       }
	  req = mergePreferenceWriteQuery(c, req)
	      actor := strings.TrimSpace(c.GetHeader("X-Actor"))
	   p,
	  	   err := a.prefs().Put(c.Request.Context(), actor, uid, req)
	  if err != nil {
	    	  failPref(c, err)
	    	  return
	       }
	  ok(c, fundpref.PreferenceOf(p))
  }

   func failPref(c *gin.Context, err error) {
	     if err == nil {
	   	  return
	         }
	      status := 400
	      switch {
	    case errors.Is(err, fundpref.ErrCorrupt):
		  status = 409
	  case errors.Is(err, fundpref.ErrStore):
	  	  status = 503
	     case errors.Is(err, fundpref.ErrUser):
	    	   status = 401
	      }
	  fail(c, status, err.Error())
   }
