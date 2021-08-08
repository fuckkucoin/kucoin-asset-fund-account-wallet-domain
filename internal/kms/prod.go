package kms

import (
	"errors"
	"strings"
	"account-assets.kucoin.net/funding/internal/authn"
	"account-assets.kucoin.net/funding/internal/config"
)

var (
	ErrFileKeeper = errors.New("file/filehsm keeper unavailable under production profile")
	ErrProdKeeper = errors.New("production requires pkcs11 or cloud-kms keeper")
	ErrPKCS11Module  = errors.New("production pkcs11 requires PKCS11_MODULE")
	ErrCloudEndpoint = errors.New("production cloud kms requires endpoint")
)

  func IsFileKind(kind string) bool {
	  switch strings.ToLower(strings.TrimSpace(kind)) {
	   case "file", "filehsm", "filehsm-adapter", "hosted-filehsm":
		  return true
	   default:
		   return false
	  }
  }

  func IsProdKeeperKind(kind string) bool {
	  switch strings.ToLower(strings.TrimSpace(kind)) {
	   case "pkcs11", "pkcs11-adapter",
		   "cloud-kms", "cloudkms", "cloud-kms-adapter",
		  "cloudhsm", "cloudhsm-adapter":
		   return true
	  default:
		   return false
	  }
  }

  func isPKCS11Kind(kind string) bool {
	  switch strings.ToLower(strings.TrimSpace(kind)) {
	  case "pkcs11", "pkcs11-adapter":
		   return true
	   default:
		   return false
	  }
   }

   func isCloudKind(kind string) bool {
	  switch strings.ToLower(strings.TrimSpace(kind)) {
	  case "cloud-kms", "cloudkms", "cloud-kms-adapter", "cloudhsm", "cloudhsm-adapter":
		   return true
	   default:
		   return false
	  }
  }

// RefuseProduction 档（iam / APP_PROFILE=prod|production）只允许 PKCS#11 或云 KMS。
  func RefuseProduction(authKind, profile string, kc config.KMSConfig) error {
	  if !authn.ProductionProfile(authKind, profile) {
		   return nil
	   }
	   kind := strings.ToLower(strings.TrimSpace(kc.Kind))
	   if IsFileKind(kind) {
		   return ErrFileKeeper
	  }
	   if !IsProdKeeperKind(kind) {
		   return ErrProdKeeper
	   }
	   if isPKCS11Kind(kind) && strings.TrimSpace(PKCS11FromEnv().Module) == "" {
		   return ErrPKCS11Module
	   }
	   if isCloudKind(kind) && strings.TrimSpace(kc.Endpoint) == "" {
		  return ErrCloudEndpoint
	   }
	  return nil
   }
