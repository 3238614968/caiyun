import importlib.util
import sys
import tempfile
import unittest
import zipfile
from pathlib import Path
from unittest import mock

MODULE_PATH = Path(__file__).resolve().parents[1] / "migrate_legacy_db.py"
SPEC = importlib.util.spec_from_file_location("migrate_legacy_db", MODULE_PATH)
module = importlib.util.module_from_spec(SPEC)
assert SPEC and SPEC.loader
sys.modules[SPEC.name] = module
SPEC.loader.exec_module(module)


def col(name, nullable=False, default_is_null=True, extra="", length=None):
    return module.Column(name, nullable, default_is_null, "", extra, length)


class LegacyMigrationTests(unittest.TestCase):
    def test_embedded_v1_key_is_valid_and_selected_for_post_migration(self):
        with mock.patch.dict(
            "os.environ",
            {"DATA_ENCRYPTION_KEYS": "v2=invalid", "DATA_ENCRYPTION_CURRENT_VERSION": "v2"},
        ):
            keys, version = module.load_configured_encryption_keys()
        self.assertEqual(version, "v1")
        self.assertEqual(set(keys), {"v1"})
        self.assertEqual(len(keys["v1"]), 32)

    def test_encryption_key_formats_match_go_configuration(self):
        raw = "0123456789abcdef0123456789abcdef"
        self.assertEqual(module.decode_encryption_key(raw), raw.encode())
        encoded = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
        self.assertEqual(module.decode_encryption_key(encoded), raw.encode())
        self.assertEqual(module.decode_encryption_key(raw.encode().hex()), raw.encode())

    def test_credential_encryption_round_trip_matches_versioned_format(self):
        key = b"0123456789abcdef0123456789abcdef"
        encrypted = module.encrypt_credential_value("secret-token", "v2", key)
        self.assertTrue(encrypted.startswith("enc:v2:"))
        plaintext, version = module.decrypt_credential_value(
            encrypted, {"v2": key}
        )
        self.assertEqual(plaintext, "secret-token")
        self.assertEqual(version, "v2")

    def test_plaintext_and_legacy_credentials_rotate_to_current_version(self):
        key1 = b"0123456789abcdef0123456789abcdef"
        key2 = b"abcdef0123456789abcdef0123456789"
        keys = {"v1": key1, "v2": key2}

        rewritten, changed, state = module.rotate_credential_value(
            "plain-auth", "v2", keys
        )
        self.assertTrue(changed)
        self.assertEqual(state, "plaintext")
        self.assertTrue(rewritten.startswith("enc:v2:"))

        legacy = module.encrypt_credential_value("legacy-token", "v1", key1)
        rewritten, changed, state = module.rotate_credential_value(
            legacy, "v2", keys
        )
        self.assertTrue(changed)
        self.assertEqual(state, "legacy")
        self.assertEqual(
            module.decrypt_credential_value(rewritten, keys),
            ("legacy-token", "v2"),
        )

    def test_current_credential_is_verified_but_not_rewritten(self):
        key = b"0123456789abcdef0123456789abcdef"
        encrypted = module.encrypt_credential_value("jwt", "v1", key)
        rewritten, changed, state = module.rotate_credential_value(
            encrypted, "v1", {"v1": key}
        )
        self.assertFalse(changed)
        self.assertEqual(state, "current")
        self.assertEqual(rewritten, encrypted)

    def test_extended_insert_counter_handles_strings(self):
        sql = "INSERT INTO `users` VALUES (1,'a;()'),(2,'it\\'s ok');"
        info = module.inspect_sql_text(
            "CREATE TABLE `users` (`id` int);\n" + sql,
            Path("legacy.sql"), "legacy.sql", "a" * 64,
        )
        self.assertEqual(info.estimated_rows["users"], 2)

    def test_archive_rejects_database_level_statements(self):
        with self.assertRaises(module.MigrationError):
            module.inspect_sql_text(
                "USE `production`;\nCREATE TABLE `users` (`id` int);",
                Path("legacy.sql"), "legacy.sql", "b" * 64,
            )

    def test_zip_path_traversal_is_rejected(self):
        with tempfile.TemporaryDirectory() as temp:
            archive = Path(temp) / "legacy.zip"
            with zipfile.ZipFile(archive, "w") as bundle:
                bundle.writestr("../legacy.sql", "SELECT 1;")
            with self.assertRaises(module.MigrationError):
                module.extract_and_inspect_archive(archive, Path(temp))

    def test_prefixed_staging_sql_removes_foreign_keys(self):
        sql = """CREATE TABLE `accounts` (
  `id` bigint NOT NULL,
  KEY `idx_id` (`id`),
  CONSTRAINT `fk_accounts_user_id` FOREIGN KEY (`id`) REFERENCES `users` (`id`)
) ENGINE=InnoDB;
"""
        rewritten = module.rewrite_source_table_names(
            sql, "_legacy_", ("accounts", "users")
        )
        self.assertIn("CREATE TABLE `_legacy_accounts`", rewritten)
        self.assertNotIn("CONSTRAINT", rewritten)
        self.assertNotIn("FOREIGN KEY", rewritten)
        self.assertNotIn(",\n) ENGINE", rewritten)


    def test_exchange_rule_alias_and_offsets_are_generated(self):
        source = [col("id", extra="auto_increment"), col("exchange_account_id"), col("user_id")]
        target = [
            col("id", extra="auto_increment"), col("exchange_account_id", nullable=True),
            col("exchange_rule_id"), col("user_id"),
            col("active_dedupe_key", nullable=True, extra="STORED GENERATED"),
        ]

        def columns(_client, db, _table):
            return source if db == "stage" else target

        offsets = {"exchange_tasks": 100, "exchange_rules": 20, "users": 10}
        with mock.patch.object(module, "query_columns", side_effect=columns):
            sql = module.validate_and_build_insert(
                object(), "stage", "target", "exchange_tasks", "exchange_tasks",
                offsets, "abcdef123456", "_legacy_users", "_legacy_accounts",
            )
        self.assertIn("s.`exchange_account_id` + 20", sql)
        self.assertIn("`exchange_rule_id`", sql)
        self.assertIn("s.`user_id` + 10", sql)
        self.assertNotIn("active_dedupe_key", sql)

    def test_accounts_keep_target_row_and_merge_source_duplicates(self):
        where_sql = module.account_insert_filter(
            "caiyun", "caiyun", "_legacy_accounts", 10,
        )
        self.assertIn("s.id = (SELECT ca.id", where_sql)
        self.assertIn("ca.user_id = s.user_id AND ca.phone = s.phone", where_sql)
        self.assertIn("(ca.deleted_at IS NULL) DESC", where_sql)
        self.assertIn("ca.is_active DESC", where_sql)
        self.assertIn("NOT EXISTS", where_sql)
        self.assertIn("ta.user_id = s.user_id + 10", where_sql)
        self.assertIn("ta.phone = s.phone", where_sql)

    def test_account_references_resolve_to_retained_account(self):
        expression = module.account_reference_expr(
            "caiyun", "caiyun", "_legacy_accounts", "s.`account_id`",
            account_offset=100, user_offset=10,
        )
        self.assertIn("COALESCE", expression)
        self.assertIn("JOIN `caiyun`.accounts ta", expression)
        self.assertIn("ta.user_id = original.user_id + 10", expression)
        self.assertIn("ca.id + 100", expression)
        self.assertIn("ca.user_id = original.user_id", expression)
        self.assertIn("ca.phone = original.phone", expression)

    def test_cloud_stats_keep_target_and_collapse_account_remap_duplicates(self):
        offsets = {"accounts": 100, "users": 10}
        where_sql = module.cloud_stat_insert_filter(
            "caiyun", "caiyun", "_legacy_cloud_stats",
            "_legacy_accounts", offsets,
        )
        self.assertIn("SELECT MIN(candidate.id)", where_sql)
        self.assertIn("candidate.user_id = s.user_id", where_sql)
        self.assertIn("candidate.date = s.date", where_sql)
        self.assertIn("NOT EXISTS", where_sql)
        self.assertIn("`caiyun`.cloud_stats existing", where_sql)
        self.assertIn("existing.user_id = s.user_id + 10", where_sql)
        self.assertIn("existing.account_id", where_sql)
        self.assertIn("existing.date = s.date", where_sql)

    def test_exchange_task_active_key_matches_schedule_dedupe_migration(self):
        source_names = {
            "id", "user_id", "exchange_account_id", "product_id",
            "task_type", "status", "deleted_at",
        }
        offsets = {"users": 10, "exchange_rules": 20, "products": 30}
        expression = module.exchange_task_active_key_expr(
            "s", source_names, offsets,
        )
        self.assertIn("SHA2(CONCAT_WS('|',", expression)
        self.assertIn("s.`user_id` + 10", expression)
        self.assertIn("s.`exchange_account_id` + 20", expression)
        self.assertIn("s.`product_id` + 30", expression)
        self.assertIn("'rule_time'", expression)
        self.assertIn("'daily'", expression)
        self.assertIn("'all'", expression)
        self.assertIn("), 256)", expression)

    def test_duplicate_active_exchange_task_is_imported_as_failed(self):
        source_names = {
            "id", "user_id", "exchange_account_id", "product_id",
            "task_type", "status", "deleted_at", "last_result",
        }
        offsets = {
            "exchange_tasks": 100, "users": 10,
            "exchange_rules": 20, "products": 30,
        }
        expression = module.expression_for_column(
            "caiyun", "caiyun", "_legacy_exchange_tasks",
            "exchange_tasks", col("status"), source_names, offsets,
            "abcdef123456", "_legacy_users", "_legacy_accounts",
        )
        self.assertIn("existing.active_dedupe_key", expression)
        self.assertIn("SELECT MIN(candidate.id)", expression)
        self.assertIn("THEN 'failed'", expression)
        self.assertIn("ELSE s.`status`", expression)

    def test_duplicate_active_exchange_task_gets_archive_reason(self):
        source_names = {
            "id", "user_id", "exchange_account_id", "product_id",
            "task_type", "status", "deleted_at",
        }
        offsets = {
            "exchange_tasks": 100, "users": 10,
            "exchange_rules": 20, "products": 30,
        }
        expression = module.expression_for_column(
            "caiyun", "caiyun", "_legacy_exchange_tasks",
            "exchange_tasks", col("skip_reason"), source_names, offsets,
            "abcdef123456", "_legacy_users", "_legacy_accounts",
        )
        self.assertIn("duplicate_active_task_archived_by_legacy_import", expression)
        self.assertIn("ELSE COALESCE('', '')", expression)

    def test_default_generated_timestamp_is_not_skipped(self):
        self.assertFalse(col("created_at", extra="DEFAULT_GENERATED").generated)
        self.assertTrue(col("dedupe", extra="STORED GENERATED").generated)

    def test_unique_collision_values_are_deterministic(self):
        expression = module.suffix_expr("s.key_name", 50, "abc123", "(s.id + 9)")
        self.assertIn("__labc123_", expression)
        self.assertIn("50 - CHAR_LENGTH", expression)


if __name__ == "__main__":
    unittest.main()
