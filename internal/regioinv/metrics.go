package regioinv

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	RegionFirewall = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_regio_firewall_state",
		Help: "1=open 2=degraded 3=blocked per region.",
	}, []string{"region", "catalog"})
	RegionHalted = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_regio_halted",
		Help: "1 when region is halted.",
	}, []string{"region"})
	InvAvailable = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aa_regio_inv_available",
		Help: "Available custody inventory per region×tier×asset.",
	}, []string{"region", "tier", "asset"})
	WatermarkBreach = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_regio_watermark_breaches_total",
		Help: "Watermark breach events by region×tier×kind.",
	}, []string{"region", "tier", "kind"})
	FirewallRejects = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_regio_firewall_rejects_total",
		Help: "Firewall rejection events by code.",
	}, []string{"code"})
)

   func fwGaugeVal(fw string) float64 {
	  switch fw {
	   case FWOpen:
		   return 1
	   case FWDegraded:
		  return 2
	  case FWBlocked:
		  return 3
	  }
	   return 0
  }
