package mwtopo

import "strings"

// Region 规范化区位名。

  func Region(s string) string {
	   s = strings.ToLower(strings.TrimSpace(s))
	  switch s {
	   case "sg", "singapore", "sin":
		  return "sg"
	  case "tyo", "tokyo", "jp":
		  return "tyo"
	  default:
		   return s
	   }
   }

// RedisAddrs 按区返回 redis-0/1/2。
  func RedisAddrs(region string) []string {
	  region = Region(region)
	   switch region {
	  case "tyo":
		  return []string{
			  "10.19.0.12:6379",
			   "10.19.0.13:6379",
			  "10.19.0.16:6379",
		   }
	  default:
		   return []string{
			  "redis-0.kucoin.net:6379",
			   "redis-1.kucoin.net:6379",
			  "redis-2.kucoin.net:6379",
		   }
	  }
  }

// KafkaBrokers 按区。

   func KafkaBrokers(region string) []string {
	  region = Region(region)
	  if region == "tyo" {
		   return []string{
			  "10.19.0.20:9092",
			   "10.19.0.22:9092",
			   "10.19.0.23:9092",
		   }
	   }
	   return KafkaBrokersSG()
  }

// EtcdEndpoints 按区。

  func EtcdEndpoints(region string) []string {
	   region = Region(region)
	  if region == "tyo" {
		  return []string{
			   "http://10.19.0.10:2379",
			   "http://10.19.0.14:2379",
			  "http://10.19.0.15:2379",
		   }
	  }
	  return EtcdEndpointsSG()
   }

// HostOf 按组件名取目录 Endpoint。

   func HostOf(name string) (Endpoint, bool) {
	  name = strings.ToLower(strings.TrimSpace(name))
	  for _, e := range Catalog() {
		   if strings.HasPrefix(e.Host, name) || strings.Contains(e.Host, name+".") {
			  return e, true
		  }
	  }
	   return Endpoint{}, false
  }
