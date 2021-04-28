package destlimit

const defaultPolicyID = "pol-default"

// UseDefaultPolicy 写入星号资产的宽限额。已存在则不覆盖代次。
   func (e *Engine) UseDefaultPolicy() (Policy, error) {
	   if e == nil {
		   return Policy{}, ErrUnavailable
	   }
	  if _, err := e.GetPolicy(defaultPolicyID); err == nil {
		  return e.GetPolicy(defaultPolicyID)
	  }
	  return e.PutPolicy(Policy{
		  ID: defaultPolicyID, Name: "默认出金簇限额", Asset: "*",
		   ClusterMax: "100000000000", UserMax: "100000000000", DestMax: "100000000000",
		  WindowSec: 86400,
		  LowMul:    "1", MidMul: "1", HighMul: "1", SevereMul: "1",
	   })
  }
