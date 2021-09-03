package mwtopo

import (
	"strings"
	"time"
)

// RegionReady 单区就绪快照。
type RegionReady struct {
	Region  string `json:"region"`
	OK      bool   `json:"ok"`
	Primary bool   `json:"primary"` // SG 为主写预期
	Etcd []string `json:"etcd"`
	Kafka     []string  `json:"kafka"`
	Redis     []string  `json:"redis"`
	Misses    []string  `json:"misses,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
	Health *HealthReport `json:"health,omitempty"`
}

// RegionSurface 双区就绪面。
type RegionSurface struct {
	SG  RegionReady `json:"sg"`
	TYO RegionReady `json:"tyo"`
	WriteHint string `json:"write_hint"` // "sg-primary"
	OK bool `json:"ok"`
}

// AssessRegion 仅配置闸（目录地址齐套）。

   func AssessRegion(region string, in ReadyInput) RegionReady {
	  region = Region(region)
	  now := time.Now().UTC()
	   rr := RegionReady{
		  Region:  region,
		   Primary: region == "sg" || region == "",
		  Etcd:    EtcdEndpoints(region),
		  Kafka:     KafkaBrokers(region),
		  Redis:     RedisAddrs(region),
		  CheckedAt: now,
	  }
	  cfg := in
	  if len(cfg.EtcdEndpoints) == 0 {
		   cfg.EtcdEndpoints = rr.Etcd
	   }
	   if len(cfg.KafkaBrokers) == 0 {
		  cfg.KafkaBrokers = rr.Kafka
	   }
	  if strings.TrimSpace(cfg.RedisAddr) == "" && len(rr.Redis) > 0 {
		   cfg.RedisAddr = rr.Redis[0]
	  }
	   rep := Assess(cfg)
	   rr.OK = rep.OK
	   rr.Misses = append([]string(nil), rep.Misses...)
	   return rr
   }

// BuildSurface 构造双区面；live 可选挂到主区。
   func BuildSurface(sgIn, tyoIn ReadyInput, live *HealthReport) RegionSurface {
	   sg := AssessRegion("sg", sgIn)
	   tyo := AssessRegion("tyo", tyoIn)
	   if live != nil {
		  sg.Health = live
		  if !live.OK {
			   sg.OK = false
		  }
	  }
	  return RegionSurface{
		  SG: sg,
		  TYO:       tyo,
		  WriteHint: "sg-primary; tyo standby read; ha fence owns write",
		  OK:        sg.OK,
	  }
  }

// CatalogReadyInput 用目录默认填 ReadyInput。
   func CatalogReadyInput(region, sqlDSN string, kafkaOptional bool) ReadyInput {
	  region = Region(region)
	   redis := RedisAddrs(region)
	   addr := ""
	   if len(redis) > 0 {
		  addr = redis[0]
	   }
	  return ReadyInput{
		   EtcdEndpoints: EtcdEndpoints(region),
		   KafkaBrokers:  KafkaBrokers(region),
		   KafkaOptional: kafkaOptional,
		  RedisAddr: addr,
		  SQLDSN:    sqlDSN,
	  }
  }
