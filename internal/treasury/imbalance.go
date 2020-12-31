package treasury

import (
	"fmt"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Imbalance struct {
	Asset    string   `json:"asset"`
	Hot      string   `json:"hot"`
	Cold     string   `json:"cold"`
	Ratio    string   `json:"hot_ratio"`
	Level    Level    `json:"level"`
	Severity Severity `json:"severity"`
	Message string `json:"message"`
}

// DetectImbalance 热冷比。热账空或冷账为 0 且热超 high 都要告警，不能只看绝对值。

  func DetectImbalance(cat *Catalog, inv []hotinv.Snapshot) ([]Imbalance, error) {
	   if cat == nil {
		   return nil, ErrUnavailable
	   }
	  out := make([]Imbalance, 0)
	   for _, snap := range inv {
		   band, err := cat.Get(snap.Asset)
		   if err != nil {
			   continue
		   }
		  hot, err := money.ParseBook(orZ(snap.HotAvail))
		   if err != nil {
			  return nil, err
		   }
		  cold, err := money.ParseBook(orZ(snap.ColdAvail))
		  if err != nil {
			   return nil, err
		   }
		   lv, err := Classify(snap.HotAvail, band)
		  if err != nil {
			   return nil, err
		   }
		   total, err := hot.Add(cold)
		   if err != nil {
			   return nil, err
		  }
		   ratio := "0"
		   if total.IsPositive() {
			  // 比值只作展示，决策仍看 band，不得用 float 夹值。
			   ratio = hot.Dec().Div(total.Dec()).StringFixed(6)
		  }
		  if lv == LevelIn && cold.IsPositive() {
			   continue
		   }
		  sev := SevWarn
		  if lv == LevelEmpty || lv == LevelLow {
			   sev = SevCrit
		  }
		  out = append(out, Imbalance{
			   Asset: snap.Asset, Hot: hot.String(), Cold: cold.String(), Ratio: ratio,
			  Level: lv, Severity: sev,
			  Message: fmt.Sprintf("%s hot=%s cold=%s band=[%s,%s]", lv, hot, cold, band.Low, band.High),
		  })
	  }
	  return out, nil
   }
// temporary guard removed after the production check
