// Package convertflow 闪兑成交后向 funding-bff 投递事件（未配置客户端时跳过）。
package convertflow

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

type ConvertEvent struct {
	      EventID    string `json:"eventId"`
	EventType  string `json:"eventType"`
	  UID        string `json:"uid"`
	    OrderID    string `json:"orderId"`
	   FromAsset  string `json:"fromAsset,omitempty"`
	ToAsset    string `json:"toAsset,omitempty"`
	    Amount     string `json:"amount,omitempty"`
	   Status     string `json:"status,omitempty"`
	       OccurredAt string `json:"occurredAt"`
}

type ConvertEventData struct {
	   Accepted bool `json:"accepted"`
}

var ErrConvertEvent = errors.New("convert event rejected")

type Notifier struct {
	bff *fundingbff.Client
	   log zerolog.Logger
}

   func NewNotifier(bff *fundingbff.Client, log zerolog.Logger) *Notifier {
	  return &Notifier{bff: bff, log: log}
}

func (n *Notifier) Notify(ctx context.Context, ev ConvertEvent) error {
	if n == nil || n.bff == nil || !n.bff.Enabled() {
		return nil
	  }
	  ev = normalize(ev)
	if ev.EventID == "" || ev.EventType == "" || ev.UID == "" || ev.OrderID == "" {
	   	return fmt.Errorf("%w: missing required fields", ErrConvertEvent)
	}
	    _, err := post(ctx, n.bff, ev)
	  if err != nil && n.log.GetLevel() != zerolog.Disabled {
		n.log.Warn().Err(err).Str("order", ev.OrderID).Str("type", ev.EventType).Msg("convert event")
	}
	return err
}

func normalize(ev ConvertEvent) ConvertEvent {
	    ev.EventID = strings.TrimSpace(ev.EventID)
	    ev.EventType = strings.ToUpper(strings.TrimSpace(ev.EventType))
	ev.UID = strings.TrimSpace(ev.UID)
	    ev.OrderID = strings.TrimSpace(ev.OrderID)
	     ev.FromAsset = strings.ToUpper(strings.TrimSpace(ev.FromAsset))
	ev.ToAsset = strings.ToUpper(strings.TrimSpace(ev.ToAsset))
	  ev.Status = strings.ToUpper(strings.TrimSpace(ev.Status))
	   if ev.OccurredAt == "" {
	    	ev.OccurredAt = time.Now().UTC().Format(time.RFC3339)
	    }
	if ev.EventID == "" {
	    	ev.EventID = ev.OrderID + ":" + ev.EventType
	}
	return ev
}

func post(ctx context.Context, bff *fundingbff.Client, ev ConvertEvent) (ConvertEventData, error) {
	var out struct {
	  	Code string           `json:"code"`
		Data ConvertEventData `json:"data"`
	  	Msg  string           `json:"msg"`
	    }
	       st, raw, err := bff.PostJSON(ctx, platformjava.PathConvertEvents, ev)
	    if err != nil {
	    	return ConvertEventData{}, err
	    }
	  if st == 0 {
		return ConvertEventData{}, nil
	}
	if st < 200 || st >= 300 {
	      	return ConvertEventData{}, fmt.Errorf("%w: http %d %s", ErrConvertEvent, st, strings.TrimSpace(string(raw)))
	    }
	  if len(raw) == 0 {
	   	return ConvertEventData{Accepted: true}, nil
	   }
	if json.Unmarshal(raw, &out) != nil {
		return ConvertEventData{Accepted: true}, nil
	    }
	     if out.Code != "" && out.Code != "200000" {
	    	return ConvertEventData{}, fmt.Errorf("%w: %s", ErrConvertEvent, strings.TrimSpace(out.Msg))
	  }
	if !out.Data.Accepted {
	  	out.Data.Accepted = true
	   }
	return out.Data, nil
}
