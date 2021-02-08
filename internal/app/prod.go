package app

import (
	"errors"
	"os"
	"strings"
	"account-assets.kucoin.net/funding/internal/authn"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/keysrc"
	"account-assets.kucoin.net/funding/internal/persist"
)

var (
	ErrProdDSN = errors.New("production Open requires MYSQL_DSN or ACCOUNT_ASSETS_PG_DSN")
	ErrProdIAM = errors.New("production AUTH_KIND=iam requires issuer and JWKS URL")
	ErrProdKYT      = errors.New("production requires KYT vendor mode and vendor URL")
	ErrProdInsecure = errors.New("production refuses AUTH_ALLOW_INSECURE")
	ErrProdMnemonic = errors.New("production refuses WALLET_REVEAL_MNEMONIC")
	ErrProdAuthOff = errors.New("production requires auth.enabled")
	ErrProdConfirm = errors.New("production requires transfer.confirm_depth >= 1")
	ErrProdRegistry = errors.New("production requires ETCD_ENDPOINTS for registry and config")
	ErrProdRegion   = errors.New("production cluster endpoints must use catalog region addresses")
	ErrProdFiat     = errors.New("production fiat vendor requires official base url and api key")
	ErrProdRedis = errors.New("production Open requires REDIS_ADDR")
	ErrProdKafka = errors.New("production requires KAFKA_BROKERS unless KAFKA_OPTIONAL")
	ErrProdFillIdent = errors.New("production requires CONVERT_FILL_REQUIRE_IDENT with HMAC secret or mTLS CNs")
)

// RefuseProduction 档（iam / APP_PROFILE=prod|production）按上线口径失败即关。
// 缺 DSN / PKCS#11 或云 KMS / IAM JWKS / KYT 厂商 / 确认深度 / 注册中心，
// 集群连接串出现目录区域以外的 IP，或打开不安全鉴权、助记词回显，进程不启动。
  func RefuseProduction(cfg *config.Config) error {
	   if cfg == nil {
		   return ErrMemForbidden
	  }
	   if !authn.ProductionProfile(cfg.Auth.Kind, cfg.Profile) {
		  return nil
	  }
	   if err := RefuseFileHSM(cfg); err != nil {
		   return err
	  }
	  if err := keysrc.RefuseSource(cfg.Auth.Kind, cfg.Profile, cfg.KeySource); err != nil {
		  return err
	   }
	  if err := keysrc.RefuseDemoKey(cfg.Auth.Kind, cfg.Profile, cfg.RSAKeyPath); err != nil {
		   return err
	  }
	   if err := keysrc.RefuseDemoKey(cfg.Auth.Kind, cfg.Profile, cfg.KMS.KEKPath); err != nil {
		   return err
	  }
	  if err := keysrc.RefuseDemoTokens(cfg.Auth.Kind, cfg.Profile, cfg.Auth.BearerTokens); err != nil {
		  return err
	  }
	   if err := keysrc.RefuseEnvMaterial(cfg.Auth.Kind, cfg.Profile, cfg.KeySource, cfg.AESKeyB64, cfg.RSAKeyPath, os.Getenv("KMS_TOKEN"), cfg.KMS.Endpoint); err != nil {
		  return err
	   }
	   if err := keysrc.RefuseLocalHMAC(cfg.Auth.Kind, cfg.Profile, cfg.KeySource); err != nil {
		  return err
	  }
	   if strings.TrimSpace(cfg.MySQLDSN) == "" && strings.TrimSpace(cfg.Postgres.DSN) == "" {
		   return ErrProdDSN
	  }
	   if strings.EqualFold(strings.TrimSpace(cfg.Auth.Kind), "iam") {
		   if strings.TrimSpace(cfg.Auth.Issuer) == "" || strings.TrimSpace(cfg.Auth.JWKSURL) == "" {
			  return ErrProdIAM
		   }
	   }
	   kytMode := strings.ToLower(strings.TrimSpace(cfg.KYT.Mode))
	  if kytMode == "" || kytMode == "allow" {
		  return ErrProdKYT
	  }
	   if kytMode == "vendor" && strings.TrimSpace(cfg.KYT.VendorURL) == "" {
		  return ErrProdKYT
	  }
	  if cfg.Auth.AllowInsecure {
		   return ErrProdInsecure
	   }
	  if cfg.WalletRevealMnemonic {
		   return ErrProdMnemonic
	   }
	   if !cfg.Auth.Enabled {
		  return ErrProdAuthOff
	   }
	  if cfg.Transfer.ConfirmDepth < 1 {
		   return ErrProdConfirm
	  }
	  if len(cfg.Etcd.Endpoints) == 0 {
		  return ErrProdRegistry
	  }
	  if strings.TrimSpace(cfg.RedisAddr) == "" {
		  return ErrProdRedis
	   }
	  if !cfg.KafkaOptional && len(cfg.KafkaBrokers) == 0 {
		   return ErrProdKafka
	   }
	   if err := refuseClusterRegion(cfg); err != nil {
		   return err
	  }
	  if v := strings.TrimSpace(cfg.Funds.FiatVendor); v != "" {
		   if config.OfficialFiatBase(v) == "" || strings.TrimSpace(cfg.Funds.FiatAPIKey) == "" {
			   return ErrProdFiat
		   }
	  }
	   // 强制闪兑成交身份闸；未开 REQUIRE 或缺 HMAC/mTLS 失败即关。
	  if err := refuseConvertFillIdent(cfg); err != nil {
		   return err
	   }
	   if err := refuseGateMemStorage(cfg); err != nil {
		  return err
	   }
	  return nil
  }

// refuseGateMemStorage 档禁止 GATE_STORAGE/STORAGE=mem，禁止静默落到进程内 Register。
// 环境变量 GATE_STORAGE 先：yaml 写 sql 但进程仍标 mem 时失败即关。

  func refuseGateMemStorage(cfg *config.Config) error {
	   if cfg == nil {
		   return persist.ErrMemRegister
	   }
	  st := strings.ToLower(strings.TrimSpace(cfg.Storage))
	   if env := strings.ToLower(strings.TrimSpace(os.Getenv("GATE_STORAGE"))); env != "" {
		   st = env
	   } else if st == "" {
		   st = strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE")))
	   }
	   return persist.RefuseMemRegister(true, st, st == "mem")
   }

// refuseConvertFillIdent 强制 CONVERT_FILL_REQUIRE_IDENT=1/true，且须配 HMAC 或 mTLS CN。
// 超越 X-Role；maker 回调必须可验签。
// 仅角色闸在不可接受。

  func refuseConvertFillIdent(cfg *config.Config) error {
	  req := strings.TrimSpace(os.Getenv("CONVERT_FILL_REQUIRE_IDENT"))
	  secret := strings.TrimSpace(os.Getenv("CONVERT_FILL_HMAC_SECRET"))
	  mtls := strings.TrimSpace(os.Getenv("CONVERT_FILL_MTLS_CNS"))
	  on := strings.EqualFold(req, "1") || strings.EqualFold(req, "true")
	  if cfg != nil {
		  if cfg.ConvertFill.RequireIdent {
			  on = true
		  }
		  if strings.TrimSpace(cfg.ConvertFill.HMACSecret) != "" {
			  secret = cfg.ConvertFill.HMACSecret
		  }
		  if len(cfg.ConvertFill.MTLSCNs) > 0 {
			  mtls = strings.Join(cfg.ConvertFill.MTLSCNs, ",")
		  }
	  }
	  if !on {
		  return ErrProdFillIdent
	  }
	  if secret == "" && mtls == "" {
		  return ErrProdFillIdent
	  }
	  return nil
  }

  func refuseClusterRegion(cfg *config.Config) error {
	  eps := []string{cfg.MySQLDSN, cfg.Postgres.DSN, cfg.RedisAddr}
	  eps = append(eps, cfg.KafkaBrokers...)
	   eps = append(eps, cfg.Etcd.Endpoints...)
	  for _, e := range eps {
		  if !config.ClusterEndpointOK(e) {
			  return ErrProdRegion
		   }
	  }
	   return nil
  }
