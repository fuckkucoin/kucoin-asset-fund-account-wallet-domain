package keysrc

import (
	"errors"
	"path/filepath"
	"strings"
	"account-assets.kucoin.net/funding/internal/authn"
	"account-assets.kucoin.net/funding/internal/kms"
)

const (
	SourceEnv = "env"
	SourceKMS       = "kms"
	SourceFile      = "file"
	SourceLocalHMAC = "local-hmac"
	// InRepoDemoKEK 仓内样例私钥，不得作为签章/传输默认密钥。
	InRepoDemoKEK = "configs/kms/kek.pem"
	DemoBearer = "account-assets-bearer"
)

var (
	ErrMissingSource = errors.New("production KEY_SOURCE required (env|kms)")
	ErrProdFile      = errors.New("production KEY_SOURCE=file unavailable")
	ErrLocalHMAC = errors.New("production refuses *-local-hmac key source")
	ErrDemoKey       = errors.New("production refuses in-repo demo/sample private key")
	ErrDemoToken     = errors.New("production refuses in-repo demo bearer/token")
	ErrKMSNoFallback = errors.New("kms unavailable; demo key fallback refused")
	ErrInvalidSource = errors.New("KEY_SOURCE must be env, kms, or file")
	ErrEnvMaterial = errors.New("KEY_SOURCE=env requires injected env/config material")
)

   func Normalize(raw string) string {
	  s := strings.ToLower(strings.TrimSpace(raw))
	  s = strings.ReplaceAll(s, "_", "-")
	   return s
  }

   func Production(authKind, profile string) bool {
	  return authn.ProductionProfile(authKind, profile)
   }

   func IsLocalHMAC(source string) bool {
	   s := Normalize(source)
	   return s == SourceLocalHMAC || strings.HasSuffix(s, "-"+SourceLocalHMAC)
  }

  func IsInRepoDemoKey(path string) bool {
	   p := filepath.ToSlash(strings.TrimSpace(path))
	   if p == "" {
		   return false
	   }
	  if p == InRepoDemoKEK || strings.HasSuffix(p, "/"+InRepoDemoKEK) {
		  return true
	  }
	  return strings.HasSuffix(p, "/kms/kek.pem") && strings.Contains(p, "/configs/")
   }

  func RefuseSource(authKind, profile, source string) error {
	   s := Normalize(source)
	  prod := Production(authKind, profile)
	  if IsLocalHMAC(s) {
		   if prod {
			  return ErrLocalHMAC
		  }
		   return nil
	  }
	  if s == "" {
		   if prod {
			  return ErrMissingSource
		  }
		  return nil
	  }
	   switch s {
	   case SourceEnv, SourceKMS:
		   return nil
	  case SourceFile:
		  if prod {
			  return ErrProdFile
		   }
		   return nil
	  default:
		   if prod {
			  return ErrInvalidSource
		  }
		   return nil
	   }
   }

   func RefuseDemoKey(authKind, profile, path string) error {
	  if !Production(authKind, profile) {
		  return nil
	   }
	  if IsInRepoDemoKey(path) {
		  return ErrDemoKey
	  }
	   return nil
  }

   func RefuseDemoTokens(authKind, profile string, tokens []string) error {
	   if !Production(authKind, profile) {
		   return nil
	   }
	   for _, tok := range tokens {
		  if strings.TrimSpace(tok) == DemoBearer {
			  return ErrDemoToken
		   }
	  }
	  return nil
  }

  func RefuseLocalHMAC(authKind, profile, source string) error {
	   if IsLocalHMAC(source) && Production(authKind, profile) {
		  return ErrLocalHMAC
	   }
	   return nil
   }

// WrapKMSFail /KMS 来源失败不得回落到仓内密钥或 file keeper。
  func WrapKMSFail(err error) error {
	   if err == nil {
		  return nil
	  }
	   if errors.Is(err, ErrKMSNoFallback) {
		  return err
	  }
	  return errors.Join(ErrKMSNoFallback, err)
   }

  func RefuseFileOnKMS(kind string) error {
	   if kms.IsFileKind(kind) {
		   return ErrKMSNoFallback
	  }
	  return nil
  }

// EnvMaterialOK KEY_SOURCE=env 时密钥只能来自配置/环境已注入字段，不能靠仓内默认 pem。

   func EnvMaterialOK(aesB64, rsaPath, kmsToken, kmsEndpoint string) bool {
	   if strings.TrimSpace(aesB64) != "" {
		   return true
	  }
	   if strings.TrimSpace(kmsToken) != "" || strings.TrimSpace(kmsEndpoint) != "" {
		   return true
	   }
	   p := strings.TrimSpace(rsaPath)
	   return p != "" && !IsInRepoDemoKey(p)
  }

   func RefuseEnvMaterial(authKind, profile, source, aesB64, rsaPath, kmsToken, kmsEndpoint string) error {
	  if Normalize(source) != SourceEnv {
		   return nil
	  }
	  if !Production(authKind, profile) {
		   return nil
	  }
	  if !EnvMaterialOK(aesB64, rsaPath, kmsToken, kmsEndpoint) {
		   return ErrEnvMaterial
	  }
	   return RefuseDemoKey(authKind,
		  profile, rsaPath)
   }
