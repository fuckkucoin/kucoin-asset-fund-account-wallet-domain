package mwtopo

import "strings"

// AlertsFromHealth 由汇总报告生成告警。
  func AlertsFromHealth(rep HealthReport) []Alert {
	  ready := map[string]bool{}
	  detail := map[string]string{}
	   region := ""
	   for _, c := range rep.Components {
		  name := strings.ToLower(strings.TrimSpace(c.Name))
		   switch name {
		  case "etcd":
			   ready["etcd"] = c.OK
			  detail["etcd"] = c.Detail
		  case "kafka":
			   ready["kafka"] = c.OK
			  detail["kafka"] = c.Detail
		  case "redis":
			  ready["redis"] = c.OK
			  detail["redis"] = c.Detail
		  case "outbox_worker", "outbox":
			   ready["outbox"] = c.OK
			  detail["outbox"] = c.Detail
		   case "worker":
			   ready["worker"] = c.OK
			   detail["worker"] = c.Detail
		  }
		   if region == "" && c.Region != "" {
			  region = c.Region
		   }
	  }
	   return AlertsFromReady(region, ready, detail)
   }

// ReadyMapFromReport 便捷 map。

  func ReadyMapFromReport(rep HealthReport) map[string]bool {
	   out := map[string]bool{}
	   for _, c := range rep.Components {
		   out[strings.ToLower(c.Name)] = c.OK
	   }
	   return out
   }
