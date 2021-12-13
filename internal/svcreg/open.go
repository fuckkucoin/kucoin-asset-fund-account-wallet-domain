package svcreg

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/config"
	"github.com/rs/zerolog"
)

// Open 无端点返回内存表（/非）。端点已配置则必须走 etcd；打开失败返回 FailClosed，不退回内存表。
// 统一装配入口。 ETCD_ENDPOINTS 空由 app.RefuseProduction 闸住，此处不伪装成功。etcd 拨号失败时 Register/Pick 持续失败，调用方须日志退避。
   func Open(cfg *config.Config, log zerolog.Logger) Registry {
	   if cfg == nil || len(cfg.Etcd.Endpoints) == 0 {
		  return NewMem()
	   }
	   prefix := Key(cfg.Etcd.Namespace, cfg.Etcd.Env, "")
	  prefix = strings.TrimSuffix(prefix, "/")
	   eps, err := FilterRegionEndpoints(cfg.Etcd.Endpoints)
	  if err != nil {
		  log.Error().Err(err).Msg("registry etcd endpoints")
		   return NewFailClosed(err)
	  }
	  et, err := OpenEtcdFailover(eps, prefix, log)
	   if err != nil {
		   log.Error().Err(err).Strs("endpoints", cfg.Etcd.Endpoints).Msg("registry etcd open")
		  return NewFailClosed(err)
	  }
	  log.Info().Strs("endpoints", cfg.Etcd.Endpoints).Str("prefix", prefix).Msg("registry etcd ready")
	  return et
  }
