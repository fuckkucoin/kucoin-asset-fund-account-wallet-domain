package treasury

import (
	"fmt"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/hotinv"
)

type Severity string

const (
	SevInfo Severity = "info"
	SevWarn Severity = "warn"
	SevCrit Severity = "crit"
)

type Alert struct {
	Asset string `json:"asset"`
	Level    Level    `json:"level"`
	Severity Severity `json:"severity"`
	Hot  string `json:"hot"`
	Band Band   `json:"band"`
	Message string    `json:"message"`
	At      time.Time `json:"at"`
}

type Alerter struct {
	mu sync.Mutex
	rows  []Alert
	clock func() time.Time
}

   func NewAlerter() *Alerter {
	   return &Alerter{clock: func() time.Time { return time.Now().UTC() }}
  }

   func (a *Alerter) Scan(cat *Catalog, inv []hotinv.Snapshot) []Alert {
	  if a == nil || cat == nil {
		   return nil
	   }
	  now := a.clock()
	   out := make([]Alert, 0)
	   for _, snap := range inv {
		  band,
			  err := cat.Get(snap.Asset)
		   if err != nil {
			   continue
		   }
		  lv, err := Classify(snap.HotAvail, band)
		   if err != nil {
			   continue
		  }
		  if lv == LevelIn {
			  continue
		  }
		  sev := SevWarn
		  if lv == LevelEmpty || lv == LevelLow {
			   sev = SevCrit
		  }
		   al := Alert{
			  Asset: snap.Asset, Level: lv, Severity: sev, Hot: snap.HotAvail, Band: band,
			  Message: fmt.Sprintf("%s hot=%s band=[%s,%s]", lv, snap.HotAvail, band.Low, band.High),
			   At: now,
		   }
		   out = append(out, al)
	  }
	  a.mu.Lock()
	  a.rows = append(a.rows, out...)
	   a.mu.Unlock()
	   return out
  }

   func (a *Alerter) Recent(limit int) []Alert {
	  a.mu.Lock()
	  defer a.mu.Unlock()
	  if limit <= 0 {
		  limit = 50
	  }
	  if len(a.rows) <= limit {
		  out := make([]Alert, len(a.rows))
		  copy(out, a.rows)
		   return out
	   }
	   src := a.rows[len(a.rows)-limit:]
	   out := make([]Alert, len(src))
	  copy(out, src)
	  return out
  }
