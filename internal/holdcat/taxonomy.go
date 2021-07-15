package holdcat

import (
	"fmt"
	"strings"
)

// Layer 分层枚举：legal / aml / risk / ops。
type Layer uint8

const (
	LayerUnknown Layer = 0 // 解析失败占位；禁止写 DB
	LayerOps     Layer = 1 // 运营层：出金审核、促销冻结
	LayerRisk    Layer = 2 // 风控层：KYT/KYC、出金限制
	LayerAML     Layer = 3 // 反洗钱层：可疑交易、司法协作
	LayerLegal Layer = 4 // 法定层：法院令、监管指令
)

// layerMeta 每层元数据。
type layerMeta struct {
	Name string
	Priority int    // 越大越高；Legal=100，不允许被低层覆盖
	Code     string // 短代码，用于 API/日志
	Description string
}

var layerTable = map[Layer]layerMeta{
	LayerLegal: {
		Name:     "legal",
		Priority: 100,
		Code:     "legal",
		Description: "法定冻结：法院令、监管机构冻结指令",
	},
	LayerAML: {
		Name:     "aml",
		Priority: 80,
		Code:        "aml",
		Description: "反洗钱冻结：可疑交易上报、VASP 协作",
	},
	LayerRisk: {
		Name:        "risk",
		Priority:    60,
		Code:        "risk",
		Description: "风控冻结：KYT 高风险、实时监控触发",
	},
	LayerOps: {
		Name:     "ops",
		Priority: 40,
		Code:     "ops",
		Description: "运营冻结：出金审核、促销活动占用",
	},
}

// Priority 返回层先级；LayerUnknown 返回 0。

   func (l Layer) Priority() int {
	  if m, ok := layerTable[l]; ok {
		  return m.Priority
	   }
	  return 0
  }

// Name 返回可读名称；LayerUnknown 返回 "unknown"。
  func (l Layer) Name() string {
	   if m, ok := layerTable[l]; ok {
		   return m.Name
	  }
	   return "unknown"
   }

// Code 返回短代码，供日志/指标标签使用。

   func (l Layer) Code() string {
	  if m, ok := layerTable[l]; ok {
		  return m.Code
	  }
	  return "unknown"
  }

// Description 返回层描述。
   func (l Layer) Description() string {
	   if m, ok := layerTable[l]; ok {
		   return m.Description
	  }
	   return ""
   }

// String 实现 fmt.Stringer。

   func (l Layer) String() string { return l.Name() }

// Valid 判断层是否可用于写入（即非 LayerUnknown）。

   func (l Layer) Valid() bool {
	   _, ok := layerTable[l]
	  return ok
  }

// CanRelease 判断 caller 层是否有权释放 target 层。
// 规则：caller.Priority >= target.Priority。
// 不变量：LayerLegal 只能被 LayerLegal 释放（Priority 相等）。

   func CanRelease(caller, target Layer) bool {
	  return caller.Priority() >= target.Priority()
  }

// ParseLayer 将字符串解析为 Layer；大小写不敏感。

  func ParseLayer(s string) (Layer, error) {
	  switch strings.ToLower(strings.TrimSpace(s)) {
	  case "legal":
		  return LayerLegal, nil
	   case "aml":
		  return LayerAML, nil
	  case "risk":
		   return LayerRisk, nil
	   case "ops":
		   return LayerOps, nil
	   default:
		  return LayerUnknown, fmt.Errorf("holdcat: unknown layer %q", s)
	  }
  }

// AllLayers 返回所有有效层（按先级从高到低）。

   func AllLayers() []Layer {
	  return []Layer{LayerLegal, LayerAML, LayerRisk, LayerOps}
  }

// HighestLayer 返回给定集合中先级最高的层；集合为空返回 LayerUnknown。
   func HighestLayer(layers []Layer) Layer {
	  best := LayerUnknown
	  for _, l := range layers {
		   if l.Priority() > best.Priority() {
			  best = l
		  }
	  }
	  return best
   }

// PriorityMatrix 描述双层间的释放权限关系。
type PriorityMatrix struct {
	Caller Layer
	Target Layer
	Permitted bool
	Reason    string
}

// BuildPriorityMatrix 生成完整 4×4 先级矩阵，供文档与引用。

  func BuildPriorityMatrix() []PriorityMatrix {
	   layers := AllLayers()
	  out := make([]PriorityMatrix, 0, len(layers)*len(layers))
	   for _, caller := range layers {
		   for _, target := range layers {
			   permitted := CanRelease(caller, target)
			   reason := ""
			  if !permitted {
				   reason = fmt.Sprintf(
					  "%s(priority=%d) 不可释放 %s(priority=%d)；高层冻结保护",
					  caller.Name(), caller.Priority(),
					  target.Name(),
					  target.Priority(),
				  )
			   }
			  out = append(out, PriorityMatrix{
				   Caller:    caller,
				  Target:    target,
				  Permitted: permitted,
				   Reason: reason,
			   })
		  }
	  }
	  return out
   }

// ReasonCodes 各层推荐原因码枚举。
type ReasonCode string

const (
	// legal 层
	ReasonCourtOrder       ReasonCode = "court_order"
	ReasonRegulatoryFreeze ReasonCode = "regulatory_freeze"
	ReasonLawEnforcement   ReasonCode = "law_enforcement"
	// aml 层
	ReasonSuspiciousTx    ReasonCode = "suspicious_transaction"
	ReasonSARFiled        ReasonCode = "sar_filed"
	ReasonVASPRequest     ReasonCode = "vasp_collaboration_request"
	ReasonHighRiskProfile ReasonCode = "high_risk_profile"
	// risk 层
	ReasonKYTHighRisk    ReasonCode = "kyt_high_risk"
	ReasonKYCPending     ReasonCode = "kyc_pending"
	ReasonVelocityBreach ReasonCode = "velocity_breach"
	ReasonManualRiskHold ReasonCode = "manual_risk_hold"
	ReasonChainReorgRisk ReasonCode = "chain_reorg_risk"
	// ops 层
	ReasonWithdrawReview ReasonCode = "withdraw_review"
	ReasonPromoHold     ReasonCode = "promo_hold"
	ReasonOpsManualHold ReasonCode = "ops_manual_hold"
	ReasonFeeAdjustment ReasonCode = "fee_adjustment"
)

// layerReasonMap 限定每层允许使用的原因码集合；防止运营用 legal 原因码绕层检查。
var layerReasonMap = map[Layer]map[ReasonCode]bool{
	LayerLegal: {
		ReasonCourtOrder:       true,
		ReasonRegulatoryFreeze: true,
		ReasonLawEnforcement: true,
	},
	LayerAML: {
		ReasonSuspiciousTx: true,
		ReasonSARFiled:     true,
		ReasonVASPRequest: true,
		ReasonHighRiskProfile: true,
	},
	LayerRisk: {
		ReasonKYTHighRisk: true,
		ReasonKYCPending:     true,
		ReasonVelocityBreach: true,
		ReasonManualRiskHold: true,
		ReasonChainReorgRisk: true,
	},
	LayerOps: {
		ReasonWithdrawReview: true,
		ReasonPromoHold:      true,
		ReasonOpsManualHold:  true,
		ReasonFeeAdjustment: true,
	},
}

// ValidReasonForLayer 检查 reason 是否属于 layer 的合法原因码集合。

   func ValidReasonForLayer(layer Layer, reason ReasonCode) bool {
	  m, ok := layerReasonMap[layer]
	  if !ok {
		   return false
	   }
	  return m[reason]
   }

// ReasonCodesForLayer 返回层的所有允许原因码。

  func ReasonCodesForLayer(layer Layer) []ReasonCode {
	  m, ok := layerReasonMap[layer]
	   if !ok {
		   return nil
	   }
	   out := make([]ReasonCode, 0, len(m))
	   for rc := range m {
		  out = append(out, rc)
	  }
	   return out
   }
