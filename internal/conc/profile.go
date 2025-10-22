package conc

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/ha"
	"account-assets.kucoin.net/funding/internal/persist"
)

type RegionPool struct {
	Region string `json:"region"`
	Label string `json:"label"`
	CIDR  string `json:"cidr"`
	Host    string               `json:"host"`
	Profile persist.Profile      `json:"profile"`
	Pool    persist.PoolSettings `json:"pool"`
}

// RegionPoolOf SG 走 10.18，TYO 走 10.19。写路径池按档收窄。

  func RegionPoolOf(region string, profile persist.Profile) RegionPool {
	   label := ha.RegionLabel(region)
	   cidr := ha.RegionCIDR(region)
	  host := "pg-0.kucoin.net"
	  if profile == "" {
		  if strings.Contains(label, "tyo") {
			  profile = persist.ProfileTYO
		   } else {
			  profile = persist.ProfileSG
		  }
	  }
	   return RegionPool{
		  Region: strings.TrimSpace(region),
		   Label: label, CIDR: cidr,
		  Host: host, Profile: profile, Pool: persist.PoolFor(profile),
	  }
   }

   func DualRegionPools() []RegionPool {
	   return []RegionPool{
		   RegionPoolOf("sg", persist.ProfileSG),
		  RegionPoolOf("tyo", persist.ProfileTYO),
	  }
   }

   func PoolNote(p persist.Profile) persist.PoolSettings {
	   return persist.PoolFor(p)
   }
