package ha

import (
	"fmt"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/metrics"
)

var ErrRegionIP = fmt.Errorf("region ip out of catalog cidr")

// ProbeSGWrite 写区 IP 必须落在新加坡 10.18.0.0/16。

   func ProbeSGWrite(ip string) error {
	   if !config.CatalogPrimaryIP(ip) {
		  metrics.RegionWriteOK.WithLabelValues("sg").Set(0)
		  return fmt.Errorf("%w: sg write", ErrRegionIP)
	   }
	   metrics.RegionWriteOK.WithLabelValues("sg").Set(1)
	  return nil
   }

// ProbeTYOStandby 备区 IP 必须落在东京 10.19.0.0/16。
  func ProbeTYOStandby(ip string) error {
	   if !config.CatalogRegionIP(ip) || config.CatalogPrimaryIP(ip) {
		   metrics.RegionWriteOK.WithLabelValues("tyo").Set(0)
		   return fmt.Errorf("%w: tyo standby", ErrRegionIP)
	  }
	   metrics.RegionWriteOK.WithLabelValues("tyo").Set(1)
	  return nil
  }

// ProbeFailover SG 写区与 TYO 备区 CIDR 均通过后记 failover 计数。不改主机。

  func ProbeFailover(sgWrite, tyoStandby string) error {
	  if err := ProbeSGWrite(sgWrite); err != nil {
		   return err
	  }
	   if err := ProbeTYOStandby(tyoStandby); err != nil {
		  return err
	   }
	   metrics.RegionFailover.WithLabelValues("probe_sg_tyo").Inc()
	  return nil
   }
