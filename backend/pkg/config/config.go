// Package config is retained as an empty compatibility package.
//
// Runtime configuration is owned by caiyun/internal/bootstrap, where CoreConfig
// is loaded once and injected into dependency assembly. Keeping an independent
// environment loader here previously created two divergent configuration paths.
package config
