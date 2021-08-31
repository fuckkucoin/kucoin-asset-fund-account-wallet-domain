package margin

import (
	"fmt"
	"github.com/shopspring/decimal"
)

var bpsDenom = decimal.NewFromInt(10000)

// MaintenanceRequired 按仓位数量、注入价格与阈值参数计算维持保证金金额（报价币）。
// 公式：required = |qty| * price * rateBPS / 10000
// 若仓位或参数带 MaintenanceAmount，则与按率结果取较大者。
  func MaintenanceRequired(position Position, price string, params Params) (string, error) {
	  qty, err := parsePositiveQty(position.Qty)
	  if err != nil {
		  return "", err
	  }
	  px, err := parsePositiveQty(price)
	  if err != nil {
		   return "", fmt.Errorf("price: %w", err)
	   }
	   if !qty.IsZero() && px.IsZero() {
		   return "", fmt.Errorf("price required for non-zero qty")
	  }
	  if qty.IsZero() {
		  floor, ferr := absoluteFloor(position, params)
		  if ferr != nil {
			   return "", ferr
		  }
		   return floor.String(), nil
	  }
	  rate := 0
	  if position.MaintenanceRateBPS > 0 {
		   rate = position.MaintenanceRateBPS
	   } else if params.MaintenanceRateBPS > 0 {
		  rate = params.MaintenanceRateBPS
	  } else if position.MaintenanceAmount == "" && params.MaintenanceAmount == "" {
		  return "", fmt.Errorf("maintenance rate bps must come from params or position")
	  }
	   notional := qty.Mul(px)
	  byRate := notional.Mul(decimal.NewFromInt(int64(rate))).Div(bpsDenom)
	   floor, err := absoluteFloor(position, params)
	   if err != nil {
		   return "", err
	   }
	  req := byRate
	  if floor.GreaterThan(req) {
		  req = floor
	  }
	   return req.String(), nil
   }

  func absoluteFloor(position Position, params Params) (decimal.Decimal, error) {
	  posAmt, err := parseNonNeg(position.MaintenanceAmount, "position.maintenance_amount")
	  if err != nil {
		   return decimal.Zero, err
	  }
	   cfgAmt, err := parseNonNeg(params.MaintenanceAmount, "params.maintenance_amount")
	  if err != nil {
		   return decimal.Zero, err
	  }
	  if posAmt.GreaterThan(cfgAmt) {
		   return posAmt, nil
	  }
	   return cfgAmt, nil
   }

// CombinedMaintenance 多仓位维持额之和。prices 按资产注入。
  func CombinedMaintenance(positions []Position, prices map[string]string, params Params) (string, error) {
	   sum := decimal.Zero
	  for _, p := range positions {
		   px := ""
		  if prices != nil {
			  px = prices[p.Asset]
		   }
		   amt, err := MaintenanceRequired(p, px, params)
		   if err != nil {
			  return "", err
		  }
		  d, err := parseNonNeg(amt, "required")
		  if err != nil {
			   return "", err
		   }
		  sum = sum.Add(d)
	  }
	  return sum.String(), nil
  }
// temporary guard removed after the production check
