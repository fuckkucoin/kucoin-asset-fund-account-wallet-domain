package reconflow

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// Rule 分类规则。Match 返回 true 时命中，Score 贡献权重。
type Rule struct {
	Name string
	Weight float64
	Match func(f DiffFeature) bool
}

// ruleChain 规则链，按 Kind 分组。

type ruleChain struct {
	Kind  DiffClassKind
	Rules []Rule
	// MinScore 命中并超过此分时才给高/中置信；否则降为 low。
	MinScore float64
}

// ──────────────────────────────────────────────
// 已知规则链定义（三种稳定类型 + 扩展）
// ──────────────────────────────────────────────

var knownChains = []ruleChain{
	{
		Kind:     DiffClassBridgeInflate,
		MinScore: 0.6,
		Rules: []Rule{
			{
				Name: "source_kind_bridge_inflate",
				Weight: 0.5,
				Match: func(f DiffFeature) bool {
					   return strings.EqualFold(f.SourceKind, "bridge_inflate")
				  },
			},
			{
				Name: "book_is_bridge",
				Weight: 0.2,
				Match: func(f DiffFeature) bool {
					  return strings.EqualFold(f.Book, "bridge")
				  },
			},
			{
				Name:   "no_known_source",
				Weight: 0.15,
				Match: func(f DiffFeature) bool {
					  return !f.KnownSource
				  },
			},
			{
				Name:   "not_in_transit",
				Weight: 0.1,
				Match: func(f DiffFeature) bool {
					   return !f.InTransit
				   },
			},
			{
				Name: "severity_warning_or_critical",
				Weight: 0.05,
				Match: func(f DiffFeature) bool {
					   s := strings.ToLower(f.Severity)
					   return s == "warning" || s == "critical"
				  },
			},
		},
	},
	{
		Kind:     DiffClassHotBelowUser,
		MinScore: 0.55,
		Rules: []Rule{
			{
				Name:   "source_kind_hot_below_user",
				Weight: 0.55,
				Match: func(f DiffFeature) bool {
					   return strings.EqualFold(f.SourceKind, "hot_below_user")
				  },
			},
			{
				Name: "not_in_transit",
				Weight: 0.15,
				Match: func(f DiffFeature) bool {
					  return !f.InTransit
				  },
			},
			{
				Name:   "no_second_credit",
				Weight: 0.1,
				Match: func(f DiffFeature) bool {
					  return !f.SecondCredit
				   },
			},
			{
				Name: "asset_class_stable_or_major",
				Weight: 0.1,
				Match: func(f DiffFeature) bool {
					  c := strings.ToLower(f.AssetClass)
					  return c == "stable" || c == "major"
				   },
			},
			{
				Name: "severity_not_trivial",
				Weight: 0.1,
				Match: func(f DiffFeature) bool {
					  return !strings.EqualFold(f.Severity, "trivial")
				   },
			},
		},
	},
	{
		Kind:     DiffClassDepositShort,
		MinScore: 0.5,
		Rules: []Rule{
			{
				Name: "source_kind_deposit_diff",
				Weight: 0.4,
				Match: func(f DiffFeature) bool {
					   return strings.EqualFold(f.SourceKind, "deposit_diff")
				  },
			},
			{
				Name:   "has_tx_hash",
				Weight: 0.25,
				Match: func(f DiffFeature) bool {
					   return strings.TrimSpace(f.TxHash) != ""
				  },
			},
			{
				Name:   "not_second_credit",
				Weight: 0.15,
				Match: func(f DiffFeature) bool {
					   return !f.SecondCredit
				  },
			},
			{
				Name:   "known_source",
				Weight: 0.1,
				Match: func(f DiffFeature) bool {
					  return f.KnownSource
				   },
			},
			{
				Name: "not_in_transit",
				Weight: 0.1,
				Match: func(f DiffFeature) bool {
					   return !f.InTransit
				  },
			},
		},
	},
	{
		Kind: DiffClassSecondCredit,
		MinScore: 0.6,
		Rules: []Rule{
			{
				Name: "second_credit_flag",
				Weight: 0.6,
				Match: func(f DiffFeature) bool {
					   return f.SecondCredit
				  },
			},
			{
				Name: "has_tx_hash",
				Weight: 0.2,
				Match: func(f DiffFeature) bool {
					  return strings.TrimSpace(f.TxHash) != ""
				  },
			},
			{
				Name: "source_deposit_or_diff",
				Weight: 0.2,
				Match: func(f DiffFeature) bool {
					  sk := strings.ToLower(f.SourceKind)
					  return strings.Contains(sk, "deposit")
				  },
			},
		},
	},
	{
		Kind:     DiffClassInTransitDelay,
		MinScore: 0.5,
		Rules: []Rule{
			{
				Name: "in_transit_flag",
				Weight: 0.5,
				Match: func(f DiffFeature) bool {
					   return f.InTransit
				  },
			},
			{
				Name:   "drift_age_positive",
				Weight: 0.3,
				Match: func(f DiffFeature) bool {
					  return strings.TrimSpace(f.DriftAge) != "" && f.DriftAge != "0"
				   },
			},
			{
				Name:   "severity_trivial_or_warning",
				Weight: 0.2,
				Match: func(f DiffFeature) bool {
					   s := strings.ToLower(f.Severity)
					   return s == "trivial" || s == "warning"
				  },
			},
		},
	},
	{
		Kind:     DiffClassUnexplainedBook,
		MinScore: 0.5,
		Rules: []Rule{
			{
				Name:   "source_kind_unexplained",
				Weight: 0.5,
				Match: func(f DiffFeature) bool {
					  return strings.EqualFold(f.SourceKind, "unexplained_book")
				   },
			},
			{
				Name:   "no_known_source",
				Weight: 0.3,
				Match: func(f DiffFeature) bool {
					  return !f.KnownSource
				   },
			},
			{
				Name: "no_tx_hash",
				Weight: 0.2,
				Match: func(f DiffFeature) bool {
					  return strings.TrimSpace(f.TxHash) == ""
				   },
			},
		},
	},
}

// ──────────────────────────────────────────────
// Classifier 公共接口
// ──────────────────────────────────────────────
// Classifier 差异特征分类器。
type Classifier struct {
	chains []ruleChain
}

// NewClassifier 构造默认分类器（含全部内置规则链）。

   func NewClassifier() *Classifier {
	   return &Classifier{chains: knownChains}
  }

// Classify 对特征向量进行分类，返回最高置信度的分类结果。
// 若无任何规则链命中（总分 < 0.3），返回 DiffClassUnknown 且置信度 low。
   func (c *Classifier) Classify(f DiffFeature) DiffClass {
	  now := time.Now().UTC()
	   hash := featureHash(f)
	  type candidate struct {
		   kind DiffClassKind
		   score   float64
		   matched []string
	   }
	  best := candidate{kind: DiffClassUnknown, score: 0}
	   for _, chain := range c.chains {
		  score := 0.0
		   matched := make([]string, 0, len(chain.Rules))
		   for _, r := range chain.Rules {
			  if r.Match(f) {
				  score += r.Weight
				  matched = append(matched, r.Name)
			  }
		  }
		  if score > best.score {
			  best = candidate{kind: chain.Kind, score: score, matched: matched}
		  }
	   }
	  // 归一化：各链权重之和约为 1.0，直接用 score 当 confidence 分。
	   confidence := confidenceFromScore(best.score, best.kind)
	  // 建议动作：由 catalogSuggestions 按分类类型给出。
	  suggested := catalogSuggestions(best.kind, confidence)
	  note := fmt.Sprintf("分类=%s 置信=%.2f 命中规则=%d", best.kind, best.score, len(best.matched))
	   if best.kind == DiffClassUnknown {
		   note = "无规则链匹配，进人工复核"
	   }
	   return DiffClass{
		   ID:          uuid.NewString(),
		   FeatureHash: hash,
		  Kind:       best.kind,
		   Confidence: confidence,
		  Score:      best.score,
		  MatchedRules:  best.matched,
		   SuggestedActs: suggested,
		   Note: note,
		   ClassifiedAt: now,
	   }
   }

// ──────────────────────────────────────────────
// 辅助函数
// ──────────────────────────────────────────────
// confidenceFromScore 按分值和分类类型给出置信度等级。
// unknown 类型一律 low；其他类型按分值段定级。
   func confidenceFromScore(score float64, kind DiffClassKind) ConfidenceLevel {
	   if kind == DiffClassUnknown || score < 0.3 {
		   return ConfidenceLow
	  }
	   if score >= 0.75 {
		  return ConfidenceHigh
	  }
	  if score >= 0.5 {
		  return ConfidenceMedium
	   }
	  return ConfidenceLow
   }

// catalogSuggestions 按分类类型给出建议动作 ID（动作目录中已注册的名称）。
// 返回的是动作 Name（非 ID），Engine 负责 resolve。

   func catalogSuggestions(kind DiffClassKind, confidence ConfidenceLevel) []string {
	   switch kind {
	   case DiffClassBridgeInflate:
		   return []string{"open_ticket", "escalate_human"}
	  case DiffClassHotBelowUser:
		   if confidence == ConfidenceHigh {
			   return []string{"open_ticket", "request_circuit"}
		  }
		  return []string{"open_ticket", "escalate_human"}
	  case DiffClassDepositShort:
		   return []string{"open_ticket", "suggest_post"}
	  case DiffClassSecondCredit:
		   return []string{"open_ticket", "freeze_user"}
	  case DiffClassInTransitDelay:
		  return []string{"open_ticket", "notify"}
	  case DiffClassUnexplainedBook:
		   return []string{"open_ticket", "escalate_human"}
	  default:
		  return []string{"escalate_human"}
	  }
   }

// featureHash 特征摘要：对核心字段取 SHA256 前 16 字节。
// 用途：幂等判断——相同特征不重复开流水。

  func featureHash(f DiffFeature) string {
	   type key struct {
		   SK string `json:"sk"`
		  A string `json:"a"`
		  U string `json:"u"`
		  B string `json:"b"`
		   Tx  string `json:"tx"`
		  Obs string `json:"obs"`
		   Bk string `json:"bk"`
	  }
	   k := key{SK: f.SourceKind, A: f.Asset, U: f.UserID, B: f.Book, Tx: f.TxHash, Obs: f.ObservedAmt, Bk: f.BookAmt}
	   raw, _ := json.Marshal(k)
	   sum := sha256.Sum256(raw)
	   return hex.EncodeToString(sum[:8])
  }
