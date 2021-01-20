package subacct

import (
	"account-assets.kucoin.net/funding/internal/validate"
	"fmt"
	"strings"
)

var (
	ErrGateTransfer = fmt.Errorf("subacct transfer gated")
	ErrGateFrozen   = fmt.Errorf("subacct parent or child frozen")
	ErrGatePriv     = fmt.Errorf("subacct transfer privilege missing")
)

// GateTransferUsers 按用户 ID 的母子划转前置闸。
func (s *Service) GateTransferUsers(fromUser, toUser string, debitFromChild bool) error {
	if s == nil {
		return ErrUnavailable
	}
	if _, err := validate.UserID(fromUser); err != nil {
		return ErrUser
	}
	if _, err := validate.UserID(toUser); err != nil {
		return ErrUser
	}
	if strings.EqualFold(strings.TrimSpace(fromUser), strings.TrimSpace(toUser)) {
		return ErrSelf
	}
	kind, id, err := s.OwnerOf(fromUser)
	if err != nil {
		return err
	}
	_, toID, err := s.OwnerOf(toUser)
	if err != nil {
		return err
	}
	return s.gateOwnersLocked(kind, id, toID, debitFromChild)
}

// GateFlexType 公开 Flex 类型转闸（owner ID）。
// PARENT_TO_SUB / SUB_TO_PARENT；子出须 PrivTransfer。
// 额度与速度仍由 quota/velocity 约束。
func (s *Service) GateFlexType(parentID, subID, flexType string) error {
	if s == nil {
		return ErrUnavailable
	}
	ft := strings.ToUpper(strings.TrimSpace(flexType))
	parentID = strings.TrimSpace(parentID)
	subID = strings.TrimSpace(subID)
	switch ft {
	case "PARENT_TO_SUB":
		return s.gateOwners("parent", parentID, subID, false)
	case "SUB_TO_PARENT":
		return s.gateOwners("child", subID, parentID, true)
	default:
		return fmt.Errorf("%w: flex type", ErrGateTransfer)
	}
}

func (s *Service) gateOwners(fromKind, fromID, toID string, debitFromChild bool) error {
	if err := s.ensureHydrated(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gateOwnersLocked(fromKind, fromID, toID, debitFromChild)
}

func (s *Service) gateOwnersLocked(fromKind, fromID, toID string, debitFromChild bool) error {
	if err := s.requireActiveLocked(fromID, true); err != nil {
		return fmt.Errorf("%w: %v", ErrGateFrozen, err)
	}
	if fromKind == "child" && debitFromChild {
		if err := s.requirePrivLocked(fromID, PrivTransfer); err != nil {
			return fmt.Errorf("%w: %v", ErrGatePriv, err)
		}
	}
	if err := s.requireActiveLocked(toID, false); err != nil {
		return fmt.Errorf("%w: %v", ErrGateFrozen, err)
	}
	return nil
}
