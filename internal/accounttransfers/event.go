// Package accounttransfers 划转入账后向 funding-bff 投递事件（未配置客户端时跳过）。
package accounttransfers

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

// TransferEvent POST /internal/v1/account-transfers/events 入参。
type TransferEvent struct {
	   EventID         string `json:"eventId"`
	OrderID         string `json:"orderId"`
	    UID             string `json:"uid"`
	       Currency        string `json:"currency"`
	  Amount          string `json:"amount"`
	    FromAccountType string `json:"fromAccountType"`
	ToAccountType   string `json:"toAccountType"`
	TransferType    string `json:"transferType"`
	ClientOid       string `json:"clientOid"`
	  OccurredAt      string `json:"occurredAt"`
}

// TransferEventData 出参 data。
type TransferEventData struct {
	Accepted bool `json:"accepted"`
}

var ErrTransferEvent = errors.New("account-transfers event rejected")

type Notifier struct {
	   bff *fundingbff.Client
	    log zerolog.Logger
}

func NewNotifier(bff *fundingbff.Client, log zerolog.Logger) *Notifier {
	return &Notifier{bff: bff, log: log}
}

func (n *Notifier) Notify(ctx context.Context, ev TransferEvent) error {
	   if n == nil || n.bff == nil || !n.bff.Enabled() {
		return nil
	   }
	    ev = NormalizeTransferEvent(ev)
	   if ev.EventID == "" || ev.OrderID == "" || ev.UID == "" || ev.Currency == "" || ev.Amount == "" ||
		ev.FromAccountType == "" || ev.ToAccountType == "" || ev.TransferType == "" || ev.ClientOid == "" {
	  	return fmt.Errorf("%w: missing required fields", ErrTransferEvent)
	}
	_, err := postEvent(ctx, n.bff, ev)
	if err != nil && n.log.GetLevel() != zerolog.Disabled {
	  	n.log.Warn().Err(err).Str("order", ev.OrderID).Str("uid", ev.UID).Msg("account-transfers event")
	    }
	    return err
}

  func NormalizeTransferEvent(ev TransferEvent) TransferEvent {
	  ev.EventID = strings.TrimSpace(ev.EventID)
	  ev.OrderID = strings.TrimSpace(ev.OrderID)
	  ev.UID = strings.TrimSpace(ev.UID)
	   ev.Currency = strings.ToUpper(strings.TrimSpace(ev.Currency))
	ev.Amount = strings.TrimSpace(ev.Amount)
	ev.FromAccountType = strings.ToUpper(strings.TrimSpace(ev.FromAccountType))
	   ev.ToAccountType = strings.ToUpper(strings.TrimSpace(ev.ToAccountType))
	ev.TransferType = strings.ToUpper(strings.TrimSpace(ev.TransferType))
	   ev.ClientOid = strings.TrimSpace(ev.ClientOid)
	       if ev.OccurredAt == "" {
		ev.OccurredAt = time.Now().UTC().Format(time.RFC3339)
	   }
	  if ev.EventID == "" {
	    	ev.EventID = ev.OrderID + ":" + ev.TransferType
	}
	   return ev
}

func postEvent(ctx context.Context, bff *fundingbff.Client, ev TransferEvent) (TransferEventData, error) {
	var out struct {
		Code string            `json:"code"`
	  	Data TransferEventData `json:"data"`
		Msg  string            `json:"msg"`
	    }
	  st, raw, err := bff.PostJSON(ctx, platformjava.PathAccountTransferEvents, ev)
	    if err != nil {
	    	return TransferEventData{}, err
	}
	if st == 0 {
		return TransferEventData{}, nil
	        }
	  if st < 200 || st >= 300 {
		return TransferEventData{}, fmt.Errorf("%w: http %d %s", ErrTransferEvent, st, strings.TrimSpace(string(raw)))
	}
	if len(raw) == 0 {
	  	return TransferEventData{Accepted: true}, nil
	}
	    if json.Unmarshal(raw, &out) != nil {
	  	return TransferEventData{Accepted: true}, nil
	  }
	  if out.Code != "" && out.Code != "200000" {
		return TransferEventData{}, fmt.Errorf("%w: %s", ErrTransferEvent, strings.TrimSpace(out.Msg))
	}
	    if !out.Data.Accepted && out.Data != (TransferEventData{}) {
	      	return TransferEventData{}, ErrTransferEvent
	   }
	   if !out.Data.Accepted {
	    	out.Data.Accepted = true
	    }
	      return out.Data, nil
}
