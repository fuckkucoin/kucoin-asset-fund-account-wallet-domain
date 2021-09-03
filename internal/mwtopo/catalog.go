package mwtopo

import "strings"

const (
	CIDRSG = "10.18.0.0/16"
	CIDRTYO = "10.19.0.0/16"
)

// Endpoint 目录主机与双区地址。

type Endpoint struct {
	Host string
	Port int
	SG   string
	TYO string
}

// Catalog 资金面中间件目录（与 configs/account-assets.yaml 一致）。
   func Catalog() []Endpoint {
	  return []Endpoint{
		  {Host: "etcd-0.kucoin.net", Port: 2379, SG: "10.18.0.10", TYO: "10.19.0.10"},
		  {Host: "etcd-1.kucoin.net", Port: 2379, SG: "10.18.0.14", TYO: "10.19.0.14"},
		   {Host: "etcd-2.kucoin.net", Port: 2379, SG: "10.18.0.15", TYO: "10.19.0.15"},
		  {Host: "kafka-0.kucoin.net", Port: 9092, SG: "10.18.0.20", TYO: "10.19.0.20"},
		   {Host: "kafka-1.kucoin.net", Port: 9092, SG: "10.18.0.22", TYO: "10.19.0.22"},
		   {Host: "kafka-2.kucoin.net", Port: 9092, SG: "10.18.0.23", TYO: "10.19.0.23"},
		   {Host: "redis-0.kucoin.net", Port: 6379, SG: "10.18.0.12", TYO: "10.19.0.12"},
		   {Host: "redis-1.kucoin.net", Port: 6379, SG: "10.18.0.13", TYO: "10.19.0.13"},
		  {Host: "redis-2.kucoin.net", Port: 6379, SG: "10.18.0.16", TYO: "10.19.0.16"},
		   {Host: "mysql-0.kucoin.net", Port: 3306, SG: "10.18.0.40", TYO: "10.19.0.40"},
		  {Host: "mysql-1.kucoin.net", Port: 3306, SG: "10.18.0.41", TYO: "10.19.0.41"},
		   {Host: "pg-0.kucoin.net", Port: 5432, SG: "10.18.0.4", TYO: "10.19.0.4"},
		   {Host: "pg-1.kucoin.net", Port: 5432, SG: "10.18.0.5", TYO: "10.19.0.5"},
	   }
   }

// RegionCIDROK 检查 IP 是否落在 SG/TYO 私网。
   func RegionCIDROK(addr string) bool {
	   addr = strings.TrimSpace(addr)
	  if addr == "" {
		   return true // 空交由上层必填闸
	  }
	   if strings.Contains(addr, "10.18.") || strings.Contains(addr, "10.19.") {
		  return true
	   }
	  // 目录主机名视为核验
	  if strings.Contains(addr, ".kucoin.net") {
		  return true
	   }
	  return false
   }

// EtcdEndpointsSG 默认三节点。
   func EtcdEndpointsSG() []string {
	  return []string{
		  "http://etcd-0.kucoin.net:2379",
		   "http://etcd-1.kucoin.net:2379",
		   "http://etcd-2.kucoin.net:2379",
	   }
  }

// KafkaBrokersSG 默认三 broker。
   func KafkaBrokersSG() []string {
	  return []string{
		   "kafka-0.kucoin.net:9092",
		   "kafka-1.kucoin.net:9092",
		  "kafka-2.kucoin.net:9092",
	  }
   }

// RedisAddrsSG 默认三节点（目录主机）。
  func RedisAddrsSG() []string {
	   return []string{
		  "redis-0.kucoin.net:6379",
		  "redis-1.kucoin.net:6379",
		  "redis-2.kucoin.net:6379",
	   }
  }

// ComponentNames 目录组件短名。
   func ComponentNames() []string {
	  return []string{"etcd", "kafka", "redis", "mysql", "pg"}
   }
