package persist

import (
	"fmt"
	"strings"
)

const (
	RegionSG = "sg"
	RegionTYO = "tyo"
	HostPG = "pg-0.kucoin.net"
	CIDRSG = "10.18.0.0/16"
	CIDRTYO = "10.19.0.0/16"
)

type RegionDSN struct {
	Region string `json:"region"`
	Host   string `json:"host"`
	CIDR string `json:"cidr"`
	DSN  string `json:"dsn"`
	Pool Profile
}

  func RegionOf(s string) string {
	   switch strings.ToLower(strings.TrimSpace(s)) {
	   case RegionSG, "a", "singapore":
		   return RegionSG
	   case RegionTYO, "b", "tokyo":
		  return RegionTYO
	   default:
		  return strings.ToLower(strings.TrimSpace(s))
	  }
   }

   func CIDROf(region string) string {
	   switch RegionOf(region) {
	   case RegionSG:
		   return CIDRSG
	  case RegionTYO:
		   return CIDRTYO
	   default:
		  return ""
	  }
   }

  func MiddlewareAddr(region, kind string) string {
	   switch RegionOf(region) {
	   case RegionSG:
		   switch strings.ToLower(strings.TrimSpace(kind)) {
		   case "redis":
			  return "10.18.0.12:6379"
		   case "kafka":
			   return "10.18.0.20:9092"
		  default:
			  return "10.18.0.4:5432"
		  }
	   case RegionTYO:
		  switch strings.ToLower(strings.TrimSpace(kind)) {
		  case "redis":
			  return "10.19.0.12:6379"
		   case "kafka":
			  return "10.19.0.20:9092"
		  default:
			   return "10.19.0.4:5432"
		   }
	   default:
		   return HostPG + ":5432"
	  }
   }

   func DSNFor(region string) RegionDSN {
	  reg := RegionOf(region)
	  host := HostPG
	  cidr := CIDROf(reg)
	   addr := MiddlewareAddr(reg, "pg")
	  prof := ProfileSG
	  if reg == RegionTYO {
		   prof = ProfileTYO
	   }
	  dsn := fmt.Sprintf("postgres://aa@%s/aa?host=%s", addr, host)
	   return RegionDSN{Region: reg, Host: host, CIDR: cidr, DSN: dsn, Pool: prof}
  }

   func DualRegion() []RegionDSN {
	  return []RegionDSN{DSNFor(RegionSG), DSNFor(RegionTYO)}
   }
