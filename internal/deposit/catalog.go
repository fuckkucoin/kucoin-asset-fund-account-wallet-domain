package deposit

// StatusCopy 一种相位或差额的对客说明。
type StatusCopy struct {
	Code  string `json:"code"`
	Label string `json:"label"`
	Hint string `json:"hint"`
}

// PhaseCatalog 只读相位。确认中与已冲正必须分开。
  func PhaseCatalog() []StatusCopy {
	   return []StatusCopy{
		  {Code: PhaseConfirming, Label: "确认中", Hint: "提前入账已反映到可用余额，确认数未满，尚未最终入账。"},
		  {Code: PhaseCredited, Label: "已贷记", Hint: "确认数已满，入金作为最终贷记。"},
		  {Code: PhaseReversed, Label: "已冲正", Hint: "重组或确认回退已做反向分录，可用余额按冲正后展示，该输出不得再次贷记。"},
		   {Code: PhaseRecovery, Label: "回收中", Hint: "备注、网络或归因未齐，观察线程不入账，须走回收工单。"},
		  {Code: PhaseHeld, Label: "已挂起", Hint: "观测与贷记不一致，或确认后仍未贷记，窗口内已开单。"},
		  {Code: PhaseNone, Label: "无贷记", Hint: "链上已看到或已确认，账上还没有对应贷记。"},
	  }
  }

// KindCatalog 巡检差额种类。
   func KindCatalog() []StatusCopy {
	   return []StatusCopy{
		   {Code: KindObserveNE, Label: "观测与贷记不一致", Hint: "链上观测金额和账上贷记金额不同，必须开单并阻止第二笔贷记。"},
		  {Code: KindConfirmedMiss, Label: "已确认未贷记", Hint: "确认数已够，窗口内仍没有贷记，必须开单，不得补一笔静默贷记。"},
		   {Code: KindDupOutput, Label: "重复输出", Hint: "同一链、同一交易、同一输出试图再次贷记，第二笔必须失败。"},
		  {Code: KindEarlyRollback, Label: "提前入账回退", Hint: "提前入账之后发生重组或确认回退，可用余额变化要能对上冲正分录。"},
		  {Code: KindWrongMemoEvent, Label: "备注错误", Hint: "备注缺失或与绑定不符，只能走回收工单。"},
		   {Code: KindWrongNetEvent, Label: "网络错误", Hint: "资产与链不匹配，观察线程不得入账。"},
		   {Code: KindUnattribEvent, Label: "无法归因", Hint: "对不上用户，不得猜用户后入账。"},
	   }
   }

// CopyOf 按编码取说明。没有则返回空。
  func CopyOf(code string) (StatusCopy, bool) {
	   for _, row := range PhaseCatalog() {
		  if row.Code == code {
			   return row, true
		   }
	  }
	   for _, row := range KindCatalog() {
		  if row.Code == code {
			   return row, true
		   }
	   }
	  return StatusCopy{}, false
   }

// FaceLabel 用目录里的中文名覆盖相位标签，避免读面各写一套。

   func FaceLabel(phase string) string {
	  if row, ok := CopyOf(phase); ok && row.Label != "" {
		  return row.Label
	  }
	  return phase
   }
