// Package earnflow 理财申购/赎回完成后向 funding-bff 投递事件（未配置客户端时跳过）。
package earnflow

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

type EarnEvent struct {
	  EventID    string `json:"eventId"`
	   EventType  string `json:"eventType"`
	UID        string `json:"uid"`
	PositionID string `json:"positionId"`
	    ProductID  string `json:"productId,omitempty"`
	Asset      string `json:"asset,omitempty"`
	  Amount     string `json:"amount,omitempty"`
	     Penalty    string `json:"penalty,omitempty"`
	    Status     string `json:"status,omitempty"`
	    OccurredAt string `json:"occurredAt"`
}

type EarnEventData struct {
	   Accepted bool `json:"accepted"`
}

var ErrEarnEvent = errors.New("earn event rejected")

type Notifier struct {
	     bff *fundingbff.Client
	    log zerolog.Logger
}

 func NewNotifier(bff *fundingbff.Client, log zerolog.Logger) *Notifier {
	  return &Notifier{bff: bff, log: log}
}

  func (n *Notifier) Notify(ctx context.Context, ev EarnEvent) error {
	   if n == nil || n.bff == nil || !n.bff.Enabled() {
	  	return nil
	    }
	      ev = normalize(ev)
	        if ev.EventID == "" || ev.EventType == "" || ev.UID == "" || ev.PositionID == "" {
	   return fmt.Errorf("%w: missing required fields", ErrEarnEvent)
	   }
	        _, err := post(ctx, n.bff, ev)
	  if err != nil && n.log.GetLevel() != zerolog.Disabled {
	   	n.log.Warn().Err(err).Str("position", ev.PositionID).Str("type", ev.EventType).Msg("earn event")
	}
	return err
}

   func normalize(ev EarnEvent) EarnEvent {
	    ev.EventID = strings.TrimSpace(ev.EventID)
	      ev.EventType = strings.ToUpper(strings.TrimSpace(ev.EventType))
	    ev.UID = strings.TrimSpace(ev.UID)
	       ev.PositionID = strings.TrimSpace(ev.PositionID)
	  ev.ProductID = strings.TrimSpace(ev.ProductID)
	  ev.Asset = strings.ToUpper(strings.TrimSpace(ev.Asset))
	    ev.Status = strings.ToLower(strings.TrimSpace(ev.Status))
	      if ev.OccurredAt == "" {
	    	ev.OccurredAt = time.Now().UTC().Format(time.RFC3339)
	      }
	if ev.EventID == "" {
	   	ev.EventID = ev.PositionID + ":" + ev.EventType
	      }
	   return ev
}

func post(ctx context.Context, bff *fundingbff.Client, ev EarnEvent) (EarnEventData, error) {
	    var out struct {
	    	Code string        `json:"code"`
	    	Data EarnEventData `json:"data"`
	  	Msg  string        `json:"msg"`
	}
	     st, raw, err := bff.PostJSON(ctx, platformjava.PathEarnEvents, ev)
	  if err != nil {
	  	return EarnEventData{}, err
	    }
	    if st == 0 {
	     	return EarnEventData{}, nil
	  }
	   if st < 200 || st >= 300 {
	       	return EarnEventData{}, fmt.Errorf("%w: http %d %s", ErrEarnEvent, st, strings.TrimSpace(string(raw)))
	}
	  if len(raw) == 0 {
		return EarnEventData{Accepted: true}, nil
	}
	if json.Unmarshal(raw, &out) != nil {
		return EarnEventData{Accepted: true}, nil
	}
	if out.Code != "" && out.Code != "200000" {
	  	return EarnEventData{}, fmt.Errorf("%w: %s", ErrEarnEvent, strings.TrimSpace(out.Msg))
	       }
	if !out.Data.Accepted {
	  	out.Data.Accepted = true
	      }
	      return out.Data, nil
}
