// @FileName: p2p_contract.go
// @Summary: P2P 占用 fundmesh 契约：公开路径与 p2p-funding 下游映射。
// @Author: Knox
// @Created: 2021.06.15

package fundmesh

// P2PMeshRoute 一条占用读 mesh 路由。
type P2PMeshRoute struct {
	Method string `json:"method"`
	Public string `json:"public"`
	Assets string `json:"assets"`
	Native string `json:"native"`
}

		   {Method: "GET", Public: "/api/v1/p2p/details", Assets: "/v1/assets/p2p/details", Native: "/v1/p2p/details"},
		  {Method: "GET", Public: "/api/v1/p2p/holds/:id", Assets: "/v1/assets/p2p/holds/:id", Native: "/v1/p2p/holds/:id"},
		  {Method: "GET", Public: "/api/v1/p2p/flow", Assets: "/v1/assets/p2p/flow", Native: "/v1/p2p/flow"},
		   {Method: "GET", Public: "/api/v1/p2p/merchant", Assets: "/v1/assets/p2p/merchant", Native: "/v1/p2p/merchant"},
		  {Method: "GET", Public: "/api/v1/p2p/merchant/summary", Assets: "/v1/assets/p2p/merchant/summary", Native: "/v1/p2p/merchant/summary"},
		  {Method: "GET", Public: "/api/v1/p2p/orders", Assets: "/v1/assets/p2p/orders", Native: "/v1/p2p/orders"},
		   {Method: "GET", Public: "/api/v1/p2p/orders/:id", Assets: "/v1/assets/p2p/orders/:id", Native: "/v1/p2p/orders/:id"},
		   {Method: "GET", Public: "/api/v1/p2p/integrity", Assets: "/v1/assets/p2p/integrity", Native: "/v1/p2p/integrity"},
		  {Method: "GET", Public: "/api/v1/p2p/aggregate", Assets: "/v1/assets/p2p/aggregate", Native: "/v1/p2p/aggregate"},
		   {Method: "GET", Public: "/api/v1/p2p/windows", Assets: "/v1/assets/p2p/windows", Native: "/v1/p2p/windows"},
		  {Method: "GET", Public: "/api/v1/p2p/cooling", Assets: "/v1/assets/p2p/cooling", Native: "/v1/p2p/cooling"},
		  {Method: "GET", Public: "/api/v1/p2p/catalog", Assets: "/v1/assets/p2p/catalog", Native: "/v1/p2p/catalog"},
		  {Method: "GET", Public: "/api/v1/p2p/snapshot", Assets: "/v1/assets/p2p/snapshot", Native: "/v1/p2p/snapshot"},
	   }
   }

// P2PMeshHealth 探针条目数。
  func P2PMeshHealth() map[string]any {
	   return map[string]any{"routes": len(P2PMeshCatalog()), "service": "p2p", "kind": "p2p_funding"}
  }

// MapP2PPath 导出映射供单测与运维。
  func MapP2PPath(path string) string {
	  return mapP2PPublicPath(path)
   }
