package regioinv

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/ha"
	"account-assets.kucoin.net/funding/internal/mwtopo"
)

// Site 是从 mwtopo 目录读出的库存区种子。本包不选主。

type Site struct {
	Code string
	Name string
	Role string
	CIDR string
}

// CatalogSites 只暴露 sg / tyo。写权仍归 ha。
  func CatalogSites() []Site {
	  return []Site{
		  {Code: "sg", Name: "singapore", Role: RolePrimary, CIDR: mwtopo.CIDRSG},
		  {Code: "tyo", Name: "tokyo", Role: RoleStandby, CIDR: mwtopo.CIDRTYO},
	  }
  }

// BindCatalog 把库存区绑到目录区位。code 不是 sg/tyo 时必须显式给出 catalog。

  func BindCatalog(code, catalog string) (string, string, error) {
	  code = strings.ToLower(strings.TrimSpace(code))
	  if code == "" || strings.ContainsAny(code, " \t|") || len(code) > 32 {
		  return "", "", ErrInvalid
	  }
	  cat := strings.ToLower(strings.TrimSpace(catalog))
	  if cat == "" {
		  cat = mwtopo.Region(code)
	  } else {
		  cat = mwtopo.Region(cat)
	  }
	  if cat != "sg" && cat != "tyo" {
		   return "", "", ErrCatalog
	   }
	   return code, cat, nil
   }

// DefaultRole 未指定角色时按目录主备填写。

  func DefaultRole(catalog, role string) string {
	  role = strings.ToLower(strings.TrimSpace(role))
	   switch role {
	   case RolePrimary, RoleStandby, RoleDC:
		   return role
	  }
	  if catalog == "sg" {
		   return RolePrimary
	   }
	  if catalog == "tyo" {
		  return RoleStandby
	   }
	   return RoleDC
  }

// ProbeWrite 用 ha 的只读 CIDR 探针核对写地址。空地址不探测。
   func ProbeWrite(catalog, ip string) error {
	  ip = strings.TrimSpace(ip)
	  if ip == "" {
		  return nil
	   }
	   switch catalog {
	  case "sg":
		  return ha.ProbeSGWrite(ip)
	  case "tyo":
		   return ha.ProbeTYOStandby(ip)
	   default:
		  return ErrCatalog
	   }
   }

// LinkProbe 把目录就绪和 ha 探针收成库存域的故障报告。不改选主状态。

  func LinkProbe(regionCode, catalog, writeIP string, ready *mwtopo.RegionReady) (ProbeReport, error) {
	   code := strings.ToLower(strings.TrimSpace(regionCode))
	  if code == "" {
		  code = catalog
	  }
	   _, cat, err := BindCatalog(code, catalog)
	  if err != nil {
		   return ProbeReport{}, err
	  }
	   rep := ProbeReport{RegionCode: code, Ready: true, Detail: "catalog " + cat}
	  if err := ProbeWrite(cat, writeIP); err != nil {
		   rep.Ready = false
		  rep.Detail = err.Error()
	  }
	   if ready != nil && !ready.OK {
		   rep.Ready = false
		  if len(ready.Misses) > 0 {
			   rep.Detail = strings.Join(ready.Misses, ",")
		  }
	   }
	   return rep, nil
  }

// CatalogReady 只读 mwtopo 的区域地址齐套情况。sqlDSN 为空时不伪造连接串。
  func CatalogReady(catalog, sqlDSN string) mwtopo.RegionReady {
	  cat := mwtopo.Region(catalog)
	  in := mwtopo.ReadyInput{
		  EtcdEndpoints: mwtopo.EtcdEndpoints(cat),
		  KafkaBrokers:  mwtopo.KafkaBrokers(cat),
		  KafkaOptional: true,
	  }
	   if addrs := mwtopo.RedisAddrs(cat); len(addrs) > 0 {
		  in.RedisAddr = addrs[0]
	   }
	   if strings.TrimSpace(sqlDSN) != "" {
		  in.SQLDSN = sqlDSN
	   } else if in.RedisAddr != "" {
		   // Assess 把空 SQL 当成缺项。目录探针只描述中间件地址是否在册，不代替库存故障。
		  in.SQLDSN = "mysql-0.kucoin.net"
	   }
	  return mwtopo.AssessRegion(cat, in)
  }
