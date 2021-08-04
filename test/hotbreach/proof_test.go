package hotbreach_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/hotbreach"
)

// ─────────────────────────────────────────────
// 包内哈希辅助（等价于 hotbreach 包内私有函数）
// ─────────────────────────────────────────────

  func testHashParty(party string) string {
	   sum := sha256.Sum256([]byte(party))
	   return hex.EncodeToString(sum[:])
  }

   func testSHA256Base64(data []byte) string {
	  sum := sha256.Sum256(data)
	   return base64.RawURLEncoding.EncodeToString(sum[:])
   }

// rebuildProofDigests 根据 RawSnapshotJSON / RawLeaseJSON / ConcurrentPartyHash
// 重建 SnapshotDigest、LeaseDigest 和 CompositeHash，用于构造可验证的证明。
  func rebuildProofDigests(p hotbreach.BreachProof) hotbreach.BreachProof {
	   p.SnapshotDigest = testSHA256Base64([]byte(p.RawSnapshotJSON))
	  p.LeaseDigest = testSHA256Base64([]byte(p.RawLeaseJSON))
	  composite := p.SnapshotDigest + "|" + p.LeaseDigest + "|" + p.ConcurrentPartyHash
	  p.CompositeHash = testSHA256Base64([]byte(composite))
	   return p
   }

// ─────────────────────────────────────────────
// 用例
// ─────────────────────────────────────────────

   func TestVerifyProof_ValidFields(t *testing.T) {
	  proof := hotbreach.BreachProof{
		   ID:       "proof-test-001",
		   BreachID: "breach-test-001",
		   Asset:    "USDT",
		  RawSnapshotJSON: `{"asset":"USDT","hot_available":"1100","hot_max":"1000","over_cap":true}`,
		   RawLeaseJSON:        `{"id":"lease-001","asset":"USDT","state":"issued"}`,
		   ConcurrentPartyHash: testHashParty("concurrent-party-001"),
		   CreatedAt: time.Date(2026,
			  9, 21, 12, 0, 0, 0, time.UTC),
	  }
	   proof = rebuildProofDigests(proof)
	   if err := hotbreach.VerifyProof(proof); err != nil {
		  t.Fatalf("VerifyProof should pass for valid proof: %v", err)
	  }
  }

  func TestVerifyProof_TamperedSnapshot(t *testing.T) {
	  proof := hotbreach.BreachProof{
		   ID:       "proof-tamper-001",
		  BreachID: "breach-test-002",
		  Asset:           "BTC",
		  RawSnapshotJSON: `{"asset":"BTC","hot_available":"11","hot_max":"10","over_cap":true}`,
		  RawLeaseJSON:    `{"id":"lease-002","asset":"BTC","state":"issued"}`,
		  ConcurrentPartyHash: testHashParty("cp-002"),
	  }
	  proof = rebuildProofDigests(proof)
	   // 篡改 RawSnapshotJSON 但不更新 SnapshotDigest
	  proof.RawSnapshotJSON = `{"asset":"BTC","hot_available":"9","hot_max":"10","over_cap":false}`
	   if err := hotbreach.VerifyProof(proof); err == nil {
		  t.Fatal("VerifyProof should fail for tampered snapshot")
	  }
  }

  func TestVerifyProof_TamperedComposite(t *testing.T) {
	   proof := hotbreach.BreachProof{
		  ID:       "proof-composite-001",
		  BreachID: "breach-test-003",
		  Asset:    "ETH",
		   RawSnapshotJSON:     `{"asset":"ETH","hot_available":"55","hot_max":"50","over_cap":true}`,
		  RawLeaseJSON:        `{"id":"lease-003","asset":"ETH","state":"issued"}`,
		  ConcurrentPartyHash: testHashParty("cp-003"),
	  }
	   proof = rebuildProofDigests(proof)
	  // 篡改 CompositeHash
	  proof.CompositeHash = "tampered-hash-value"
	   if err := hotbreach.VerifyProof(proof); err == nil {
		   t.Fatal("VerifyProof should fail for tampered composite hash")
	   }
   }

  func TestVerifyProof_MissingFields(t *testing.T) {
	   proof := hotbreach.BreachProof{
		   ID: "proof-missing-001",
		  BreachID: "breach-test-004",
		  Asset:    "USDT",
		  // SnapshotDigest, LeaseDigest, ConcurrentPartyHash 均为空
	  }
	  if err := hotbreach.VerifyProof(proof); err == nil {
		   t.Fatal("VerifyProof should fail when required fields missing")
	   }
  }

  func TestVerifyProof_TamperedLease(t *testing.T) {
	   proof := hotbreach.BreachProof{
		  ID:              "proof-lease-tamper-001",
		  BreachID:        "breach-test-005",
		  Asset:           "USDT",
		  RawSnapshotJSON: `{"asset":"USDT","hot_available":"1050","hot_max":"1000","over_cap":true}`,
		   RawLeaseJSON:    `{"id":"lease-005","asset":"USDT","snapshot_hot":"950","state":"issued"}`,
		   ConcurrentPartyHash: testHashParty("cp-005"),
	  }
	   proof = rebuildProofDigests(proof)
	  // 篡改 RawLeaseJSON 不更新 LeaseDigest
	   proof.RawLeaseJSON = `{"id":"lease-005","asset":"USDT","snapshot_hot":"0","state":"issued"}`
	  if err := hotbreach.VerifyProof(proof); err == nil {
		  t.Fatal("VerifyProof should fail for tampered lease JSON")
	   }
  }

// ─────────────────────────────────────────────
// 表驱动：证明验证场景
// ─────────────────────────────────────────────

type proofVerifyCase struct {
	name string
	raw      hotbreach.BreachProof
	tamper   func(p hotbreach.BreachProof) hotbreach.BreachProof
	expectOK bool
}

  func TestProofVerify_Table(t *testing.T) {
	  base := hotbreach.BreachProof{
		  RawSnapshotJSON:     `{"asset":"USDT","hot_available":"1100","hot_max":"1000","excess":"100","over_cap":true}`,
		   RawLeaseJSON:        `{"id":"tbl-lease","asset":"USDT","generation":1,"snapshot_hot":"950"}`,
		  ConcurrentPartyHash: testHashParty("table-concurrent-party"),
	  }
	  cases := []proofVerifyCase{
		  {
			  name: "valid proof",
			  raw:  base,
			   tamper: func(p hotbreach.BreachProof) hotbreach.BreachProof { return p },
			   expectOK: true,
		   },
		   {
			   name: "tampered snapshot",
			  raw: base,
			   tamper: func(p hotbreach.BreachProof) hotbreach.BreachProof {
				   p.RawSnapshotJSON = `{"asset":"USDT","hot_available":"999","over_cap":false}`
				  return p
			  },
			  expectOK: false,
		   },
		  {
			   name: "tampered lease",
			  raw: base,
			   tamper: func(p hotbreach.BreachProof) hotbreach.BreachProof {
				   p.RawLeaseJSON = `{"id":"tbl-lease","asset":"USDT","generation":999}`
				   return p
			   },
			  expectOK: false,
		  },
		  {
			  name: "tampered composite",
			   raw: base,
			   tamper: func(p hotbreach.BreachProof) hotbreach.BreachProof {
				  p.CompositeHash = "000000000000"
				   return p
			  },
			   expectOK: false,
		  },
		  {
			   name: "empty composite party hash",
			  raw: hotbreach.BreachProof{
				   RawSnapshotJSON: base.RawSnapshotJSON,
				  RawLeaseJSON:    base.RawLeaseJSON,
				   // ConcurrentPartyHash 为空
			   },
			  tamper:   func(p hotbreach.BreachProof) hotbreach.BreachProof { return rebuildProofDigests(p) },
			   expectOK: false, // missing field check
		  },
	   }
	  for _, tc := range cases {
		  tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			   p := rebuildProofDigests(tc.raw)
			   p = tc.tamper(p)
			  err := hotbreach.VerifyProof(p)
			  if tc.expectOK && err != nil {
				   t.Fatalf("expected OK, got: %v", err)
			   }
			   if !tc.expectOK && err == nil {
				  t.Fatal("expected error, got nil")
			   }
		   })
	   }
   }
