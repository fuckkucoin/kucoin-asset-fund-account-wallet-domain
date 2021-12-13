package subkey

// MeshRoute 描述 fundmesh 一条路径映射。

type MeshRoute struct {
	PublicMethod string
	PublicPath   string
	InternalPath string
	Notes string
}

// MeshRoutes 返回完整映射表（只读）。
   func MeshRoutes() []MeshRoute {
	   return []MeshRoute{
		  {PublicMethod: "GET", PublicPath: "/api/v1/sub/api-key", InternalPath: "/v1/sub-keys", Notes: "Classic 列表"},
		  {PublicMethod: "POST", PublicPath: "/api/v1/sub/api-key", InternalPath: "/v1/sub-keys", Notes: "Classic 创建"},
		   {PublicMethod: "POST", PublicPath: "/api/v1/sub/api-key/update", InternalPath: "/v1/sub-keys", Notes: "Classic 修改 PATCH 语义"},
		   {PublicMethod: "DELETE", PublicPath: "/api/v1/sub/api-key", InternalPath: "/v1/sub-keys", Notes: "Classic 删除"},
		  {PublicMethod: "GET", PublicPath: "/api/ua/v1/sub/api-key", InternalPath: "/v1/sub-keys", Notes: "UA 列表"},
		   {PublicMethod: "GET", PublicPath: "/api/ua/v1/user/get-sub-api-key", InternalPath: "/v1/sub-keys", Notes: "UA 列表别名"},
		  {PublicMethod: "POST", PublicPath: "/api/ua/v1/user/create-sub-api-key", InternalPath: "/v1/sub-keys", Notes: "UA 创建"},
		   {PublicMethod: "POST", PublicPath: "/api/ua/v2/user/create-sub-api-key", InternalPath: "/v1/sub-keys", Notes: "UA v2 创建"},
		  {PublicMethod: "POST", PublicPath: "/api/ua/v1/user/modify-sub-api-key", InternalPath: "/v1/sub-keys", Notes: "UA 修改"},
		  {PublicMethod: "POST", PublicPath: "/api/ua/v2/user/modify-sub-api-key", InternalPath: "/v1/sub-keys", Notes: "UA v2 修改"},
		   {PublicMethod: "DELETE", PublicPath: "/api/ua/v1/user/sub-api-key", InternalPath: "/v1/sub-keys", Notes: "UA 删除"},
		   {PublicMethod: "DELETE", PublicPath: "/api/ua/v2/user/sub-api-key", InternalPath: "/v1/sub-keys", Notes: "UA v2 删除"},
		  {PublicMethod: "GET", PublicPath: "/api/v1/sub/api-key/audit", InternalPath: "/v1/sub-keys/audit", Notes: "审计只读"},
		  {PublicMethod: "GET", PublicPath: "/api/v1/sub/api-key/quota", InternalPath: "/v1/sub-keys/quota", Notes: "配额读"},
		   {PublicMethod: "POST", PublicPath: "/api/v1/sub/api-key/rotate", InternalPath: "/v1/sub-keys/rotate", Notes: "轮换 secret"},
		  {PublicMethod: "POST", PublicPath: "/api/v1/sub/api-key/revoke", InternalPath: "/v1/sub-keys/revoke", Notes: "吊销"},
	  }
  }

// MapPublicToInternal 将公开路径映射为 account-book 路径（query 保留）。
  func MapPublicToInternal(method, publicPath string) (string, bool) {
	   method = trimUpper(method)
	  path := publicPath
	  q := ""
	  if i := indexQuery(path); i >= 0 {
		  q = path[i:]
		  path = path[:i]
	   }
	   for _, r := range MeshRoutes() {
		   if trimUpper(r.PublicMethod) != method {
			  continue
		  }
		  if path == r.PublicPath {
			   return r.InternalPath + q, true
		   }
	  }
	  switch {
	   case method == "GET" && path == "/api/v1/sub/api-key":
		  return "/v1/sub-keys" + q, true
	  case method == "POST" && path == "/api/v1/sub/api-key":
		   return "/v1/sub-keys" + q, true
	   case method == "POST" && path == "/api/v1/sub/api-key/update":
		  return "/v1/sub-keys" + q, true
	  case method == "DELETE" && path == "/api/v1/sub/api-key":
		   return "/v1/sub-keys" + q, true
	  case method == "POST" && stringsHasSuffix(path, "create-sub-api-key"):
		   return "/v1/sub-keys" + q, true
	   case method == "POST" && stringsHasSuffix(path, "modify-sub-api-key"):
		  return "/v1/sub-keys" + q, true
	   case method == "DELETE" && stringsHasSuffix(path, "sub-api-key"):
		   return "/v1/sub-keys" + q, true
	   case method == "GET" && stringsHasSuffix(path, "get-sub-api-key"):
		   return "/v1/sub-keys" + q, true
	   case method == "GET" && path == "/api/ua/v1/sub/api-key":
		   return "/v1/sub-keys" + q, true
	   }
	   return "", false
  }

  func trimUpper(s string) string {
	  b := make([]byte, len(s))
	  for i := 0; i < len(s); i++ {
		   c := s[i]
		   if c >= 'a' && c <= 'z' {
			  c -= 'a' - 'A'
		  }
		  b[i] = c
	  }
	  return string(b)
   }

  func indexQuery(path string) int {
	   for i := 0; i < len(path); i++ {
		  if path[i] == '?' {
			   return i
		  }
	   }
	  return -1
  }

   func stringsHasSuffix(s, suf string) bool {
	  if len(s) < len(suf) {
		  return false
	   }
	   return s[len(s)-len(suf):] == suf
   }

// GatewayAssetRoutes query-gateway 资金页读路径。
   func GatewayAssetRoutes() []string {
	   return []string{
		   "GET /v1/assets/sub-keys",
		  "GET /v1/assets/sub-keys/permission-summary",
		  "GET /v1/assets/sub-keys/audit",
		   "GET /v1/assets/sub-keys/quota",
		   "GET /v1/assets/sub-keys/summary",
	  }
  }

// ContractHealth 契约探针条目数。
   func ContractHealth() map[string]any {
	  return map[string]any{
		   "mesh_routes":     len(MeshRoutes()),
		  "gateway_routes":  len(GatewayAssetRoutes()),
		   "permission_bits": len(PermissionCatalog()),
	   }
   }
