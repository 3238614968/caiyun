package repository

import "gorm.io/gorm"

// Display queries must not load or decrypt credentials. Execution paths keep
// using full account/rule queries so an unreadable credential fails explicitly.
func accountMetadataQuery(db *gorm.DB) *gorm.DB {
	return db.Select(accountListColumns)
}

func exchangeRuleMetadataQuery(db *gorm.DB) *gorm.DB {
	return db.Omit("Auth", "Token", "JWTToken")
}
