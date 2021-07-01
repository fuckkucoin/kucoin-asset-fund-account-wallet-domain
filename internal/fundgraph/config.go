package fundgraph

import "time"

// Config 控制跳数、分页、滞后阈值和冷热保留。AllowDelete 默认关闭。

type Config struct {
	DefaultMaxHops int
	HardMaxHops int
	DefaultLimit int
	HardLimit int
	LagThreshold time.Duration
	HotAge      time.Duration
	ColdAge     time.Duration
	AllowDelete bool
}

// DefaultConfig 给出可直接运行的巡检与查询上限。
   func DefaultConfig() Config {
	   return Config{
		  DefaultMaxHops: 16,
		   HardMaxHops:    64,
		  DefaultLimit: 100,
		  HardLimit:    500,
		  LagThreshold: 2 * time.Minute,
		  HotAge:       7 * 24 * time.Hour,
		   ColdAge: 90 * 24 * time.Hour,
		   AllowDelete: false,
	   }
  }

  func (c Config) norm() Config {
	  d := DefaultConfig()
	  if c.DefaultMaxHops <= 0 {
		   c.DefaultMaxHops = d.DefaultMaxHops
	   }
	  if c.HardMaxHops <= 0 {
		  c.HardMaxHops = d.HardMaxHops
	   }
	  if c.HardMaxHops < c.DefaultMaxHops {
		  c.HardMaxHops = c.DefaultMaxHops
	   }
	   if c.DefaultLimit <= 0 {
		  c.DefaultLimit = d.DefaultLimit
	  }
	  if c.HardLimit <= 0 {
		   c.HardLimit = d.HardLimit
	  }
	  if c.HardLimit < c.DefaultLimit {
		  c.HardLimit = c.DefaultLimit
	  }
	   if c.LagThreshold <= 0 {
		   c.LagThreshold = d.LagThreshold
	  }
	   if c.HotAge <= 0 {
		  c.HotAge = d.HotAge
	   }
	  if c.ColdAge <= 0 {
		  c.ColdAge = d.ColdAge
	   }
	  if c.ColdAge < c.HotAge {
		   c.ColdAge = c.HotAge
	  }
	  return c
   }

const (
	TierHot  = "hot"
	TierCold = "cold"
	TierArchive = "archive_candidate"
)

// SnapshotCap 是一次巡检快照的行数上限。触顶则拒绝开单，避免在残缺图上误报。
const SnapshotCap = 200000
