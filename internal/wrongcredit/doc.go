// @FileName: doc.go
// @Summary: 错贷记发现与追回冲销编排引擎。
// - 只处理已 credited 的错误贷记单据（WrongCreditCase），不干预 r5 科目树、
// r2 预贷记归因，也不改写 r1 入金 conserve 公式或账本 Apply/fence。
// - 与 reorg-reversal 批次互斥：同一 credit_ref（transfer_id）不能同时在
// reorg 路径和 wrong-credit 路径做冲销（CreditMutex 强制）。
// - 本包不 clone depclaim ticket，也不共享其 FSM；WrongCredit 是独立域。
// 不变量：
// - WrongCreditCase.ID 全局唯一，IdempotencyKey 对单据幂等。
// - 同一 CreditRef 至多一个 OPEN 状态的 WrongCreditCase（重复开案拒绝）。
// - 追回金额不超过原始贷记金额（PartialClaw 拆解后 Remainder → bad_debt）。
// - 冲销分录满足 ValidateJournal（借贷各资产轧平）。
// @Author: Kinsley
// @Created: 2021.12.29

package wrongcredit
