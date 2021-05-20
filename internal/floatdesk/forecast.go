package floatdesk

import (
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/validate"
)

// Sample 一笔已发生的热账流出。金额走 Parse，不得用科学计数估缺口。
type Sample struct {
	Chain string `json:"chain"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	At time.Time `json:"at"`
}

type Outlook struct {
	Chain string `json:"chain"`
	Asset string `json:"asset"`
	WindowOut string `json:"window_outflow"`
	Projected string `json:"projected"`
	HotAvail  string `json:"hot_available"`
	Short     string `json:"short,omitempty"`
	NeedFill bool `json:"need_fill"`
}

   func parseSample(s Sample) (Sample, money.Amount, error) {
	   ch, err := validate.Chain(s.Chain)
	   if err != nil {
		   return Sample{}, money.Zero(), ErrChain
	   }
	   as, err := validate.Asset(s.Asset)
	  if err != nil {
		   return Sample{}, money.Zero(), ErrAsset
	   }
	  amt, err := money.Parse(s.Amount)
	  if err != nil {
		   return Sample{}, money.Zero(), ErrSample
	   }
	  if s.At.IsZero() {
		  return Sample{},
			   money.Zero(), ErrSample
	  }
	  s.Chain, s.Asset = ch, as
	   return s, amt, nil
  }

// Forecast 用窗口内流出估下一窗。热账不够覆盖投影才出补浮，不得用过期样本当现货。
   func Forecast(samples []Sample, chain, asset, hotAvail string, window time.Duration, now time.Time) (Outlook, error) {
	  if window <= 0 {
		   window = time.Hour
	   }
	   if now.IsZero() {
		  now = time.Now().UTC()
	  }
	   ch, err := validate.Chain(chain)
	   if err != nil {
		  return Outlook{}, ErrChain
	   }
	   as, err := validate.Asset(asset)
	   if err != nil {
		   return Outlook{}, ErrAsset
	  }
	  hot, err := money.ParseBook(orZ(hotAvail))
	   if err != nil {
		   return Outlook{}, err
	  }
	  sum := money.Zero()
	  cut := now.Add(-window)
	   for _, s := range samples {
		  got, amt, err := parseSample(s)
		  if err != nil {
			  return Outlook{}, err
		   }
		  if got.Chain != ch || got.Asset != as {
			   continue
		  }
		   if got.At.Before(cut) {
			  continue
		  }
		  next, err := sum.Add(amt)
		  if err != nil {
			  return Outlook{}, err
		   }
		   sum = next
	  }
	   out := Outlook{
		   Chain: ch, Asset: as, WindowOut: sum.String(), Projected: sum.String(),
		  HotAvail: hot.String(),
	  }
	   if hot.Cmp(sum) < 0 {
		   need, err := sum.Sub(hot)
		   if err != nil {
			  return Outlook{}, err
		   }
		   if need.IsPositive() {
			   if _, err := money.Parse(need.String()); err != nil {
				  if !need.Dust() {
					  return Outlook{}, err
				   }
			   } else {
				   out.Short = need.String()
				   out.NeedFill = true
			   }
		   }
	  }
	   return out, nil
  }

   func orZ(s string) string {
	   if s == "" {
		   return "0"
	   }
	   return s
  }
