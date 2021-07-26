package hotreserve

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/shopspring/decimal"
)

var (
	forecastBias = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_hot_forecast_bias",
		Help: "Point estimate minus realized hot remainder; positive means the point was high",
	}, []string{"asset"})
	reserveCoverage = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_hot_reserve_coverage",
		Help: "Open and earmarked reservation divided by hot available",
	}, []string{"asset"})
	alignDrift = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_hot_align_drift",
		Help: "1 when reservation, freeze track and broadcast track are not aligned",
	}, []string{"asset"})
	signBlocked = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_hot_sign_block_total",
		Help: "Reserve create, sign or submit rejected by the hot reserve gate",
	}, []string{"phase", "reason"})
)

  func noteBlock(phase, reason string) {
	  if phase == "" {
		  phase = "unknown"
	   }
	   if reason == "" {
		  reason = "unknown"
	   }
	   signBlocked.WithLabelValues(phase, reason).Inc()
  }

  func noteDrift(asset string, drift bool) {
	   v := 0.0
	   if drift {
		   v = 1
	  }
	   alignDrift.WithLabelValues(asset).Set(v)
   }

  func noteBias(asset, bias string) {
	   d, err := decimal.NewFromString(bias)
	   if err != nil {
		   return
	  }
	  f, _ := d.Float64()
	   forecastBias.WithLabelValues(asset).Set(f)
  }

  func (s *Service) paintForecast(row *HotForecast) {
	  if row == nil {
		   return
	  }
	  hot,
		   err := parseNonNeg(row.HotAvail)
	   if err != nil {
		  return
	   }
	  reserved := s.sumState(row.Asset, StateOpen).Add(s.sumState(row.Asset, StateEarmarked))
	   cov := 0.0
	  if hot.IsPositive() {
		   f, _ := reserved.Div(hot).Float64()
		   cov = f
	  } else if reserved.IsPositive() {
		  cov = 1
	  }
	  reserveCoverage.WithLabelValues(row.Asset).Set(cov)
	   if row.Bias != "" {
		  noteBias(row.Asset, row.Bias)
	   }
   }
