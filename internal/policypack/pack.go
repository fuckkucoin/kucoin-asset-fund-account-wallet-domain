package policypack

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/policy"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Pack struct {
	Name string `json:"name"`
	KYC          string     `json:"kyc"`
	Doc          policy.Doc `json:"doc"`
	DestReuseMax int        `json:"dest_reuse_max"`
	DestReuseWin   time.Duration `json:"dest_reuse_window"`
	HourlyVelocity int           `json:"hourly_velocity"`
	DailyVelocity  int           `json:"daily_velocity"`
	NightHold bool `json:"night_hold"`
	NightStartHour  int  `json:"night_start_hour"`
	NightEndHour    int  `json:"night_end_hour"`
	FirstDestHold   bool `json:"first_dest_hold"`
	NewDestDailyMax int  `json:"new_dest_daily_max"`
	AllowlistRequired bool `json:"allowlist_required"`
	WeekendHold bool              `json:"weekend_hold"`
	AssetMax    map[string]string `json:"asset_max,omitempty"`
	ClusterMax  int               `json:"cluster_max"`
	CorrelationMax int `json:"correlation_max"`
	DestReuse24h   int `json:"dest_reuse_24h"`
}

type Input struct {
	Kind policy.Kind
	Amount     money.Amount
	TodaySum   money.Amount
	Velocity1m int
	Velocity1h int
	Velocity24h int
	DestCount   int
	DestCount24h int
	FirstDest    bool
	HourUTC      int
	Allowlisted   bool
	NewDestsToday int
	Asset         string
	Weekday       time.Weekday
	CheckDay      bool
	ClusterSize int
	Correlated int
}

type Decision struct {
	Allow      bool   `json:"allow"`
	Reason     string `json:"reason,omitempty"`
	Pack       string `json:"pack"`
	Version    int    `json:"policy_version"`
	KYC        string `json:"kyc,omitempty"`
	HoldReview bool   `json:"hold_review,omitempty"`
}

// Eval 用冻结当时的包。缺包或金额解析失败当不可用，调用方必须失败即关。
  func Eval(p Pack, in Input) (Decision, error) {
	  if strings.TrimSpace(p.Name) == "" || p.Doc.Version <= 0 {
		  return Decision{}, fmt.Errorf("policy pack required")
	   }
	   out := Decision{Pack: p.Name, Version: p.Doc.Version, KYC: p.KYC}
	  base, err := policy.Eval(p.Doc, policy.Request{
		   Kind: in.Kind, Amount: in.Amount, TodaySum: in.TodaySum,
		  Velocity: in.Velocity1m, Allowlisted: in.Allowlisted || !p.AllowlistRequired,
	   })
	   if err != nil {
		   return Decision{}, err
	  }
	  if !base.Allow {
		  out.Reason = base.Reason
		  return out, nil
	   }
	  if p.AllowlistRequired && in.Kind == policy.KindWithdraw && !in.Allowlisted {
		   out.Reason = "not allowlisted"
		  return out, nil
	   }
	  if p.HourlyVelocity > 0 && in.Velocity1h >= p.HourlyVelocity {
		   out.Reason = "hourly velocity"
		  return out, nil
	   }
	  if p.DailyVelocity > 0 && in.Velocity24h >= p.DailyVelocity {
		  out.Reason = "daily velocity"
		  return out, nil
	  }
	  if p.DestReuseMax > 0 && in.DestCount >= p.DestReuseMax {
		   out.Reason = "destination reuse"
		   return out, nil
	  }
	   if p.DestReuse24h > 0 && in.DestCount24h >= p.DestReuse24h {
		   out.Reason = "destination reuse 24h"
		  return out, nil
	  }
	  if p.ClusterMax > 0 && in.ClusterSize >= p.ClusterMax {
		  out.Reason = "destination cluster"
		  return out, nil
	   }
	  if p.CorrelationMax > 0 && in.Correlated >= p.CorrelationMax {
		  out.Reason = "destination correlation"
		   return out, nil
	  }
	  if p.NewDestDailyMax > 0 && in.FirstDest && in.NewDestsToday >= p.NewDestDailyMax {
		   out.Reason = "new destination daily cap"
		   return out, nil
	   }
	  if p.NightHold && inNight(in.HourUTC,
		  p.NightStartHour, p.NightEndHour) {
		  out.Reason = "night window"
		   out.HoldReview = true
		   return out, nil
	   }
	  if p.FirstDestHold && in.FirstDest {
		   out.Reason = "first destination hold"
		   out.HoldReview = true
		  return out, nil
	   }
	   if p.WeekendHold && in.CheckDay && (in.Weekday == time.Saturday || in.Weekday == time.Sunday) {
		   out.Reason = "weekend hold"
		  out.HoldReview = true
		  return out, nil
	  }
	   if capAmt, ok := p.AssetMax[strings.ToUpper(strings.TrimSpace(in.Asset))]; ok && capAmt != "" {
		  max, err := money.Parse(capAmt)
		   if err != nil {
			  return Decision{}, err
		   }
		   if in.Amount.Cmp(max) > 0 {
			  out.Reason = "asset cap"
			   return out, nil
		  }
	   }
	   out.Allow = true
	  return out, nil
  }

  func inNight(hour, start, end int) bool {
	  if start == end {
		   return false
	  }
	   if start < 0 {
		  start = 22
	  }
	  if end < 0 {
		  end = 6
	   }
	   if start < end {
		  return hour >= start && hour < end
	   }
	   return hour >= start || hour < end
  }

type Catalog struct {
	mu    sync.RWMutex
	packs map[string]Pack
}

   func NewCatalog() *Catalog {
	   c := &Catalog{packs: map[string]Pack{}}
	   for _, p := range DefaultPacks() {
		  c.packs[p.Name] = p
	   }
	  return c
  }

   func (c *Catalog) Put(p Pack) error {
	  if p.Name == "" || p.Doc.Version <= 0 {
		  return fmt.Errorf("pack name and version required")
	   }
	  c.mu.Lock()
	   defer c.mu.Unlock()
	   c.packs[p.Name] = p
	   return nil
  }

   func (c *Catalog) Get(name string) (Pack, error) {
	   c.mu.RLock()
	  defer c.mu.RUnlock()
	  p, ok := c.packs[name]
	   if !ok {
		   return Pack{}, fmt.Errorf("policy pack %s not found", name)
	   }
	  return p, nil
   }

  func (c *Catalog) Names() []string {
	  c.mu.RLock()
	   defer c.mu.RUnlock()
	   out := make([]string, 0, len(c.packs))
	  for n := range c.packs {
		   out = append(out, n)
	  }
	   return out
   }

  func (c *Catalog) ForKYC(level string) (Pack, error) {
	   level = strings.ToLower(strings.TrimSpace(level))
	  switch level {
	  case "", "0", "kyc0", "none":
		  return c.Get("kyc0")
	  case "1", "kyc1":
		  return c.Get("kyc1")
	  case "2", "kyc2":
		  return c.Get("kyc2")
	  case "vip":
		  return c.Get("vip")
	  default:
		   return c.Get("kyc0")
	  }
   }

  func DefaultPacks() []Pack {
	   v1 := policy.DefaultV1()
	  kyc0 := v1
	   kyc0.MaxSingle = "200"
	   kyc0.DailyWithdraw = "500"
	   kyc0.MaxVelocity = 5
	  kyc1 := v1
	  kyc1.MaxSingle = "2000"
	  kyc1.DailyWithdraw = "8000"
	  kyc1.MaxVelocity = 12
	   kyc2 := v1
	  kyc2.Version = 2
	   vip := v1
	   vip.Version = 3
	  vip.MaxSingle = "20000"
	   vip.DailyWithdraw = "80000"
	   vip.MaxVelocity = 40
	   return []Pack{
		   {
			   Name: "kyc0", KYC: "0", Doc: kyc0,
			   DestReuseMax: 3,
			   DestReuseWin: 24 * time.Hour,
			  HourlyVelocity: 3, DailyVelocity: 8,
			  NightHold: true, NightStartHour: 22, NightEndHour: 6,
			  FirstDestHold: true, NewDestDailyMax: 1, AllowlistRequired: true,
			   WeekendHold: true, AssetMax: map[string]string{"BTC": "0.05", "ETH": "2"},
			  ClusterMax: 4,
			  CorrelationMax: 4, DestReuse24h: 3,
		   },
		   {
			   Name: "kyc1", KYC: "1", Doc: kyc1,
			  DestReuseMax: 8, DestReuseWin: 24 * time.Hour,
			   HourlyVelocity: 8, DailyVelocity: 20,
			  NightHold: true, NightStartHour: 0, NightEndHour: 4,
			   FirstDestHold: true, NewDestDailyMax: 3, AllowlistRequired: true,
			   ClusterMax: 8, CorrelationMax: 8, DestReuse24h: 6,
		  },
		  {
			   Name: "kyc2", KYC: "2", Doc: kyc2,
			  DestReuseMax: 20, DestReuseWin: 24 * time.Hour,
			  HourlyVelocity: 15, DailyVelocity: 40,
			   FirstDestHold: false, NewDestDailyMax: 8, AllowlistRequired: true,
			   ClusterMax: 20, CorrelationMax: 20, DestReuse24h: 12,
		   },
		   {
			  Name: "vip", KYC: "vip", Doc: vip,
			   DestReuseMax: 50, DestReuseWin: 24 * time.Hour,
			   HourlyVelocity: 30, DailyVelocity: 80,
			  AllowlistRequired: false,
			  ClusterMax:        80, CorrelationMax: 80, DestReuse24h: 40,
		  },
	   }
   }
