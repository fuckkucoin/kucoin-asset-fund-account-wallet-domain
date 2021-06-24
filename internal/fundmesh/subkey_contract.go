// @FileName: subkey_contract.go
// @Summary: fundmesh 模块 subkey_contract。
// 子户 API Key fundmesh 契约：公开路径与 account-book 下游映射摘要。
// @Author: Knox
// @Created: 2021.06.15

package fundmesh

// SubKeyMeshRoute 描述一条子户 Key mesh 路由。

type SubKeyMeshRoute struct {
	Method string
	Public string
	Book string
}

// SubKeyMeshCatalog 返回登记路径（运维只读）。

   func SubKeyMeshCatalog() []SubKeyMeshRoute {
	  return []SubKeyMeshRoute{
		   {Method: "GET", Public: "/api/v1/sub/api-key", Book: "/v1/sub-keys"},
		   {Method: "POST", Public: "/api/v1/sub/api-key", Book: "/v1/sub-keys"},
		   {Method: "PATCH", Public: "/api/v1/sub/api-key/update", Book: "/v1/sub-keys"},
		   {Method: "DELETE", Public: "/api/v1/sub/api-key", Book: "/v1/sub-keys"},
		  {Method: "POST", Public: "/api/v1/sub/api-key/rotate", Book: "/v1/sub-keys/rotate"},
		  {Method: "POST", Public: "/api/v1/sub/api-key/revoke", Book: "/v1/sub-keys/revoke"},
		   {Method: "GET", Public: "/api/v1/sub/api-key/audit", Book: "/v1/sub-keys/audit"},
		   {Method: "GET", Public: "/api/v1/sub/api-key/quota", Book: "/v1/sub-keys/quota"},
		   {Method: "POST", Public: "/api/ua/v1/user/create-sub-api-key", Book: "/v1/sub-keys"},
		  {Method: "POST", Public: "/api/ua/v1/user/modify-sub-api-key", Book: "/v1/sub-keys"},
		   {Method: "DELETE", Public: "/api/ua/v1/user/sub-api-key", Book: "/v1/sub-keys"},
	  }
  }

// SubKeyMeshHealth 探针条目数。

   func SubKeyMeshHealth() map[string]any {
	  return map[string]any{"routes": len(SubKeyMeshCatalog()), "service": "account"}
  }
