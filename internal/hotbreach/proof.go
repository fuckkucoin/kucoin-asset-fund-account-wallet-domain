package hotbreach

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"github.com/google/uuid"
)

// buildProof 从 BreachCase、PreLease 与热库存快照构造击穿证明。

   func buildProof(bc BreachCase, lease PreLease, snap hotinv.Snapshot, now time.Time) (BreachProof, error) {
	   rawSnap, err := json.Marshal(snap)
	  if err != nil {
		  return BreachProof{}, fmt.Errorf("hotbreach: marshal snapshot: %w", err)
	   }
	   rawLease, err := json.Marshal(lease)
	   if err != nil {
		   return BreachProof{}, fmt.Errorf("hotbreach: marshal lease: %w", err)
	   }
	   snapDigest := base64url(sha256sum(rawSnap))
	  leaseDigest := base64url(sha256sum(rawLease))
	   cpHash := bc.ConcurrentPartyHash
	  if cpHash == "" {
		  cpHash = hashParty(bc.ConcurrentParty)
	  }
	   compositeRaw := snapDigest + "|" + leaseDigest + "|" + cpHash
	  compositeHash := base64url(sha256sum([]byte(compositeRaw)))
	  return BreachProof{
		   ID: uuid.NewString(),
		   BreachID: bc.ID,
		   Asset:          bc.Asset,
		   SnapshotDigest: snapDigest,
		   LeaseDigest:    leaseDigest,
		  ConcurrentPartyHash: cpHash,
		  CompositeHash:       compositeHash,
		  RawSnapshotJSON:     string(rawSnap),
		  RawLeaseJSON:        string(rawLease),
		  CreatedAt: now,
	  }, nil
   }

// VerifyProof 验证证明的 CompositeHash 是否与原始数据一致。
// 返回 nil 表示验证通过，否则为校验失败原因。
   func VerifyProof(proof BreachProof) error {
	  if proof.SnapshotDigest == "" || proof.LeaseDigest == "" || proof.ConcurrentPartyHash == "" {
		   return fmt.Errorf("hotbreach: proof missing required fields")
	   }
	   // 重建 CompositeHash
	   compositeRaw := proof.SnapshotDigest + "|" + proof.LeaseDigest + "|" + proof.ConcurrentPartyHash
	  expected := base64url(sha256sum([]byte(compositeRaw)))
	   if expected != proof.CompositeHash {
		   return fmt.Errorf("hotbreach: proof composite hash mismatch: expected %s, got %s", expected, proof.CompositeHash)
	  }
	   // 验证 SnapshotDigest 与原始 JSON 一致
	  if proof.RawSnapshotJSON != "" {
		   actual := base64url(sha256sum([]byte(proof.RawSnapshotJSON)))
		  if actual != proof.SnapshotDigest {
			   return fmt.Errorf("hotbreach: proof snapshot digest mismatch")
		   }
	   }
	  // 验证 LeaseDigest 与原始 JSON 一致
	   if proof.RawLeaseJSON != "" {
		  actual := base64url(sha256sum([]byte(proof.RawLeaseJSON)))
		  if actual != proof.LeaseDigest {
			  return fmt.Errorf("hotbreach: proof lease digest mismatch")
		   }
	  }
	   return nil
  }

// hashParty 计算并发方标识的 SHA-256 hex。

   func hashParty(party string) string {
	  sum := sha256.Sum256([]byte(strings.TrimSpace(party)))
	  return hex.EncodeToString(sum[:])
   }

// sha256sum 计算 SHA-256 摘要。
   func sha256sum(data []byte) []byte {
	  sum := sha256.Sum256(data)
	  return sum[:]
   }

// base64url 将字节编码为 base64url（无填充）。

   func base64url(data []byte) string {
	   return base64.RawURLEncoding.EncodeToString(data)
  }

// unmarshalFence 辅助：从字节反序列化 DrainFence（用于批量扫描）。
  func unmarshalFence(body []byte, f *DrainFence) error {
	  return json.Unmarshal(body, f)
  }

// generateID 生成唯一 ID（UUID v4）。

   func generateID() string {
	   return uuid.NewString()
   }
