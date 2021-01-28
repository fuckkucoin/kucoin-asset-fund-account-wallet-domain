package kcfund

import (
	    "encoding/json"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/exjob"
	"account-assets.kucoin.net/funding/internal/fundsurf"
	"github.com/gin-gonic/gin"
)

  func (a *API) registerLedgerExport(g *gin.RouterGroup) {
	        g.POST("/v1/accounts/ledgers/export", a.createLedgerExport)
	     g.GET("/v1/accounts/ledgers/export/:exportId", a.getLedgerExport)
	     g.GET("/v1/accounts/ledgers/export/:exportId/download", a.downloadLedgerExport)
  }

  func (a *API) createLedgerExport(c *gin.Context) {
	      body, err := bindLedgerExportBody(c)
	  if err != nil {
	      	  fail(c, 400, err.Error())
	   	  return
	      }
	      uid, okU := a.requireUser(c)
	        if !okU {
		   return
	   }
	   kind := strings.ToLower(strings.TrimSpace(body.Kind))
	     if kind == "" {
	    	  kind = "all"
	     }
	         format := strings.ToLower(strings.TrimSpace(body.Format))
	       if format == "" {
	    	   format = "csv"
	      }
	   start, end := parseExportRange(body.StartAt, body.EndAt)
	  if err := fundsurf.CheckExportWindow(start, end); err != nil {
		  fail(c, 400, err.Error())
	       	  return
	     }
	        payload := map[string]string{
	  	  "user_id": uid, "kind": kind, "format": format,
		  "start_at": start.UTC().Format(time.RFC3339),
	    	  "end_at": end.UTC().Format(time.RFC3339),
	  }
	         raw, _ := json.Marshal(payload)
	       if code, resp, ok := a.tryMeshStatementExport(c, raw); ok {
		   if code != 0 {
	   		   c.Data(code, "application/json", resp)
	   	   }
	  	  return
	        }
	       book := a.exportBook()
	       if book == nil {
	    	  fail(c, 503, "export unavailable")
	    	  return
	           }
	  job, err := book.EnqueueWindow(uid, kind, format, start, end)
	     if err != nil {
	  	   fail(c, 400, err.Error())
	  	   return
	     }
	      a.emitExportEvent(c.Request.Context(), job)
	   ok(c, exportJobJSON(job))
   }

   func (a *API) getLedgerExport(c *gin.Context) {
	   uid, okU := a.requireUser(c)
	    if !okU {
	  	   return
	   }
	      id := strings.TrimSpace(c.Param("exportId"))
	     book := a.exportBook()
	  if book == nil {
	       	  fail(c, 503, "export unavailable")
	   	  return
	  }
	          job, err := book.GetFor(uid, id)
	      if err != nil {
	   	   if err == exjob.ErrGone || err == exjob.ErrOwner {
			  fail(c, 404, err.Error())
	  		  return
	    	  }
	  	  fail(c, 400, err.Error())
		   return
	    }
	  ok(c, exportJobJSON(job))
  }

   func (a *API) downloadLedgerExport(c *gin.Context) {
	  uid, okU := a.requireUser(c)
	      if !okU {
	   	  return
	  }
	         id := strings.TrimSpace(c.Param("exportId"))
	  book := a.exportBook()
	       if book == nil {
	    	  fail(c, 503, "export unavailable")
	    	  return
	  }
	  job, err := book.GetFor(uid, id)
	     if err != nil {
	       	  if err == exjob.ErrGone || err == exjob.ErrOwner {
			  fail(c, 404, err.Error())
			  return
	        	  }
		  fail(c, 400, err.Error())
	  	  return
	  }
	  body, err := book.GetBody(id, uid)
	  if err != nil {
		  fail(c, 400, err.Error())
	  	  return
	  }
	     ct := "text/csv; charset=utf-8"
	  if strings.EqualFold(job.Format, "json") {
		  ct = "application/json; charset=utf-8"
	     }
	     c.Header("Content-Disposition", "attachment; filename=\"fund-export-"+id+"."+job.Format+"\"")
	  c.Data(200, ct, []byte(body))
  }

   func (a *API) exportBook() *exjob.Book {
	     if a == nil || a.rt == nil || a.rt.Exports == nil {
	   	   return nil
	  }
	     b := a.rt.Exports
	     if a.rt.KV != nil {
		  b = b.BindKV(a.rt.KV)
	       }
	     if a.rt.Ledger != nil {
	    	  b = b.BindLedger(a.rt.Ledger)
	      }
	     if a.rt.SQL != nil {
	   	  b = b.BindSQL(a.rt.SQL)
	      }
	  return b
   }

 func parseExportRange(startRaw, endRaw string) (time.Time, time.Time) {
	  end := time.Now().UTC()
	   start := end.Add(-100 * 24 * time.Hour)
	      if t := parseUnixOrRFC(endRaw); !t.IsZero() {
	   	  end = t.UTC()
	      }
	  if t := parseUnixOrRFC(startRaw); !t.IsZero() {
	   	   start = t.UTC()
	      }
	      if start.After(end) {
	  	  start, end = end, start
	      }
	      return start, end
   }
