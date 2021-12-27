package subacct

// CASGuard 冻结 CAS 只读复验面。

type CASGuard interface {
	GuardDebit(ownerID,
		casID, asset, amount string) error
	GuardUnfreezeChild(childID string) error
	NoteDebitResult(ownerID, casID, asset, amount, idem string, ok bool, refuse string)
}

// BindCASGuard 挂接冻结 CAS 引擎。
func (s *Service) BindCASGuard(g CASGuard) *Service {
	if s != nil {
		s.cas = g
	}
	return s
}

func (s *Service) EnterCASInFlight() {
	if s != nil {
		s.casInFlight++
	}
}

func (s *Service) LeaveCASInFlight() {
	if s != nil && s.casInFlight > 0 {
		s.casInFlight--
	}
}

func (s *Service) guardDebit(owner, casID, asset, amount string) error {
	if s == nil || s.cas == nil || s.casInFlight > 0 {
		return nil
	}
	return s.cas.GuardDebit(owner, casID, asset, amount)
}

func (s *Service) noteDebit(owner, casID, asset, amount, idem string, ok bool, refuse string) {
	if s == nil || s.cas == nil || s.casInFlight > 0 {
		return
	}
	s.cas.NoteDebitResult(owner,
		casID, asset, amount, idem, ok, refuse)
}

func (s *Service) guardUnfreezeChild(childID string) error {
	if s == nil || s.cas == nil || s.casInFlight > 0 {
		return nil
	}
	return s.cas.GuardUnfreezeChild(childID)
}
