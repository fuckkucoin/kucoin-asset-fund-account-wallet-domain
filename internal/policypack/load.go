package policypack

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/policy"
)

type filePack struct {
	Name string `json:"name"`
	KYC string `json:"kyc"`
	Doc fileDoc `json:"doc"`
	DestReuseMax    int    `json:"dest_reuse_max"`
	DestReuseWindow string `json:"dest_reuse_window"`
	HourlyVelocity  int    `json:"hourly_velocity"`
	DailyVelocity int `json:"daily_velocity"`
	NightHold bool `json:"night_hold"`
	NightStartHour int `json:"night_start_hour"`
	NightEndHour  int  `json:"night_end_hour"`
	FirstDestHold bool `json:"first_dest_hold"`
	NewDestDailyMax int `json:"new_dest_daily_max"`
	AllowlistRequired bool `json:"allowlist_required"`
	WeekendHold       bool `json:"weekend_hold"`
	AssetMax map[string]string `json:"asset_max"`
}

type fileDoc struct {
	Version   int    `json:"version"`
	MaxSingle string `json:"max_single"`
	DailyWithdraw string `json:"daily_withdraw"`
	DailyDeposit  string `json:"daily_deposit"`
	MinAmount     string `json:"min_amount"`
	MaxVelocity int `json:"max_velocity"`
	VelocityWindow string `json:"velocity_window"`
	Allowlist bool `json:"allowlist"`
}

   func (f filePack) toPack() (Pack, error) {
	   if strings.TrimSpace(f.Name) == "" || f.Doc.Version <= 0 {
		  return Pack{}, fmt.Errorf("pack name and version required")
	   }
	  win, err := parseDur(f.DestReuseWindow, 24*time.Hour)
	   if err != nil {
		   return Pack{}, err
	   }
	   vwin, err := parseDur(f.Doc.VelocityWindow, time.Minute)
	   if err != nil {
		   return Pack{}, err
	   }
	   return Pack{
		  Name: f.Name, KYC: f.KYC,
		   Doc: policy.Doc{
			   Version: f.Doc.Version, MaxSingle: f.Doc.MaxSingle, DailyWithdraw: f.Doc.DailyWithdraw,
			  DailyDeposit: f.Doc.DailyDeposit, MinAmount: f.Doc.MinAmount, MaxVelocity: f.Doc.MaxVelocity,
			   VelocityWin: vwin, Allowlist: f.Doc.Allowlist,
		   },
		  DestReuseMax: f.DestReuseMax, DestReuseWin: win,
		   HourlyVelocity: f.HourlyVelocity, DailyVelocity: f.DailyVelocity,
		  NightHold: f.NightHold, NightStartHour: f.NightStartHour, NightEndHour: f.NightEndHour,
		   FirstDestHold: f.FirstDestHold, NewDestDailyMax: f.NewDestDailyMax,
		  AllowlistRequired: f.AllowlistRequired, WeekendHold: f.WeekendHold,
		  AssetMax: f.AssetMax,
	  }, nil
   }

   func parseDur(s string, fallback time.Duration) (time.Duration, error) {
	   s = strings.TrimSpace(s)
	  if s == "" {
		  return fallback, nil
	  }
	  d, err := time.ParseDuration(s)
	  if err != nil {
		  return 0, err
	  }
	  return d, nil
   }

   func LoadFile(path string) (Pack, error) {
	  raw, err := os.ReadFile(path)
	   if err != nil {
		   return Pack{}, err
	  }
	   var f filePack
	  if err := json.Unmarshal(raw, &f); err != nil {
		  return Pack{}, err
	  }
	  return f.toPack()
  }

   func LoadDir(dir string) (*Catalog, error) {
	  c := NewCatalog()
	  if strings.TrimSpace(dir) == "" {
		  return c, nil
	  }
	  ents, err := os.ReadDir(dir)
	  if err != nil {
		   return nil, err
	   }
	  n := 0
	  for _, e := range ents {
		   if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			  continue
		  }
		   p, err := LoadFile(filepath.Join(dir, e.Name()))
		   if err != nil {
			   return nil, fmt.Errorf("%s: %w", e.Name(), err)
		  }
		   if err := c.Put(p); err != nil {
			  return nil, err
		  }
		   n++
	  }
	  if n == 0 {
		   return nil, fmt.Errorf("no policy packs in %s", dir)
	   }
	   return c, nil
   }
