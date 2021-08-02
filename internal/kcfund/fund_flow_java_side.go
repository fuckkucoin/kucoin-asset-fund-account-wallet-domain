package kcfund

import (
	    "context"

	    "account-assets.kucoin.net/funding/internal/exjob"
	    "account-assets.kucoin.net/funding/internal/fundflow"
)

func (a *API) emitExportEvent(ctx context.Context, job exjob.Job) {
	  if a == nil || a.ffNotify == nil {
	    	return
	   }
	    st := job.Status
	    if st != exjob.StatusDone && st != exjob.StatusFailed {
	  	return
	  }
	    _ = a.ffNotify.NotifyExport(ctx, fundflow.ExportEvent{
	        	ExportID: job.ID,
		UID:      job.UserID,
		Status:   job.Status,
		Kind:     job.Kind,
	      })
}
