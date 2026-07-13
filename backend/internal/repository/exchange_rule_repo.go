package repository

import "gorm.io/gorm"

// ExchangeRuleRepository 是 ExchangeAccountRepository 的语义化别名。
// 底层物理表已统一为 exchange_rules；Repository 名称保留 ExchangeAccountRepository 作为兼容层。
type ExchangeRuleRepository = ExchangeAccountRepository

func NewExchangeRuleRepository(db *gorm.DB) *ExchangeRuleRepository {
	return NewExchangeAccountRepository(db)
}
