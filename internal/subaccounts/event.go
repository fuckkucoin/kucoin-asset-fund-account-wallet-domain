// Package subaccounts 子账户生命周期变更通知 funding-bff（未配置客户端时跳过）。
package subaccounts

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

// LifecycleEvent POST /internal/v1/sub-accounts/events。
type LifecycleEvent struct {
	  EventID    string         `json:"eventId"`
	      EventType  string         `json:"eventType"`
	   MasterUID  string         `json:"masterUid"`
	SubUserID  string         `json:"subUserId"`
	SubName    string         `json:"subName,omitempty"`
	Payload    map[string]any `json:"payload,omitempty"`
	OccurredAt string         `json:"occurredAt"`
}

type LifecycleData struct {
	    Accepted bool `json:"accepted"`
}

var ErrLifecycleEvent = errors.New("sub-accounts lifecycle event rejected")

type Notifier struct {
	   bff *fundingbff.Client
	   log zerolog.Logger
}

func NewNotifier(bff *fundingbff.Client, log zerolog.Logger) *Notifier {
	return &Notifier{bff: bff, log: log}
}

func (n *Notifier) Notify(ctx context.Context, ev LifecycleEvent) error {
	       if n == nil || n.bff == nil || !n.bff.Enabled() {
	  	return nil
	      }
	ev = NormalizeLifecycle(ev)
	if ev.EventID == "" || ev.EventType == "" || ev.MasterUID == "" || ev.SubUserID == "" {
	  	return fmt.Errorf("%w: missing required fields", ErrLifecycleEvent)
	}
	  _, err := post(ctx, n.bff, ev)
	      if err != nil && n.log.GetLevel() != zerolog.Disabled {
	   	n.log.Warn().Err(err).Str("sub", ev.SubUserID).Str("type", ev.EventType).Msg("sub-accounts lifecycle")
	       }
	       return err
}

func NormalizeLifecycle(ev LifecycleEvent) LifecycleEvent {
	    ev.EventID = strings.TrimSpace(ev.EventID)
	  ev.EventType = strings.ToUpper(strings.TrimSpace(ev.EventType))
	    ev.MasterUID = strings.TrimSpace(ev.MasterUID)
	    ev.SubUserID = strings.TrimSpace(ev.SubUserID)
	  ev.SubName = strings.TrimSpace(ev.SubName)
	   if ev.OccurredAt == "" {
		ev.OccurredAt = time.Now().UTC().Format(time.RFC3339)
	    }
	  if ev.EventID == "" {
		ev.EventID = ev.SubUserID + ":" + ev.EventType
	   }
	return ev
}

  func post(ctx context.Context, bff *fundingbff.Client, ev LifecycleEvent) (LifecycleData, error) {
	  var out struct {
	   	Code string        `json:"code"`
	    	Data LifecycleData `json:"data"`
	      	Msg  string        `json:"msg"`
	}
	  st, raw, err := bff.PostJSON(ctx, platformjava.PathSubAccountEvents, ev)
	if err != nil {
	   	return LifecycleData{}, err
	}
	    if st == 0 {
	     	return LifecycleData{}, nil
	}
	    if st < 200 || st >= 300 {
	   	return LifecycleData{}, fmt.Errorf("%w: http %d %s", ErrLifecycleEvent, st, strings.TrimSpace(string(raw)))
	   }
	if len(raw) == 0 {
	  	return LifecycleData{Accepted: true}, nil
	    }
	    if json.Unmarshal(raw, &out) != nil {
		return LifecycleData{Accepted: true}, nil
	      }
	  if out.Code != "" && out.Code != "200000" {
		return LifecycleData{}, fmt.Errorf("%w: %s", ErrLifecycleEvent, strings.TrimSpace(out.Msg))
	  }
	if !out.Data.Accepted {
	  	out.Data.Accepted = true
	        }
	return out.Data, nil
}
