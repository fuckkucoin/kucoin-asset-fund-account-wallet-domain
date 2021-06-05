// Package fundflow 流水导出任务终态通知 funding-bff（未配置客户端时跳过）。
package fundflow

import (
	  "context"
	  "encoding/json"
	    "errors"
	"fmt"
	   "strings"
	  "time"

	"account-assets.kucoin.net/funding/internal/fundingbff"
	"account-assets.kucoin.net/funding/internal/platformjava"
	   "github.com/rs/zerolog"
)

type ExportEvent struct {
	  EventID    string `json:"eventId"`
	  ExportID   string `json:"exportId"`
	   UID        string `json:"uid"`
	   Status     string `json:"status"`
	   Kind       string `json:"kind"`
	OccurredAt string `json:"occurredAt"`
}

type ExportEventData struct {
	  Accepted bool `json:"accepted"`
}

var ErrExportEvent = errors.New("fund-flow export-event rejected")

type Notifier struct {
	    bff *fundingbff.Client
	    log zerolog.Logger
}

func NewNotifier(bff *fundingbff.Client, log zerolog.Logger) *Notifier {
	    return &Notifier{bff: bff, log: log}
}

   func (n *Notifier) NotifyExport(ctx context.Context, ev ExportEvent) error {
	   if n == nil || n.bff == nil || !n.bff.Enabled() {
	  	return nil
	}
	   ev = normalizeExport(ev)
	    if ev.EventID == "" || ev.ExportID == "" || ev.UID == "" || ev.Status == "" {
	  	return fmt.Errorf("%w: missing required fields", ErrExportEvent)
	   }
	  _, err := postExport(ctx, n.bff, ev)
	  if err != nil && n.log.GetLevel() != zerolog.Disabled {
		n.log.Warn().Err(err).Str("export", ev.ExportID).Str("status", ev.Status).Msg("fund-flow export-event")
	   }
	      return err
}

func normalizeExport(ev ExportEvent) ExportEvent {
	    ev.EventID = strings.TrimSpace(ev.EventID)
	    ev.ExportID = strings.TrimSpace(ev.ExportID)
	ev.UID = strings.TrimSpace(ev.UID)
	  ev.Status = strings.ToLower(strings.TrimSpace(ev.Status))
	   ev.Kind = strings.ToLower(strings.TrimSpace(ev.Kind))
	  if ev.OccurredAt == "" {
	       	ev.OccurredAt = time.Now().UTC().Format(time.RFC3339)
	}
	if ev.EventID == "" {
		ev.EventID = ev.ExportID + ":" + ev.Status
	}
	    return ev
}

func postExport(ctx context.Context, bff *fundingbff.Client, ev ExportEvent) (ExportEventData, error) {
	   var out struct {
	   	Code string          `json:"code"`
	  	Data ExportEventData `json:"data"`
	    	Msg  string          `json:"msg"`
	    }
	  st, raw, err := bff.PostJSON(ctx, platformjava.PathFundFlowExportEvents, ev)
	if err != nil {
		return ExportEventData{}, err
	   }
	      if st == 0 {
	     	return ExportEventData{}, nil
	  }
	   if st < 200 || st >= 300 {
	      	return ExportEventData{}, fmt.Errorf("%w: http %d %s", ErrExportEvent, st, strings.TrimSpace(string(raw)))
	    }
	      if len(raw) == 0 {
	   	return ExportEventData{Accepted: true}, nil
	       }
	      if json.Unmarshal(raw, &out) != nil {
	     	return ExportEventData{Accepted: true}, nil
	   }
	    if out.Code != "" && out.Code != "200000" {
	     	return ExportEventData{}, fmt.Errorf("%w: %s", ErrExportEvent, strings.TrimSpace(out.Msg))
	}
	if !out.Data.Accepted {
	      	out.Data.Accepted = true
	    }
	       return out.Data, nil
}
