package etcdx

// LiteralSG 新加坡 etcd 地址（含端口，IP 形式）。
   func LiteralSG() []string {
	   return []string{
		  "http://10.18.0.10:2379",
		  "http://10.18.0.14:2379",
		  "http://10.18.0.15:2379",
	   }
  }

// LiteralTYO 东京 etcd 地址（IP 形式）。

  func LiteralTYO() []string {
	   return []string{
		   "http://10.19.0.10:2379",
		   "http://10.19.0.14:2379",
		   "http://10.19.0.15:2379",
	  }
   }

// CatalogHostSG 目录主机（带端口）。

   func CatalogHostSG() []string {
	   return []string{
		   "http://etcd-0.kucoin.net:2379",
		   "http://etcd-1.kucoin.net:2379",
		  "http://etcd-2.kucoin.net:2379",
	   }
   }

// CatalogHostTYO 东京目录（主机名或 IP）。

   func CatalogHostTYO() []string {
	   return []string{
		  "http://10.19.0.10:2379",
		  "http://10.19.0.14:2379",
		   "http://10.19.0.15:2379",
	   }
   }

// ForRegion 综合选择。

   func ForRegion(region string, literal bool) EndpointSet {
	  region = normalizeRegion(region)
	   var eps []string
	  switch {
	  case region == "tyo" && literal:
		  eps = LiteralTYO()
	  case region == "tyo":
		   eps = CatalogHostTYO()
	  case literal:
		   eps = LiteralSG()
	  default:
		  eps = CatalogHostSG()
	  }
	   return EndpointSet{Region: region, Endpoints: eps}
  }

   func normalizeRegion(s string) string {
	  switch s {
	   case "tyo", "tokyo", "jp":
		   return "tyo"
	   default:
		   return "sg"
	   }
   }
