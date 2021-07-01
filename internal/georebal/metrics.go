package georebal

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	PlansTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_georebal_plans_total",
		Help: "Geo-rebalance plans by final state.",
	}, []string{"state"})
	LegsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_georebal_legs_total",
		Help: "Geo-rebalance legs by final state.",
	}, []string{"state"})
	FirewallBlocks = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_georebal_firewall_blocks_total",
		Help: "Firewall rejections by code.",
	}, []string{"code"})
)
