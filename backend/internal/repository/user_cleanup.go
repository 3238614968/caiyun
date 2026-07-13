package repository

import (
	"fmt"

	"caiyun/internal/models"
)

// DeleteByUserID permanently removes exchange records because they have no
// soft-delete marker and may contain upstream response text.
func (r *ExchangeRecordRepository) DeleteByUserID(userID uint) error {
	return r.db.Where("user_id = ?", userID).Delete(&models.ExchangeRecord{}).Error
}

// DeleteByUserID permanently removes exchange tasks during user erasure. The
// enclosing Unit of Work deletes records first, so foreign-key ordering remains
// deterministic.
func (r *ExchangeTaskRepository) DeleteByUserID(userID uint) error {
	return r.db.Unscoped().Where("user_id = ?", userID).Delete(&models.ExchangeTask{}).Error
}

// DeleteByUserID clears credential copies before permanently removing rules.
func (r *ExchangeAccountRepository) DeleteByUserID(userID uint) error {
	if err := r.db.Model(&models.ExchangeAccount{}).
		Where("user_id = ?", userID).
		Updates(map[string]interface{}{
			"phone":     "",
			"auth":      "",
			"token":     "",
			"jwt_token": "",
			"remark":    "",
			"is_active": false,
		}).Error; err != nil {
		return err
	}
	return r.db.Unscoped().Where("user_id = ?", userID).Delete(&models.ExchangeAccount{}).Error
}

func (r *TaskLogRepository) DeleteByUserID(userID uint) error {
	return r.db.Unscoped().Where("user_id = ?", userID).Delete(&models.TaskLog{}).Error
}

func (r *CloudStatsRepository) DeleteByUserID(userID uint) error {
	return r.db.Unscoped().Where("user_id = ?", userID).Delete(&models.CloudStats{}).Error
}

// DeleteByUserID clears cloud credentials with per-row unique tombstone phone
// values before physical deletion, avoiding conflicts with the user/phone index.
func (r *AccountRepository) DeleteByUserID(userID uint) error {
	var accountIDs []uint
	if err := r.db.Model(&models.Account{}).Where("user_id = ?", userID).Pluck("id", &accountIDs).Error; err != nil {
		return err
	}
	for _, id := range accountIDs {
		if err := r.db.Model(&models.Account{}).
			Where("id = ? AND user_id = ?", id, userID).
			Updates(map[string]interface{}{
				"phone":     fmt.Sprintf("deleted-%d", id),
				"auth":      "",
				"token":     "",
				"jwt_token": "",
				"remark":    "",
				"is_active": false,
			}).Error; err != nil {
			return err
		}
	}
	return r.db.Unscoped().Where("user_id = ?", userID).Delete(&models.Account{}).Error
}

func (r *WSMessageRepository) DeleteByUserID(userID uint) error {
	return r.db.Where("user_id = ?", userID).Delete(&models.WebSocketMessage{}).Error
}

// AnonymizeByUserID retains non-personal audit evidence while clearing direct
// identifiers and potentially sensitive captured payloads.
func (r *AuditLogRepository) AnonymizeByUserID(userID uint) error {
	return r.db.Model(&models.AuditLog{}).
		Where("user_id = ?", userID).
		Updates(map[string]interface{}{
			"username":      "deleted-user",
			"ip":            "",
			"user_agent":    "",
			"request_data":  "",
			"response_data": "",
			"error_msg":     "",
		}).Error
}

// DeleteByUserID removes durable asynchronous command payloads owned by a
// deleted user. Payloads can contain account identifiers and must not outlive
// the user-erasure transaction.
func (r *OperationRepository) DeleteByUserID(userID uint) error {
	return r.db.Where("user_id = ?", userID).Delete(&models.Operation{}).Error
}

// DeleteByAccountID removes commands directly associated with a deleted cloud
// account. Batch commands have account_id=0 and remain attached to the user.
func (r *OperationRepository) DeleteByAccountID(accountID uint) error {
	return r.db.Where("account_id = ?", accountID).Delete(&models.Operation{}).Error
}

// DeleteByUserID physically removes refresh-session hashes. Clear replacement
// links first so this remains deterministic even on databases that enforce the
// self-referential foreign key without ON DELETE SET NULL.
func (r *RefreshSessionRepository) DeleteByUserID(userID uint) error {
	if err := r.db.Model(&models.RefreshSession{}).
		Where("user_id = ?", userID).
		Update("replaced_by_session_id", nil).Error; err != nil {
		return err
	}
	return r.db.Where("user_id = ?", userID).Delete(&models.RefreshSession{}).Error
}

// DeleteByAccountID deletes exchange records whose rule belongs to the cloud
// account. It must run before rules are deleted.
func (r *ExchangeRecordRepository) DeleteByAccountID(accountID uint) error {
	ruleIDs := r.db.Model(&models.ExchangeAccount{}).Select("id").Where("account_id = ?", accountID)
	return r.db.Where("exchange_rule_id IN (?)", ruleIDs).Delete(&models.ExchangeRecord{}).Error
}

// DeleteByAccountID physically deletes exchange tasks whose rule belongs to
// the cloud account. It must run before rules are deleted.
func (r *ExchangeTaskRepository) DeleteByAccountID(accountID uint) error {
	ruleIDs := r.db.Model(&models.ExchangeAccount{}).Select("id").Where("account_id = ?", accountID)
	return r.db.Unscoped().Where("exchange_rule_id IN (?)", ruleIDs).Delete(&models.ExchangeTask{}).Error
}

// DeleteByAccountID clears copied credentials before deleting exchange rules
// associated with the cloud account.
func (r *ExchangeAccountRepository) DeleteByAccountID(accountID uint) error {
	if err := r.db.Model(&models.ExchangeAccount{}).
		Where("account_id = ?", accountID).
		Updates(map[string]interface{}{
			"phone":     "",
			"auth":      "",
			"token":     "",
			"jwt_token": "",
			"remark":    "",
			"is_active": false,
		}).Error; err != nil {
		return err
	}
	return r.db.Unscoped().Where("account_id = ?", accountID).Delete(&models.ExchangeAccount{}).Error
}

// DeleteByAccountID permanently removes statistics during account erasure.
func (r *CloudStatsRepository) DeleteByAccountID(accountID uint) error {
	return r.db.Unscoped().Where("account_id = ?", accountID).Delete(&models.CloudStats{}).Error
}

// AnonymizeAndDeleteByID overwrites credentials and unique identity data before
// physically deleting one cloud account.
func (r *AccountRepository) AnonymizeAndDeleteByID(accountID uint) error {
	if err := r.db.Model(&models.Account{}).
		Where("id = ?", accountID).
		Updates(map[string]interface{}{
			"phone":           fmt.Sprintf("deleted-%d", accountID),
			"auth":            "",
			"token":           "",
			"jwt_token":       "",
			"remark":          "",
			"is_active":       false,
			"jwt_error_count": 0,
		}).Error; err != nil {
		return err
	}
	return r.db.Unscoped().Delete(&models.Account{}, accountID).Error
}
