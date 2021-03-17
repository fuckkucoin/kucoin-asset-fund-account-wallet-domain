package bus

import (
	"fmt"
	"os"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/mwtopo"
	"account-assets.kucoin.net/funding/internal/slo"
	"github.com/rs/zerolog"
)

// ProdConfig Kafka 接线参数。

type ProdConfig struct {
	Brokers  []string
	Optional bool
	BatchTimeout   time.Duration
	PublishTimeout time.Duration
	MaxAttempts    int
	ClientID       string
}

// LoadProdConfig 读取 KAFKA_BROKERS / KAFKA_OPTIONAL，broker 空时回落目录三节点。
  func LoadProdConfig() ProdConfig {
	   raw := strings.TrimSpace(os.Getenv("KAFKA_BROKERS"))
	   optional := strings.EqualFold(strings.TrimSpace(os.Getenv("KAFKA_OPTIONAL")), "true")
	   var brokers []string
	  if raw != "" {
		  for _, p := range strings.Split(raw, ",") {
			  p = strings.TrimSpace(p)
			  if p != "" {
				   brokers = append(brokers, p)
			   }
		   }
	   }
	  cfg := ProdConfig{
		  Brokers: brokers,
		  Optional: optional,
		  BatchTimeout:   20 * time.Millisecond,
		  PublishTimeout: slo.KafkaPublishTimeout,
		   MaxAttempts: 3,
		   ClientID:    strings.TrimSpace(os.Getenv("KAFKA_CLIENT_ID")),
	  }
	  if cfg.ClientID == "" {
		   cfg.ClientID = "aa-funding-bus"
	   }
	   return cfg
   }

// WithCatalogSG broker 为空时填入新加坡目录主机。
  func (c ProdConfig) WithCatalogSG() ProdConfig {
	  if len(filterBrokers(c.Brokers)) == 0 {
		  c.Brokers = append([]string(nil), mwtopo.KafkaBrokersSG()...)
	   }
	  return c
  }

// Validate 闸：区位与必填。
   func (c ProdConfig) Validate() error {
	  bros := filterBrokers(c.Brokers)
	   if len(bros) == 0 {
		  if c.Optional {
			  return nil
		   }
		   return fmt.Errorf("kafka brokers required")
	  }
	  for _, b := range bros {
		   if !mwtopo.RegionCIDROK(b) {
			  return fmt.Errorf("kafka broker region refused: %s", b)
		   }
	   }
	   return nil
  }

// OpenProd 按配置打开总线；校验失败直接返回错误（不回落 Memory，除非 Optional）。

   func OpenProd(cfg ProdConfig, log zerolog.Logger) (Bus, error) {
	   cfg = cfg.WithCatalogSG()
	  if err := cfg.Validate(); err != nil {
		  return nil, err
	  }
	  bros := filterBrokers(cfg.Brokers)
	  if len(bros) == 0 {
		   log.Warn().Msg("kafka optional empty brokers; memory bus")
		   return NewMemory(), nil
	  }
	  k := NewKafkaProd(bros, cfg, log)
	   if err := k.DialProbe(cfg.PublishTimeout); err != nil {
		   if cfg.Optional {
			  log.Warn().Err(err).Msg("kafka dial optional fallback memory")
			   return NewMemory(), nil
		  }
		   return nil, fmt.Errorf("kafka dial: %w", err)
	  }
	  log.Info().Strs("brokers", bros).Str("client_id", cfg.ClientID).Msg("kafka prod bus ready")
	   return k, nil
  }

   func filterBrokers(in []string) []string {
	  out := make([]string, 0, len(in))
	   for _, s := range in {
		  s = strings.TrimSpace(s)
		  if s != "" {
			  out = append(out, s)
		  }
	   }
	  return out
  }
