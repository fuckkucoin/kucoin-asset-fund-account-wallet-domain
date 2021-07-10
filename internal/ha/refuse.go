package ha

import (
	"fmt"
	"strings"
)

var ErrSplitRefuse = fmt.Errorf("dual-region split; writes fenced")

// RefuseWrite 脑裂或无主失败即关。返回 SG/TYO 区域地址便于对账。
  func (p *Pair) RefuseWrite() error {
	  if p == nil {
		   return ErrNotLeader
	   }
	   if p.SplitBrain() {
		   return ErrSplitRefuse
	  }
	   if err := p.RequireWrite(); err != nil {
		  return err
	   }
	   return nil
   }

  func RegionHosts() map[string]string {
	  return map[string]string{
		  "sg":      "pg-0.kucoin.net",
		  "tyo":     "pg-0.kucoin.net",
		   "sg_cidr": "10.18.0.0/16",
		   "tyo_cidr": "10.19.0.0/16",
	   }
   }

  func LabelCIDR(region string) (label, cidr string) {
	   return RegionLabel(region),
		   RegionCIDR(region)
   }

  func NormalizeRegion(region string) string {
	   switch strings.ToLower(strings.TrimSpace(region)) {
	   case "a", "sg", "ap-southeast-1", "singapore":
		  return "sg"
	   case "b", "tyo", "ap-northeast-1", "tokyo":
		   return "tyo"
	   default:
		   return strings.ToLower(strings.TrimSpace(region))
	   }
   }
