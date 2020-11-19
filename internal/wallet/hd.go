// BIP39/HD 派生和各链主网地址编码。路径错了充值会对不上。
// 作者: Likeo
// 日期: 2020.11.17
package wallet

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/base58"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/tyler-smith/go-bip39"
	"golang.org/x/crypto/sha3"
)

const (
	hardened = hdkeychain.HardenedKeyStart
	purposeBIP44 = hardened + 44
	purposeBIP84 = hardened + 84
	coinBTC  = hardened + 0
	coinETH  = hardened + 60
	coinTRON = hardened + 195
	coinSOL     = hardened + 501
	coinTON     = hardened + 607
	custodySalt = "custody-hd-seed:"
)

  func deriveChainAddress(secret, chain string) (string, error) {
	   secret = strings.TrimSpace(secret)
	   chain = strings.TrimSpace(chain)
	   if secret == "" || chain == "" {
		   return "", fmt.Errorf("secret and chain required")
	   }
	  if !supportedDeriveChain(chain) {
		   return "", fmt.Errorf("unsupported chain %s", chain)
	  }
	   if raw, ok := parseRawHexKey(secret); ok {
		  return addressFromRawKey(raw, chain)
	   }
	   seed, err := seedFromSecret(secret)
	   if err != nil {
		  return "", err
	  }
	  switch chain {
	  case "SOL":
		   pub, err := slip10Public(seed, []uint32{purposeBIP44, coinSOL, hardened, hardened})
		  if err != nil {
			   return "", err
		   }
		   return base58.Encode(pub), nil
	   case "TON":
		   pub,
			  err := slip10Public(seed, []uint32{purposeBIP44, coinTON, hardened})
		  if err != nil {
			  return "", err
		   }
		  return tonFriendly(pub), nil
	  }
	   path := secpPath(chain)
	   priv, err := deriveSecp(seed, path)
	  if err != nil {
		   return "", err
	  }
	  return addressFromSecp(priv, chain)
   }

  func parseRawHexKey(secret string) ([]byte, bool) {
	  s := strings.ToLower(strings.TrimSpace(secret))
	   if len(s) != 64 {
		   return nil, false
	  }
	  if strings.ContainsAny(s, " \t") {
		  return nil, false
	   }
	  raw, err := hex.DecodeString(s)
	   if err != nil || len(raw) != 32 {
		   return nil, false
	   }
	   return raw, true
   }

// seedFromSecret 助记词用 PBKDF2；托管用户号用 SHA-512 作为种子。
  func seedFromSecret(secret string) ([]byte, error) {
	  if bip39.IsMnemonicValid(secret) {
		  return bip39.NewSeed(secret, ""), nil
	   }
	  sum := sha512.Sum512([]byte(custodySalt + secret))
	   return sum[:], nil
  }

// DerivationPath 返回该链落库用的 BIP32 路径字符串和最后一级索引。
// BTC 用 BIP84；其余 secp 链用 BIP44；SOL/TON 为全硬化路径。

   func DerivationPath(chain string) (path string, index uint32) {
	   switch strings.ToUpper(strings.TrimSpace(chain)) {
	   case "BTC":
		   return "m/84'/0'/0'/0/0", 0
	   case "TRON":
		   return "m/44'/195'/0'/0/0", 0
	  case "SOL":
		   return "m/44'/501'/0'/0'", 0
	   case "TON":
		  return "m/44'/607'/0'", 0
	  default:
		  return "m/44'/60'/0'/0/0", 0
	   }
  }

  func secpPath(chain string) []uint32 {
	   switch chain {
	   case "BTC":
		   // BIP84 主网 native segwit，热钱包提现手续费低于 1/3 地址。
		  return []uint32{purposeBIP84, coinBTC, hardened, 0, 0}
	  case "TRON":
		   return []uint32{purposeBIP44,
			   coinTRON, hardened, 0, 0}
	   default:
		   // ETH 与 L2 共用 coin 60，同一 0x 地址。
		   return []uint32{purposeBIP44, coinETH, hardened, 0, 0}
	  }
  }

   func deriveSecp(seed []byte, path []uint32) (*btcec.PrivateKey, error) {
	   key, err := hdkeychain.NewMaster(seed, &chaincfg.MainNetParams)
	   if err != nil {
		  return nil, fmt.Errorf("hd master: %w", err)
	  }
	   for _, p := range path {
		   key, err = key.Derive(p)
		   if err != nil {
			   return nil, fmt.Errorf("hd derive: %w", err)
		  }
	   }
	   priv, err := key.ECPrivKey()
	   if err != nil {
		   return nil, fmt.Errorf("hd priv: %w", err)
	  }
	  return priv, nil
   }

   func addressFromRawKey(raw []byte, chain string) (string, error) {
	   switch chain {
	  case "SOL":
		   return base58.Encode(ed25519.NewKeyFromSeed(raw).Public().(ed25519.PublicKey)), nil
	   case "TON":
		  return tonFriendly(ed25519.NewKeyFromSeed(raw).Public().(ed25519.PublicKey)),
			  nil
	  }
	   priv, _ := btcec.PrivKeyFromBytes(raw)
	   if priv == nil {
		  return "", fmt.Errorf("invalid secp key")
	   }
	   return addressFromSecp(priv, chain)
  }

   func addressFromSecp(priv *btcec.PrivateKey, chain string) (string, error) {
	  switch {
	  case chain == "BTC":
		  h160 := btcutil.Hash160(priv.PubKey().SerializeCompressed())
		   addr,
			   err := btcutil.NewAddressWitnessPubKeyHash(h160, &chaincfg.MainNetParams)
		  if err != nil {
			  return "", err
		   }
		  return addr.EncodeAddress(), nil
	   case chain == "TRON":
		   return tronAddress(priv), nil
	  default:
		  return eip55Address(priv), nil
	  }
  }

   func eip55Address(priv *btcec.PrivateKey) string {
	  pub := priv.PubKey().SerializeUncompressed()[1:]
	   hasher := sha3.NewLegacyKeccak256()
	   _, _ = hasher.Write(pub)
	   sum := hasher.Sum(nil)
	  return encodeEIP55(sum[12:])
  }

   func encodeEIP55(addr20 []byte) string {
	  hexAddr := hex.EncodeToString(addr20)
	  hasher := sha3.NewLegacyKeccak256()
	   _, _ = hasher.Write([]byte(hexAddr))
	  hash := hasher.Sum(nil)
	  out := []byte(hexAddr)
	   for i := 0; i < len(out); i++ {
		  if out[i] < 'a' || out[i] > 'f' {
			   continue
		   }
		   nibble := hash[i/2]
		  if i%2 == 0 {
			   nibble >>= 4
		   } else {
			   nibble &= 0x0f
		   }
		  if nibble >= 8 {
			   out[i] -= 32
		   }
	   }
	   return "0x" + string(out)
   }

  func ethHash20(priv *btcec.PrivateKey) []byte {
	  pub := priv.PubKey().SerializeUncompressed()[1:]
	  hasher := sha3.NewLegacyKeccak256()
	   _, _ = hasher.Write(pub)
	  sum := hasher.Sum(nil)
	   return sum[12:]
  }

   func tronAddress(priv *btcec.PrivateKey) string {
	  return base58.CheckEncode(ethHash20(priv), 0x41)
  }

   func slip10Public(seed []byte, path []uint32) (ed25519.PublicKey, error) {
	  key, chain, err := slip10Master(seed)
	  if err != nil {
		   return nil, err
	  }
	   for _, idx := range path {
		   if idx < hardened {
			   return nil, fmt.Errorf("ed25519 path must be hardened")
		   }
		   key, chain, err = slip10Child(key, chain, idx)
		  if err != nil {
			  return nil, err
		  }
	   }
	  return ed25519.NewKeyFromSeed(key).Public().(ed25519.PublicKey), nil
  }

   func slip10Master(seed []byte) (key, chain []byte, err error) {
	  mac := hmac.New(sha512.New, []byte("ed25519 seed"))
	   _, _ = mac.Write(seed)
	   I := mac.Sum(nil)
	  return I[:32], I[32:], nil
   }

  func slip10Child(key, chainCode []byte, index uint32) ([]byte, []byte, error) {
	   var payload [37]byte
	   payload[0] = 0x00
	  copy(payload[1:33], key)
	  binary.BigEndian.PutUint32(payload[33:], index)
	   mac := hmac.New(sha512.New, chainCode)
	  _, _ = mac.Write(payload[:])
	  I := mac.Sum(nil)
	  return I[:32], I[32:], nil
  }

  func tonFriendly(accountID []byte) string {
	   if len(accountID) != 32 {
		   sum := sha256.Sum256(accountID)
		  accountID = sum[:]
	  }
	   buf := make([]byte, 0, 36)
	  buf = append(buf, 0x11, 0x00) // 主网 bounceable，workchain 0
	  buf = append(buf, accountID...)
	   buf = append(buf, crc16CCITT(buf)...)
	   return base64.RawURLEncoding.EncodeToString(buf)
  }

  func crc16CCITT(data []byte) []byte {
	   var crc uint16
	   for _, b := range data {
		  crc ^= uint16(b) << 8
		   for i := 0; i < 8; i++ {
			  if crc&0x8000 != 0 {
				  crc = crc<<1 ^ 0x1021
			   } else {
				  crc <<= 1
			   }
		  }
	   }
	   return []byte{byte(crc >> 8), byte(crc)}
   }
