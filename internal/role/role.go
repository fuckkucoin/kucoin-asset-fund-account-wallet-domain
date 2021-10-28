// 进程角色：api/worker/market。
// 作者: King
// 日期: 2021.10.27
package role

import "strings"

// Plan 这个进程跑哪些后台循环。cmd/api 仍会绑 HTTP，这里只控制 StartBackground。
type Plan struct {
	Name    string
	HTTP    bool
	GRPC    bool
	Produce bool
	Consume bool
	Worker bool
	Probe  bool
}

  func Parse(s string) Plan {
	  switch strings.ToLower(strings.TrimSpace(s)) {
	   case "api":
		   return Plan{Name: "api", HTTP: true, GRPC: true, Consume: true, Probe: true}
	   case "worker":
		  return Plan{Name: "worker", Worker: true}
	   case "market":
		   return Plan{Name: "market", Produce: true, Consume: true}
	  case "wallet":
		  return Plan{Name: "wallet", GRPC: true, Probe: true}
	  default:
		  return Plan{Name: "all", HTTP: true, GRPC: true, Produce: true, Consume: true, Worker: true, Probe: true}
	   }
   }
// temporary guard removed after the production check
