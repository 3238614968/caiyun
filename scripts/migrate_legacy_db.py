#!/usr/bin/env python3
"""本地旧版数据库迁移工具。

直接运行脚本会进入中文交互向导；也保留命令行参数，便于重复演练。
默认执行完整事务后回滚，只有明确选择正式导入时才会提交。
"""
from __future__ import annotations

import argparse
import base64
import dataclasses
import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import textwrap
import zipfile
from datetime import datetime, timezone
from pathlib import Path
from typing import Dict, List, Mapping, Optional, Sequence, Tuple

LATEST_SCHEMA_VERSION = "017_exchange_record_rule_fk"
DEFAULT_ARCHIVE = Path(r"D:\Hai\下载\caiyun_2026-07-14_13-15-25_mysql_data_BqANV.sql.zip")
DEFAULT_ENCRYPTION_VERSION = "v1"
DEFAULT_DATA_ENCRYPTION_KEYS = "v1=308e56efedac2f3e383483ce09c28d595dd856ad2a1ddf06c2f67ad59b8ed777"
CREDENTIAL_TABLES = ("accounts", "exchange_rules")
CREDENTIAL_COLUMNS = ("auth", "token", "jwt_token")
EXPECTED_SOURCE_TABLES = (
    "users", "accounts", "announcements", "products", "system_configs",
    "task_configs", "exchange_accounts", "exchange_tasks", "exchange_records",
    "exchange_task_history", "cloud_stats", "audit_logs", "task_logs",
    "web_socket_messages",
)
# This is also the foreign-key-safe insertion order.
TABLE_MAP: Tuple[Tuple[str, str], ...] = (
    ("users", "users"),
    ("accounts", "accounts"),
    ("announcements", "announcements"),
    ("products", "products"),
    ("system_configs", "system_configs"),
    ("task_configs", "task_configs"),
    ("exchange_accounts", "exchange_rules"),
    ("exchange_tasks", "exchange_tasks"),
    ("exchange_records", "exchange_records"),
    ("exchange_task_history", "exchange_task_history"),
    ("cloud_stats", "cloud_stats"),
    ("audit_logs", "audit_logs"),
    ("task_logs", "task_logs"),
    ("web_socket_messages", "web_socket_messages"),
)
REFERENCE_COLUMNS: Mapping[str, Mapping[str, str]] = {
    "accounts": {"user_id": "users"},
    "exchange_rules": {"user_id": "users", "account_id": "accounts"},
    "exchange_tasks": {
        "user_id": "users", "exchange_account_id": "exchange_rules",
        "exchange_rule_id": "exchange_rules", "product_id": "products",
    },
    "exchange_records": {
        "user_id": "users", "exchange_account_id": "exchange_rules",
        "exchange_rule_id": "exchange_rules", "exchange_task_id": "exchange_tasks",
        "product_id": "products",
    },
    "exchange_task_history": {"task_id": "exchange_tasks", "account_id": "accounts"},
    "cloud_stats": {"user_id": "users", "account_id": "accounts"},
    "audit_logs": {"user_id": "users"},
    "task_logs": {"user_id": "users", "account_id": "accounts"},
    "web_socket_messages": {"user_id": "users"},
}
TARGET_SOURCE_ALIASES: Mapping[Tuple[str, str], str] = {
    ("exchange_tasks", "exchange_rule_id"): "exchange_account_id",
    ("exchange_records", "exchange_rule_id"): "exchange_account_id",
}
IDENTIFIER_RE = re.compile(r"^[A-Za-z0-9_]+$")
CREATE_TABLE_RE = re.compile(r"CREATE TABLE `([^`]+)`", re.IGNORECASE)
INSERT_RE = re.compile(r"INSERT INTO `([^`]+)` VALUES\s*", re.IGNORECASE)
DATABASE_LEVEL_RE = re.compile(
    r"(?im)^\s*(?:CREATE\s+DATABASE|DROP\s+DATABASE|USE\s+)(?:\s|`)",
)


class MigrationError(RuntimeError):
    pass


@dataclasses.dataclass(frozen=True)
class Column:
    name: str
    nullable: bool
    default_is_null: bool
    default_value: str
    extra: str
    char_max_length: Optional[int]

    @property
    def generated(self) -> bool:
        return any(marker in self.extra.upper() for marker in ("VIRTUAL GENERATED", "STORED GENERATED"))

    @property
    def auto_increment(self) -> bool:
        return "auto_increment" in self.extra.lower()


@dataclasses.dataclass(frozen=True)
class ArchiveInfo:
    archive: Path
    sql_name: str
    sha256: str
    tables: Tuple[str, ...]
    estimated_rows: Mapping[str, int]


@dataclasses.dataclass
class MigrationPlan:
    archive: ArchiveInfo
    staging_prefix: str
    target_db: str
    run_tag: str
    source_counts: Dict[str, int]
    offsets: Dict[str, int]
    merge_sql: str


@dataclasses.dataclass
class CredentialRepairSummary:
    table: str
    scanned_rows: int = 0
    changed_rows: int = 0
    changed_fields: int = 0
    empty_fields: int = 0
    current_fields: int = 0
    plaintext_fields: int = 0
    legacy_fields: int = 0


class MySQLClient:
    def __init__(self, executable: str, host: str, port: int, user: str,
                 password: str, connect_timeout: int) -> None:
        resolved = shutil.which(executable)
        if not resolved and Path(executable).is_file():
            resolved = str(Path(executable).resolve())
        self.executable = resolved
        self.host = host
        self.port = port
        self.user = user
        self.password = password
        self.connect_timeout = connect_timeout
        self.pymysql = None
        if not resolved:
            try:
                import pymysql
                from pymysql.constants import CLIENT
            except ImportError as error:
                raise MigrationError(
                    "本机未找到 mysql.exe，且未安装 PyMySQL。请运行: "
                    "python -m pip install PyMySQL"
                ) from error
            self.pymysql = pymysql
            self.pymysql_client_flags = CLIENT.MULTI_STATEMENTS
            print("数据库驱动: PyMySQL（未使用 mysql.exe）")

    def open_pymysql(self, database: str, autocommit: bool = False):
        try:
            import pymysql
            from pymysql.constants import CLIENT
        except ImportError as error:
            raise MigrationError(
                "字段重加密需要 PyMySQL。请运行: python -m pip install PyMySQL"
            ) from error
        try:
            return pymysql.connect(
                host=self.host,
                port=self.port,
                user=self.user,
                password=self.password,
                database=database,
                charset="utf8mb4",
                connect_timeout=self.connect_timeout,
                autocommit=autocommit,
                client_flag=CLIENT.MULTI_STATEMENTS,
                cursorclass=pymysql.cursors.DictCursor,
            )
        except pymysql.MySQLError as error:
            code = error.args[0] if error.args else "unknown"
            message = error.args[1] if len(error.args) > 1 else str(error)
            raise MigrationError(
                f"PyMySQL 连接失败（错误 {code}）: {message}"
            ) from error

    def _command(self) -> List[str]:
        return [
            self.executable, "--batch", "--raw", "--skip-column-names",
            "--default-character-set=utf8mb4",
            f"--connect-timeout={self.connect_timeout}",
            "-h", self.host, "-P", str(self.port), "-u", self.user,
        ]

    def _environment(self) -> Dict[str, str]:
        env = dict(os.environ)
        if self.password:
            env["MYSQL_PWD"] = self.password
        return env

    @staticmethod
    def _format_pymysql_value(value: object) -> str:
        if value is None:
            return "NULL"
        if isinstance(value, bytes):
            return value.decode("utf-8", errors="replace")
        return str(value)

    def _run_pymysql(self, sql: str, database: Optional[str] = None) -> str:
        try:
            connection = self.pymysql.connect(
                host=self.host,
                port=self.port,
                user=self.user,
                password=self.password,
                database=database,
                charset="utf8mb4",
                connect_timeout=self.connect_timeout,
                autocommit=True,
                client_flag=self.pymysql_client_flags,
            )
            output_lines: List[str] = []
            try:
                with connection.cursor() as cursor:
                    cursor.execute(sql)
                    while True:
                        if cursor.description is not None:
                            for row in cursor.fetchall():
                                output_lines.append("\t".join(
                                    self._format_pymysql_value(value) for value in row
                                ))
                        if not cursor.nextset():
                            break
            finally:
                connection.close()
            return "\n".join(output_lines) + ("\n" if output_lines else "")
        except self.pymysql.MySQLError as error:
            code = error.args[0] if error.args else "unknown"
            message = error.args[1] if len(error.args) > 1 else str(error)
            raise MigrationError(f"PyMySQL 执行失败（错误 {code}）: {message}") from error

    def run(self, sql: str, database: Optional[str] = None) -> str:
        if self.pymysql is not None:
            return self._run_pymysql(sql, database)
        command = self._command() + ["-e", sql]
        if database:
            command.append(database)
        proc = subprocess.run(
            command, env=self._environment(), stdout=subprocess.PIPE,
            stderr=subprocess.PIPE, check=False,
        )
        if proc.returncode != 0:
            stderr = proc.stderr.decode("utf-8", errors="replace").strip()
            raise MigrationError(f"mysql 执行失败（退出码 {proc.returncode}）: {stderr}")
        return proc.stdout.decode("utf-8", errors="replace")

    def run_script(self, sql: str, database: Optional[str] = None) -> str:
        if self.pymysql is not None:
            return self._run_pymysql(sql, database)
        command = self._command() + ["--binary-mode=1"]
        if database:
            command.append(database)
        proc = subprocess.run(
            command, env=self._environment(), input=sql.encode("utf-8"),
            stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False,
        )
        if proc.returncode != 0:
            stderr = proc.stderr.decode("utf-8", errors="replace").strip()
            raise MigrationError(f"mysql 脚本执行失败（退出码 {proc.returncode}）: {stderr}")
        return proc.stdout.decode("utf-8", errors="replace")

    def run_file(self, sql_file: Path, database: str) -> str:
        if self.pymysql is not None:
            return self._run_pymysql(sql_file.read_text(encoding="utf-8-sig"), database)
        with sql_file.open("rb") as stream:
            proc = subprocess.run(
                self._command() + ["--binary-mode=1", database],
                env=self._environment(), stdin=stream,
                stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False,
            )
        if proc.returncode != 0:
            stderr = proc.stderr.decode("utf-8", errors="replace").strip()
            raise MigrationError(f"导入旧备份失败（退出码 {proc.returncode}）: {stderr}")
        return proc.stdout.decode("utf-8", errors="replace")

STAGING_TABLE_PREFIX_DEFAULT = "_legacy_"


def rewrite_source_table_names(sql: str, prefix: str, tables: Sequence[str]) -> str:
    """把旧表改为带前缀的中转表，并移除中转表不需要的外键约束。"""
    for table in tables:
        sql = sql.replace(f"`{table}`", f"`{prefix}{table}`")

    # InnoDB 外键约束名在同一数据库内必须唯一。新系统正式表已经占用了旧约束名，
    # 中转表只负责暂存数据，因此直接移除 CONSTRAINT ... FOREIGN KEY 定义。
    sql = re.sub(
        r"(?mi)^[ \t]*CONSTRAINT\s+`[^`]+`\s+FOREIGN\s+KEY\s+.*(?:\r?\n|$)",
        "",
        sql,
    )
    # 删除最后一个外键定义后，前一条 KEY/PRIMARY KEY 可能留下结尾逗号。
    sql = re.sub(r",([ \t]*\r?\n\)[ \t]*ENGINE=)", r"\1", sql)
    return sql


def quote_identifier(value: str) -> str:
    if not IDENTIFIER_RE.fullmatch(value):
        raise MigrationError(f"非法 MySQL 标识符: {value!r}")
    return f"`{value}`"


def sql_string(value: str) -> str:
    return "'" + value.replace("\\", "\\\\").replace("'", "''") + "'"


def normalize_encryption_version(raw: str) -> str:
    value = raw.strip().lower()
    if not value:
        return ""
    if not value.startswith("v"):
        value = "v" + value
    if not 2 <= len(value) <= 32 or not re.fullmatch(r"v[a-z0-9_-]+", value):
        return ""
    return value


def decode_encryption_key(raw: str) -> bytes:
    value = raw.strip()
    if len(value) == 32:
        return value.encode("utf-8")

    padded = value + "=" * ((4 - len(value) % 4) % 4)
    try:
        decoded = base64.b64decode(padded, validate=True)
        if len(decoded) == 32:
            return decoded
    except (ValueError, TypeError):
        pass

    try:
        decoded = bytes.fromhex(value)
        if len(decoded) == 32:
            return decoded
    except ValueError:
        pass
    raise MigrationError("加密密钥需要是 32 字节原始字符串、Base64 或十六进制")


def parse_encryption_keys(raw: str, current_version: str) -> Dict[str, bytes]:
    value = raw.strip()
    if not value:
        raise MigrationError("未输入 DATA_ENCRYPTION_KEYS")

    entries = [item.strip() for item in re.split(r"[\r\n;,]+", value) if item.strip()]
    if len(entries) == 1 and "=" not in entries[0]:
        return {current_version: decode_encryption_key(entries[0])}

    keys: Dict[str, bytes] = {}
    for entry in entries:
        if "=" not in entry:
            raise MigrationError(f"加密密钥条目 {entry!r} 缺少 version=key 格式")
        raw_version, raw_key = entry.split("=", 1)
        version = normalize_encryption_version(raw_version)
        if not version:
            raise MigrationError(f"加密版本号无效: {raw_version!r}")
        keys[version] = decode_encryption_key(raw_key)
    if not keys:
        raise MigrationError("未解析到任何可用加密密钥")
    return keys


def encrypted_value_parts(value: str) -> Optional[Tuple[str, str]]:
    if not value.startswith("enc:"):
        return None
    rest = value[4:]
    separator = rest.find(":")
    if separator <= 0 or separator >= len(rest) - 1:
        return None
    version = normalize_encryption_version(rest[:separator])
    if not version:
        return None
    return version, rest[separator + 1:]


def decrypt_credential_value(value: str, keys: Mapping[str, bytes]) -> Tuple[str, str]:
    parts = encrypted_value_parts(value)
    if parts is None:
        return value, ""
    version, encoded_payload = parts
    key = keys.get(version)
    if key is None:
        raise MigrationError(f"密文字段版本 {version} 缺少对应密钥")
    try:
        from Crypto.Cipher import AES
    except ImportError as error:
        raise MigrationError(
            "字段重加密需要 PyCryptodome。请运行: python -m pip install pycryptodome"
        ) from error

    padded = encoded_payload + "=" * ((4 - len(encoded_payload) % 4) % 4)
    try:
        payload = base64.b64decode(padded, validate=True)
    except (ValueError, TypeError) as error:
        raise MigrationError("密文字段 Base64 解码失败") from error
    if len(payload) < 12 + 16:
        raise MigrationError("密文字段长度不足")

    nonce = payload[:12]
    ciphertext = payload[12:-16]
    tag = payload[-16:]
    try:
        plaintext = AES.new(key, AES.MODE_GCM, nonce=nonce).decrypt_and_verify(
            ciphertext, tag
        )
        return plaintext.decode("utf-8"), version
    except (ValueError, UnicodeDecodeError) as error:
        raise MigrationError(f"密文字段使用版本 {version} 解密失败") from error


def encrypt_credential_value(value: str, version: str, key: bytes) -> str:
    try:
        from Crypto.Cipher import AES
    except ImportError as error:
        raise MigrationError(
            "字段重加密需要 PyCryptodome。请运行: python -m pip install pycryptodome"
        ) from error
    nonce = os.urandom(12)
    cipher = AES.new(key, AES.MODE_GCM, nonce=nonce)
    ciphertext, tag = cipher.encrypt_and_digest(value.encode("utf-8"))
    payload = nonce + ciphertext + tag
    encoded = base64.b64encode(payload).decode("ascii").rstrip("=")
    return f"enc:{version}:{encoded}"


def rotate_credential_value(
    value: Optional[str], current_version: str, keys: Mapping[str, bytes]
) -> Tuple[str, bool, str]:
    normalized = value or ""
    if not normalized:
        return normalized, False, "empty"
    plaintext, source_version = decrypt_credential_value(normalized, keys)
    if source_version == current_version:
        return normalized, False, "current"
    rewritten = encrypt_credential_value(
        plaintext, current_version, keys[current_version]
    )
    return rewritten, rewritten != normalized, "legacy" if source_version else "plaintext"


def _apply_repair_state(summary: CredentialRepairSummary, state: str) -> None:
    if state == "empty":
        summary.empty_fields += 1
    elif state == "current":
        summary.current_fields += 1
    elif state == "plaintext":
        summary.plaintext_fields += 1
    elif state == "legacy":
        summary.legacy_fields += 1


def repair_credential_fields(
    client: MySQLClient,
    target_db: str,
    keys: Mapping[str, bytes],
    current_version: str,
    apply: bool,
) -> List[CredentialRepairSummary]:
    if current_version not in keys:
        raise MigrationError(f"当前版本 {current_version} 缺少对应加密密钥")
    connection = client.open_pymysql(target_db, autocommit=False)
    summaries: List[CredentialRepairSummary] = []
    try:
        with connection.cursor() as cursor:
            for table in CREDENTIAL_TABLES:
                summary = CredentialRepairSummary(table=table)
                summaries.append(summary)
                lock_clause = " FOR UPDATE" if apply else ""
                cursor.execute(
                    f"SELECT id, auth, token, jwt_token "
                    f"FROM {quote_identifier(table)} ORDER BY id ASC{lock_clause}"
                )
                rows = cursor.fetchall()
                for row in rows:
                    summary.scanned_rows += 1
                    updates: Dict[str, str] = {}
                    originals: Dict[str, str] = {}
                    for column in CREDENTIAL_COLUMNS:
                        original = row.get(column) or ""
                        originals[column] = original
                        try:
                            rewritten, changed, state = rotate_credential_value(
                                original, current_version, keys
                            )
                        except MigrationError as error:
                            raise MigrationError(
                                f"表 {table} id={row['id']} column={column}: {error}"
                            ) from error
                        _apply_repair_state(summary, state)
                        if changed:
                            updates[column] = rewritten
                    if not updates:
                        continue
                    summary.changed_rows += 1
                    summary.changed_fields += len(updates)
                    if not apply:
                        continue

                    assignments = [f"{quote_identifier(column)}=%s" for column in updates]
                    assignments.append("updated_at=NOW(6)")
                    parameters: List[object] = list(updates.values())
                    parameters.extend([
                        row["id"],
                        originals["auth"],
                        originals["token"],
                        originals["jwt_token"],
                    ])
                    cursor.execute(
                        f"UPDATE {quote_identifier(table)} "
                        f"SET {', '.join(assignments)} "
                        "WHERE id=%s "
                        "AND BINARY COALESCE(auth, '')=BINARY %s "
                        "AND BINARY COALESCE(token, '')=BINARY %s "
                        "AND BINARY COALESCE(jwt_token, '')=BINARY %s",
                        parameters,
                    )
                    if cursor.rowcount != 1:
                        raise MigrationError(
                            f"表 {table} id={row['id']} 扫描后发生并发更新，已停止提交"
                        )
        if apply:
            connection.commit()
        else:
            connection.rollback()
        return summaries
    except MigrationError:
        connection.rollback()
        raise
    except Exception as error:
        connection.rollback()
        args = getattr(error, "args", ())
        code = args[0] if args else "unknown"
        message = args[1] if len(args) > 1 else str(error)
        raise MigrationError(f"字段重加密数据库操作失败（错误 {code}）: {message}") from error
    finally:
        connection.close()


def print_repair_summaries(summaries: Sequence[CredentialRepairSummary]) -> None:
    for summary in summaries:
        print(
            f"{summary.table}: scanned_rows={summary.scanned_rows} "
            f"changed_rows={summary.changed_rows} changed_fields={summary.changed_fields} "
            f"plaintext_fields={summary.plaintext_fields} "
            f"legacy_fields={summary.legacy_fields} "
            f"current_fields={summary.current_fields} empty_fields={summary.empty_fields}"
        )


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def count_insert_tuples(sql: str, start: int) -> Tuple[int, int]:
    count = depth = 0
    in_string = escaped = False
    index = start
    while index < len(sql):
        char = sql[index]
        if in_string:
            if escaped:
                escaped = False
            elif char == "\\":
                escaped = True
            elif char == "'":
                if index + 1 < len(sql) and sql[index + 1] == "'":
                    index += 1
                else:
                    in_string = False
        else:
            if char == "'":
                in_string = True
            elif char == "(":
                if depth == 0:
                    count += 1
                depth += 1
            elif char == ")":
                depth -= 1
                if depth < 0:
                    raise MigrationError("备份 INSERT 语句括号不匹配")
            elif char == ";" and depth == 0:
                return count, index + 1
        index += 1
    raise MigrationError("备份中存在未结束的 INSERT 语句")


def inspect_sql_text(sql: str, archive: Path, sql_name: str, digest: str) -> ArchiveInfo:
    if DATABASE_LEVEL_RE.search(sql):
        raise MigrationError("备份包含数据库级 CREATE/DROP/USE 语句，已停止以保护目标数据库")
    tables = tuple(dict.fromkeys(CREATE_TABLE_RE.findall(sql)))
    estimated: Dict[str, int] = {name: 0 for name in tables}
    position = 0
    while True:
        match = INSERT_RE.search(sql, position)
        if not match:
            break
        rows, position = count_insert_tuples(sql, match.end())
        estimated[match.group(1)] = estimated.get(match.group(1), 0) + rows
    return ArchiveInfo(archive, sql_name, digest, tables, estimated)


def extract_and_inspect_archive(archive: Path, destination: Path) -> Tuple[ArchiveInfo, Path]:
    if not archive.is_file():
        raise MigrationError(f"备份文件不存在: {archive}")
    digest = sha256_file(archive)
    if archive.suffix.lower() == ".zip":
        with zipfile.ZipFile(archive) as bundle:
            members = [item for item in bundle.infolist() if not item.is_dir()]
            sql_members = [item for item in members if item.filename.lower().endswith(".sql")]
            if len(sql_members) != 1:
                raise MigrationError(f"ZIP 中必须恰好包含一个 SQL 文件，当前为 {len(sql_members)} 个")
            member = sql_members[0]
            member_path = Path(member.filename)
            if member_path.is_absolute() or ".." in member_path.parts:
                raise MigrationError("ZIP SQL 路径不安全")
            if member.file_size > 8 * 1024 * 1024 * 1024:
                raise MigrationError("SQL 解压后超过 8 GiB 限制")
            sql_path = destination / "legacy.sql"
            with bundle.open(member) as source, sql_path.open("wb") as target:
                shutil.copyfileobj(source, target, length=1024 * 1024)
            sql_name = member.filename
    elif archive.suffix.lower() == ".sql":
        sql_path = destination / "legacy.sql"
        shutil.copy2(archive, sql_path)
        sql_name = archive.name
    else:
        raise MigrationError("备份格式必须是 .sql 或仅包含一个 .sql 的 .zip")
    sql_text = sql_path.read_text(encoding="utf-8", errors="strict")
    return inspect_sql_text(sql_text, archive, sql_name, digest), sql_path


def require_expected_source_tables(info: ArchiveInfo) -> None:
    actual, expected = set(info.tables), set(EXPECTED_SOURCE_TABLES)
    missing = sorted(expected - actual)
    unknown = sorted(t for t in actual - expected if info.estimated_rows.get(t, 0) > 0)
    if missing:
        raise MigrationError("旧备份缺少必要表: " + ", ".join(missing))
    if unknown:
        raise MigrationError("旧备份含未映射且有数据的表: " + ", ".join(unknown))


def parse_tsv(output: str, expected_columns: int) -> List[List[str]]:
    rows: List[List[str]] = []
    for line in output.splitlines():
        if not line.strip():
            continue
        values = line.split("\t")
        if len(values) != expected_columns:
            raise MigrationError(f"mysql 返回列数异常: {line!r}")
        rows.append(values)
    return rows


def query_columns(client: MySQLClient, database: str, table: str) -> List[Column]:
    sentinel = "__CAIYUN_NULL_DEFAULT__"
    sql = f"""
SELECT COLUMN_NAME, IS_NULLABLE,
       IF(COLUMN_DEFAULT IS NULL, {sql_string(sentinel)}, COLUMN_DEFAULT),
       EXTRA,
       IF(CHARACTER_MAXIMUM_LENGTH IS NULL, '', CHARACTER_MAXIMUM_LENGTH)
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = {sql_string(database)} AND TABLE_NAME = {sql_string(table)}
ORDER BY ORDINAL_POSITION
"""
    result = []
    for name, nullable, default, extra, char_length in parse_tsv(client.run(sql), 5):
        result.append(Column(
            name=name,
            nullable=nullable == "YES",
            default_is_null=default == sentinel,
            default_value="" if default == sentinel else default,
            extra=extra,
            char_max_length=int(char_length) if char_length else None,
        ))
    if not result:
        raise MigrationError(f"表不存在或没有字段: {database}.{table}")
    return result


def query_scalar_int(client: MySQLClient, sql: str, database: Optional[str] = None) -> int:
    output = client.run(sql, database).strip()
    if not re.fullmatch(r"-?\d+", output):
        raise MigrationError(f"期望整数查询结果，实际为: {output!r}")
    return int(output)


def normalize_email_expr(alias: str = "s") -> str:
    value = f"TRIM({alias}.email)"
    return (
        f"CASE WHEN {alias}.deleted_at IS NOT NULL OR NULLIF({value}, '') IS NULL THEN NULL "
        f"WHEN LOCATE('@', {value}) > 0 THEN CONCAT(SUBSTRING_INDEX({value}, '@', 1), '@', "
        f"LOWER(SUBSTRING_INDEX({value}, '@', -1))) ELSE {value} END"
    )


def suffix_expr(base_expr: str, max_length: int, tag: str, new_id_expr: str) -> str:
    suffix = f"CONCAT('__l{tag}_', {new_id_expr})"
    return f"CONCAT(LEFT({base_expr}, GREATEST(1, {max_length} - CHAR_LENGTH({suffix}))), {suffix})"


def username_expr(target_db: str, offset: int, tag: str) -> str:
    new_id = f"(s.id + {offset})"
    candidate = suffix_expr("TRIM(s.username)", 50, tag, new_id)
    conflict = (
        f"EXISTS (SELECT 1 FROM {quote_identifier(target_db)}.users u "
        f"WHERE u.username = s.username OR u.normalized_username = LOWER(TRIM(s.username)))"
    )
    return f"CASE WHEN {conflict} THEN {candidate} ELSE s.username END"


def email_exprs(source_db: str, source_users_table: str, target_db: str,
                offset: int, tag: str) -> Tuple[str, str]:
    normalized = normalize_email_expr("s")
    new_id = f"(s.id + {offset})"
    alias_email = f"CONCAT('legacy+{tag}-', {new_id}, '@import.invalid')"
    duplicate_in_source = (
        f"(SELECT COUNT(*) FROM {quote_identifier(source_db)}.{quote_identifier(source_users_table)} su "
        f"WHERE su.deleted_at IS NULL AND {normalize_email_expr('su')} = {normalized}) > 1"
    )
    conflict = (
        f"EXISTS (SELECT 1 FROM {quote_identifier(target_db)}.users tu "
        f"WHERE tu.normalized_email = {normalized}) OR {duplicate_in_source}"
    )
    email = (
        f"CASE WHEN s.deleted_at IS NOT NULL OR NULLIF(TRIM(s.email), '') IS NULL THEN s.email "
        f"WHEN {conflict} THEN {alias_email} ELSE s.email END"
    )
    normalized_result = (
        f"CASE WHEN s.deleted_at IS NOT NULL OR NULLIF(TRIM(s.email), '') IS NULL THEN NULL "
        f"WHEN {conflict} THEN LOWER({alias_email}) ELSE {normalized} END"
    )
    return email, normalized_result


def natural_unique_expr(target_db: str, target_table: str, column: str,
                        source_expr: str, replacement_expr: str) -> str:
    conflict = (
        f"EXISTS (SELECT 1 FROM {quote_identifier(target_db)}.{quote_identifier(target_table)} t "
        f"WHERE t.{quote_identifier(column)} = {source_expr})"
    )
    return f"CASE WHEN {conflict} THEN {replacement_expr} ELSE {source_expr} END"


def source_column_for(target_table: str, target_column: str) -> str:
    return TARGET_SOURCE_ALIASES.get((target_table, target_column), target_column)


def shifted_reference(source_expr: str, offset: int) -> str:
    return (
        f"CASE WHEN {source_expr} IS NULL OR {source_expr} = 0 "
        f"THEN {source_expr} ELSE {source_expr} + {offset} END"
    )


def account_preference_order(alias: str) -> str:
    return (
        f"({alias}.deleted_at IS NULL) DESC, {alias}.is_active DESC, "
        f"{alias}.updated_at DESC, {alias}.id DESC"
    )


def account_insert_filter(source_db: str, target_db: str,
                          source_accounts_table: str, user_offset: int) -> str:
    source = f"{quote_identifier(source_db)}.{quote_identifier(source_accounts_table)}"
    target = f"{quote_identifier(target_db)}.accounts"
    canonical = (
        f"s.id = (SELECT ca.id FROM {source} ca "
        "WHERE ca.user_id = s.user_id AND ca.phone = s.phone "
        f"ORDER BY {account_preference_order('ca')} LIMIT 1)"
    )
    target_missing = (
        f"NOT EXISTS (SELECT 1 FROM {target} ta "
        f"WHERE ta.user_id = s.user_id + {user_offset} AND ta.phone = s.phone)"
    )
    return f"{canonical} AND {target_missing}"


def account_reference_expr(source_db: str, target_db: str,
                           source_accounts_table: str, source_expr: str,
                           account_offset: int, user_offset: int) -> str:
    source = f"{quote_identifier(source_db)}.{quote_identifier(source_accounts_table)}"
    target = f"{quote_identifier(target_db)}.accounts"
    existing_target = (
        f"SELECT ta.id FROM {source} original "
        f"JOIN {target} ta ON ta.user_id = original.user_id + {user_offset} "
        "AND ta.phone = original.phone "
        f"WHERE original.id = {source_expr} ORDER BY ta.id LIMIT 1"
    )
    canonical_legacy = (
        f"SELECT ca.id + {account_offset} FROM {source} original "
        f"JOIN {source} ca ON ca.user_id = original.user_id AND ca.phone = original.phone "
        f"WHERE original.id = {source_expr} "
        f"ORDER BY {account_preference_order('ca')} LIMIT 1"
    )
    return (
        f"CASE WHEN {source_expr} IS NULL OR {source_expr} = 0 THEN {source_expr} "
        f"ELSE COALESCE(({existing_target}), ({canonical_legacy})) END"
    )


def expected_account_insert_count(client: MySQLClient, source_db: str,
                                  target_db: str, source_accounts_table: str,
                                  user_offset: int) -> int:
    where_sql = account_insert_filter(
        source_db, target_db, source_accounts_table, user_offset,
    )
    return query_scalar_int(
        client,
        f"SELECT COUNT(*) FROM {quote_identifier(source_accounts_table)} s WHERE {where_sql}",
        source_db,
    )


def cloud_stat_insert_filter(source_db: str, target_db: str,
                             source_table: str, source_accounts_table: str,
                             offsets: Mapping[str, int]) -> str:
    source = f"{quote_identifier(source_db)}.{quote_identifier(source_table)}"
    target = f"{quote_identifier(target_db)}.cloud_stats"
    source_account = account_reference_expr(
        source_db, target_db, source_accounts_table, "s.account_id",
        offsets["accounts"], offsets["users"],
    )
    candidate_account = account_reference_expr(
        source_db, target_db, source_accounts_table, "candidate.account_id",
        offsets["accounts"], offsets["users"],
    )
    canonical = (
        f"s.id = (SELECT MIN(candidate.id) FROM {source} candidate "
        "WHERE candidate.user_id = s.user_id AND candidate.date = s.date "
        f"AND {candidate_account} = {source_account})"
    )
    target_missing = (
        f"NOT EXISTS (SELECT 1 FROM {target} existing "
        f"WHERE existing.user_id = s.user_id + {offsets['users']} "
        f"AND existing.account_id = {source_account} "
        "AND existing.date = s.date)"
    )
    return f"{canonical} AND {target_missing}"


def expected_cloud_stat_insert_count(client: MySQLClient, source_db: str,
                                     target_db: str, source_table: str,
                                     source_accounts_table: str,
                                     offsets: Mapping[str, int]) -> int:
    where_sql = cloud_stat_insert_filter(
        source_db, target_db, source_table, source_accounts_table, offsets,
    )
    return query_scalar_int(
        client,
        f"SELECT COUNT(*) FROM {quote_identifier(source_table)} s WHERE {where_sql}",
        source_db,
    )


def exchange_task_schedule_value(alias: str, source_names: set[str],
                                 column: str, fallback_sql: str,
                                 numeric: bool = False) -> str:
    if column not in source_names:
        return fallback_sql
    value = f"{alias}.{quote_identifier(column)}"
    if numeric:
        return f"COALESCE({value}, {fallback_sql})"
    return f"COALESCE(NULLIF({value}, ''), {fallback_sql})"


def exchange_task_active_key_expr(alias: str, source_names: set[str],
                                  offsets: Mapping[str, int]) -> str:
    rule_column = source_column_for("exchange_tasks", "exchange_rule_id")
    parts = [
        f"{alias}.`user_id` + {offsets['users']}",
        f"{alias}.{quote_identifier(rule_column)} + {offsets['exchange_rules']}",
        f"{alias}.`product_id` + {offsets['products']}",
        exchange_task_schedule_value(alias, source_names, "task_type", "'fixed'"),
        exchange_task_schedule_value(
            alias, source_names, "scheduled_exchange_time", "'rule_time'",
        ),
        exchange_task_schedule_value(alias, source_names, "restock_cycle", "'daily'"),
        exchange_task_schedule_value(
            alias, source_names, "restock_weekday", "-1", numeric=True,
        ),
        exchange_task_schedule_value(
            alias, source_names, "restock_day_of_month", "-1", numeric=True,
        ),
        exchange_task_schedule_value(alias, source_names, "restock_times", "'-'"),
        exchange_task_schedule_value(alias, source_names, "custom_cron", "'-'"),
        exchange_task_schedule_value(alias, source_names, "calendar_policy", "'all'"),
        exchange_task_schedule_value(alias, source_names, "holiday_dates", "'-'"),
        exchange_task_schedule_value(alias, source_names, "workday_dates", "'-'"),
    ]
    return "SHA2(CONCAT_WS('|', " + ", ".join(parts) + "), 256)"


def exchange_task_active_condition(alias: str) -> str:
    return (
        f"{alias}.deleted_at IS NULL "
        f"AND {alias}.status IN ('pending', 'running')"
    )


def exchange_task_conflict_expr(source_db: str, target_db: str,
                                source_table: str, source_names: set[str],
                                offsets: Mapping[str, int], alias: str = "s") -> str:
    source = f"{quote_identifier(source_db)}.{quote_identifier(source_table)}"
    target = f"{quote_identifier(target_db)}.exchange_tasks"
    source_key = exchange_task_active_key_expr(alias, source_names, offsets)
    candidate_key = exchange_task_active_key_expr("candidate", source_names, offsets)
    target_exists = (
        f"EXISTS (SELECT 1 FROM {target} existing "
        f"WHERE existing.active_dedupe_key = {source_key})"
    )
    canonical_source = (
        f"{alias}.id = (SELECT MIN(candidate.id) FROM {source} candidate "
        f"WHERE {exchange_task_active_condition('candidate')} "
        f"AND {candidate_key} = {source_key})"
    )
    return (
        f"({exchange_task_active_condition(alias)} AND "
        f"({target_exists} OR NOT ({canonical_source})))"
    )


def expression_for_column(source_db: str, target_db: str, source_table: str,
                          target_table: str, column: Column, source_names: set[str],
                          offsets: Mapping[str, int], tag: str,
                          source_users_table: str,
                          source_accounts_table: str) -> Optional[str]:
    name = column.name
    source_name = source_column_for(target_table, name)
    source_expr = f"s.{quote_identifier(source_name)}"
    new_id = f"(s.id + {offsets[target_table]})"

    if name == "id":
        return new_id

    if target_table == "exchange_tasks" and name in {"status", "skip_reason", "last_result"}:
        conflict = exchange_task_conflict_expr(
            source_db, target_db, source_table, source_names, offsets,
        )
        if name == "status":
            original = source_expr if source_name in source_names else "'pending'"
            return f"CASE WHEN {conflict} THEN 'failed' ELSE {original} END"
        if name == "skip_reason":
            original = source_expr if source_name in source_names else "''"
            return (
                f"CASE WHEN {conflict} THEN "
                "'duplicate_active_task_archived_by_legacy_import' "
                f"ELSE COALESCE({original}, '') END"
            )
        original = source_expr if source_name in source_names else "NULL"
        return (
            f"CASE WHEN {conflict} THEN "
            "'重复活动任务已在旧数据导入时归档为失败状态' "
            f"ELSE {original} END"
        )

    if target_table == "users":
        user_name = username_expr(target_db, offsets["users"], tag)
        email, normalized_email = email_exprs(
            source_db, source_users_table, target_db, offsets["users"], tag,
        )
        if name == "username":
            return user_name
        if name == "normalized_username":
            return (
                f"CASE WHEN s.deleted_at IS NULL THEN LOWER(TRIM({user_name})) "
                f"ELSE CONCAT('deleted-', {new_id}) END"
            )
        if name == "email":
            return email
        if name == "normalized_email":
            return normalized_email

    if target_table == "products" and name == "prize_id":
        replacement = f"CONCAT('legacy-{tag}-', {new_id})"
        return natural_unique_expr(target_db, target_table, name, source_expr, replacement)
    if target_table == "system_configs" and name == "key_name":
        replacement = suffix_expr("s.key_name", column.char_max_length or 100, tag, new_id)
        return natural_unique_expr(target_db, target_table, name, source_expr, replacement)
    if target_table == "task_configs" and name == "task_type":
        replacement = suffix_expr("s.task_type", column.char_max_length or 50, tag, new_id)
        return natural_unique_expr(target_db, target_table, name, source_expr, replacement)
    if target_table == "web_socket_messages" and name == "message_id":
        return f"CONCAT('legacy-{tag}-', {new_id})"
    if target_table == "web_socket_messages" and name == "sequence":
        return new_id

    reference_target = REFERENCE_COLUMNS.get(target_table, {}).get(name)
    if reference_target:
        if source_name not in source_names:
            return None
        if reference_target == "accounts":
            return account_reference_expr(
                source_db, target_db, source_accounts_table, source_expr,
                offsets["accounts"], offsets["users"],
            )
        return shifted_reference(source_expr, offsets[reference_target])
    if source_name in source_names:
        return source_expr
    return None


def validate_and_build_insert(client: MySQLClient, source_db: str, target_db: str,
                              source_table: str, target_table: str,
                              offsets: Mapping[str, int], tag: str,
                              source_users_table: str,
                              source_accounts_table: str) -> str:
    source_columns = query_columns(client, source_db, source_table)
    target_columns = query_columns(client, target_db, target_table)
    source_names = {column.name for column in source_columns}
    insert_columns: List[str] = []
    select_expressions: List[str] = []
    missing_required: List[str] = []

    for column in target_columns:
        if column.generated:
            continue
        expression = expression_for_column(
            source_db, target_db, source_table, target_table, column,
            source_names, offsets, tag, source_users_table,
            source_accounts_table,
        )
        if expression is not None:
            insert_columns.append(quote_identifier(column.name))
            select_expressions.append(expression)
        elif not (column.auto_increment or column.nullable or not column.default_is_null):
            missing_required.append(column.name)

    if missing_required:
        raise MigrationError(
            f"{target_db}.{target_table} 存在旧库无法提供的必填字段: "
            + ", ".join(missing_required)
        )
    columns_sql = ", ".join(insert_columns)
    values_sql = ",\n       ".join(select_expressions)
    where_clause = ""
    if target_table == "accounts":
        where_clause = "\nWHERE " + account_insert_filter(
            source_db, target_db, source_accounts_table, offsets["users"],
        )
    elif target_table == "cloud_stats":
        where_clause = "\nWHERE " + cloud_stat_insert_filter(
            source_db, target_db, source_table, source_accounts_table, offsets,
        )
    return textwrap.dedent(f"""
        INSERT INTO {quote_identifier(target_db)}.{quote_identifier(target_table)} ({columns_sql})
        SELECT {values_sql}
        FROM {quote_identifier(source_db)}.{quote_identifier(source_table)} s{where_clause};
        SELECT 'MIGRATED', {sql_string(target_table)}, ROW_COUNT();
    """).strip()


def build_merge_sql(client: MySQLClient, archive: ArchiveInfo, source_db: str,
                    target_db: str, offsets: Mapping[str, int],
                    source_counts: Mapping[str, int], execute: bool,
                    prefix: str) -> str:
    tag = archive.sha256[:12]
    sections = [
        "SET NAMES utf8mb4;",
        "SET SESSION TRANSACTION ISOLATION LEVEL SERIALIZABLE;",
        "START TRANSACTION;",
    ]
    source_users_table = prefix + "users"
    source_accounts_table = prefix + "accounts"
    for source_table, target_table in TABLE_MAP:
        sections.append(validate_and_build_insert(
            client, source_db, target_db, prefix + source_table, target_table,
            offsets, tag, source_users_table, source_accounts_table,
        ))
    if execute:
        sections.append("COMMIT;")
    else:
        sections.append("ROLLBACK;")
    return "\n\n".join(sections) + "\n"


def drop_staging_tables(client: MySQLClient, database: str, prefix: str) -> None:
    statements = ["SET FOREIGN_KEY_CHECKS=0"]
    statements.extend(
        f"DROP TABLE IF EXISTS {quote_identifier(prefix + table)}"
        for table in reversed(EXPECTED_SOURCE_TABLES)
    )
    statements.append("SET FOREIGN_KEY_CHECKS=1")
    client.run_script(";\n".join(statements) + ";\n", database)


def query_database_tables(client: MySQLClient, database: str) -> set[str]:
    sql = (
        "SELECT TABLE_NAME FROM information_schema.TABLES "
        f"WHERE TABLE_SCHEMA={sql_string(database)} AND TABLE_TYPE='BASE TABLE'"
    )
    return {row[0] for row in parse_tsv(client.run(sql), 1)}


def validate_target_schema(client: MySQLClient, target_db: str) -> None:
    exists = query_scalar_int(
        client,
        "SELECT COUNT(*) FROM information_schema.SCHEMATA "
        f"WHERE SCHEMA_NAME={sql_string(target_db)}",
    )
    if exists != 1:
        raise MigrationError(f"目标数据库不存在: {target_db}")
    target_tables = query_database_tables(client, target_db)
    required = {target for _, target in TABLE_MAP} | {"schema_migrations"}
    missing = sorted(required - target_tables)
    if missing:
        raise MigrationError("目标数据库缺少表: " + ", ".join(missing))
    version_count = query_scalar_int(
        client,
        "SELECT COUNT(*) FROM schema_migrations "
        f"WHERE version={sql_string(LATEST_SCHEMA_VERSION)}",
        target_db,
    )
    if version_count != 1:
        raise MigrationError(
            f"目标数据库尚未应用 {LATEST_SCHEMA_VERSION}；请先执行 caiyun-linux migrate"
        )


def source_row_counts(client: MySQLClient, target_db: str, prefix: str) -> Dict[str, int]:
    return {
        table: query_scalar_int(
            client,
            f"SELECT COUNT(*) FROM {quote_identifier(prefix + table)}",
            target_db,
        )
        for table in EXPECTED_SOURCE_TABLES
    }


def target_offsets(client: MySQLClient, target_db: str) -> Dict[str, int]:
    result: Dict[str, int] = {}
    for _, target_table in TABLE_MAP:
        if target_table not in result:
            result[target_table] = query_scalar_int(
                client,
                f"SELECT COALESCE(MAX(id), 0) FROM {quote_identifier(target_table)}",
                target_db,
            )
    return result


def parse_migrated_counts(output: str) -> Dict[str, int]:
    counts: Dict[str, int] = {}
    for line in output.splitlines():
        values = line.split("\t")
        if len(values) == 3 and values[0] == "MIGRATED":
            counts[values[1]] = int(values[2])
    return counts


def validate_migrated_counts(actual: Mapping[str, int],
                             expected_source: Mapping[str, int]) -> None:
    errors = []
    for source_table, target_table in TABLE_MAP:
        expected = expected_source[source_table]
        got = actual.get(target_table)
        if got != expected:
            errors.append(f"{source_table}->{target_table}: expected={expected}, actual={got}")
    if errors:
        raise MigrationError("迁移行数校验失败: " + "; ".join(errors))


def ledger_exists(client: MySQLClient, target_db: str) -> bool:
    return query_scalar_int(
        client,
        "SELECT COUNT(*) FROM information_schema.TABLES "
        f"WHERE TABLE_SCHEMA={sql_string(target_db)} "
        "AND TABLE_NAME='legacy_migration_runs' AND TABLE_TYPE='BASE TABLE'",
    ) == 1


def ensure_ledger(client: MySQLClient, target_db: str, archive: ArchiveInfo) -> None:
    ddl = f"""
CREATE TABLE IF NOT EXISTS {quote_identifier(target_db)}.legacy_migration_runs (
    archive_sha256 CHAR(64) NOT NULL,
    archive_name VARCHAR(255) NOT NULL,
    source_database VARCHAR(64) NOT NULL,
    status VARCHAR(20) NOT NULL,
    started_at DATETIME(6) NOT NULL,
    completed_at DATETIME(6) NULL,
    details JSON NULL,
    PRIMARY KEY (archive_sha256),
    KEY idx_legacy_migration_status (status, started_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
"""
    client.run(ddl)
    existing = query_scalar_int(
        client,
        "SELECT COUNT(*) FROM legacy_migration_runs "
        f"WHERE archive_sha256={sql_string(archive.sha256)}",
        target_db,
    )
    if existing:
        raise MigrationError("该备份 SHA256 已有迁移记录，已停止重复导入")


def check_previous_run(client: MySQLClient, target_db: str, archive: ArchiveInfo) -> None:
    if not ledger_exists(client, target_db):
        return
    existing = query_scalar_int(
        client,
        "SELECT COUNT(*) FROM legacy_migration_runs "
        f"WHERE archive_sha256={sql_string(archive.sha256)}",
        target_db,
    )
    if existing:
        raise MigrationError("该备份 SHA256 已有迁移记录，已停止重复演练或导入")


def write_report(output_dir: Path, plan: MigrationPlan, mode: str,
                 migrated_counts: Optional[Mapping[str, int]] = None) -> Tuple[Path, Path]:
    output_dir.mkdir(parents=True, exist_ok=True)
    stem = f"legacy-migration-{plan.run_tag}"
    sql_path = output_dir / f"{stem}.sql"
    report_path = output_dir / f"{stem}.json"
    sql_path.write_text(plan.merge_sql, encoding="utf-8", newline="\n")
    report = {
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "mode": mode,
        "archive": str(plan.archive.archive),
        "archive_member": plan.archive.sql_name,
        "archive_sha256": plan.archive.sha256,
        "target_database": plan.target_db,
        "staging_prefix": plan.staging_prefix,
        "source_counts": plan.source_counts,
        "offsets": plan.offsets,
        "migrated_counts": dict(migrated_counts or {}),
        "sql_file": str(sql_path),
    }
    report_path.write_text(
        json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )
    return sql_path, report_path


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description="将旧版移动云盘 MySQL 备份合并到已有数据的新系统数据库",
        formatter_class=argparse.ArgumentDefaultsHelpFormatter,
    )
    parser.add_argument("--archive", type=Path, help="旧版 .sql 或 .sql.zip 备份")
    parser.add_argument("--interactive", action="store_true", help="强制使用中文交互向导")
    parser.add_argument("--inspect-only", action="store_true", help="仅离线检查备份，不连接数据库")
    parser.add_argument("--execute", action="store_true", help="提交迁移；默认事务演练后回滚")
    parser.add_argument(
        "--repair-credentials", action="store_true",
        help="仅修复迁移后的账号敏感字段，不读取或导入备份",
    )
    parser.add_argument("--mysql-bin", default=os.getenv("MYSQL_BIN", "mysql"), help="mysql 客户端路径")
    parser.add_argument("--host", default=os.getenv("DB_HOST", "127.0.0.1"))
    parser.add_argument("--port", default=int(os.getenv("DB_PORT", "3306")), type=int)
    parser.add_argument("--user", default=os.getenv("DB_USER", "root"))
    parser.add_argument("--target-db", default=os.getenv("DB_NAME", "caiyun"))
    parser.add_argument(
        "--password-env", default="DB_PASSWORD",
        help="保存数据库密码的环境变量名；密码不会进入命令行参数",
    )
    parser.add_argument("--connect-timeout", default=10, type=int)
    parser.add_argument(
        "--staging-prefix", default=STAGING_TABLE_PREFIX_DEFAULT,
        help="目标库内旧数据表的名称前缀，用于替代独立的临时库；默认 _legacy_",
    )
    parser.add_argument("--keep-staging", action="store_true", help="保留前缀表用于排查")
    parser.add_argument("--output-dir", type=Path, default=Path(".local/migration-reports"))
    return parser


def prompt_value(label: str, default: object = "") -> str:
    suffix = f" [{default}]" if str(default) else ""
    try:
        value = input(f"{label}{suffix}: ").strip()
    except (EOFError, KeyboardInterrupt) as error:
        raise MigrationError("交互操作已取消") from error
    return value or str(default)


def configure_interactive_archive_and_mode(args: argparse.Namespace) -> None:
    print("=" * 58)
    print("移动云盘旧数据库迁移（本地交互工具）")
    print("支持备份迁移，以及迁移后的账号敏感字段重加密修复。")
    print("=" * 58)
    print("\n请选择操作：")
    print("  1. 仅检查备份（不连接数据库）")
    print("  2. 数据库迁移演练（执行后回滚，推荐）")
    print("  3. 正式导入（提交数据）")
    print("  4. 修复迁移后敏感字段（解决账号相关接口 500）")
    print("  0. 退出")
    while True:
        choice = prompt_value("输入序号", "2")
        if choice == "0":
            raise SystemExit(0)
        if choice == "4":
            args.repair_credentials = True
            return
        if choice in {"1", "2", "3"}:
            args.inspect_only = choice == "1"
            args.execute = choice == "3"
            default_archive = args.archive or DEFAULT_ARCHIVE
            archive_text = prompt_value("旧数据库备份文件", default_archive).strip('"\'')
            if not archive_text:
                raise MigrationError("未指定旧数据库备份文件")
            args.archive = Path(archive_text)
            return
        print("请输入 0、1、2、3 或 4。")

def configure_interactive_database(args: argparse.Namespace) -> str:
    print("\n数据库连接信息（直接回车使用括号内默认值）：")
    args.mysql_bin = prompt_value("mysql 客户端（缺少时自动使用 PyMySQL）", args.mysql_bin)
    args.host = prompt_value("数据库地址", args.host)
    port_text = prompt_value("数据库端口", args.port)
    try:
        args.port = int(port_text)
    except ValueError as error:
        raise MigrationError(f"数据库端口不是有效数字: {port_text}") from error
    args.user = prompt_value("数据库用户", args.user)
    args.target_db = prompt_value("新系统数据库名", args.target_db)

    env_password = os.getenv(args.password_env, "")
    hint = f"（回车使用环境变量 {args.password_env}）" if env_password else ""
    entered_password = prompt_value(f"数据库密码{hint}")
    return entered_password or env_password


def print_archive_summary(info: ArchiveInfo) -> None:
    print(f"archive={info.archive}")
    print(f"archive_sha256={info.sha256}")
    print(f"sql_member={info.sql_name}")
    total_rows = 0
    for table in info.tables:
        rows = info.estimated_rows.get(table, 0)
        total_rows += rows
        print(f"table={table} estimated_rows={rows}")
    print(f"estimated_total_rows={total_rows}")


def load_configured_encryption_keys() -> Tuple[Dict[str, bytes], str]:
    current_version = DEFAULT_ENCRYPTION_VERSION
    keys = parse_encryption_keys(DEFAULT_DATA_ENCRYPTION_KEYS, current_version)
    if current_version not in keys:
        raise MigrationError(
            f"当前版本 {current_version} 未在 DATA_ENCRYPTION_KEYS 中配置"
        )
    return keys, current_version


def configure_encryption_keys() -> Tuple[Dict[str, bytes], str]:
    configured_keys = DEFAULT_DATA_ENCRYPTION_KEYS
    configured_version = DEFAULT_ENCRYPTION_VERSION

    print("\n字段加密配置（已写入本地默认 v1 配置）：")
    current_version = normalize_encryption_version(
        prompt_value("当前加密版本 DATA_ENCRYPTION_CURRENT_VERSION", configured_version)
    )
    if not current_version:
        raise MigrationError("当前加密版本号无效")
    entered_keys = prompt_value("DATA_ENCRYPTION_KEYS（直接回车使用本地默认配置）")
    keys = parse_encryption_keys(entered_keys or configured_keys, current_version)
    if current_version not in keys:
        raise MigrationError(
            f"当前版本 {current_version} 未在 DATA_ENCRYPTION_KEYS 中配置"
        )
    print("已加载加密版本: " + ", ".join(sorted(keys)))
    return keys, current_version


def run_interactive_credential_repair(
    client: MySQLClient, target_db: str
) -> None:
    keys, current_version = configure_encryption_keys()
    print("\n[1/3] 扫描 accounts、exchange_rules 敏感字段")
    preview = repair_credential_fields(
        client, target_db, keys, current_version, apply=False
    )
    print_repair_summaries(preview)
    changed_rows = sum(item.changed_rows for item in preview)
    changed_fields = sum(item.changed_fields for item in preview)
    if changed_rows == 0:
        print("[3/3] 校验通过：所有敏感字段已经是当前加密版本，无需写入")
        return

    print(f"\n待修复 {changed_rows} 行、{changed_fields} 个字段。")
    confirmation = prompt_value("输入 REPAIR 确认重加密并提交")
    if confirmation != "REPAIR":
        print("已取消字段修复，数据库未写入。")
        return

    print("[2/3] 使用 AES-GCM 重加密并提交事务")
    applied = repair_credential_fields(
        client, target_db, keys, current_version, apply=True
    )
    print_repair_summaries(applied)
    print("[3/3] 重新扫描验证")
    verified = repair_credential_fields(
        client, target_db, keys, current_version, apply=False
    )
    remaining = sum(item.changed_fields for item in verified)
    if remaining:
        raise MigrationError(f"重加密后仍有 {remaining} 个字段需要修复")
    print("字段重加密校验通过；现在可以重启 API 和 Worker。")


def main(argv: Optional[Sequence[str]] = None) -> int:
    raw_args = list(sys.argv[1:] if argv is None else argv)
    args = build_parser().parse_args(raw_args)
    interactive = args.interactive or not raw_args

    if interactive:
        configure_interactive_archive_and_mode(args)
    elif args.archive is None and not args.repair_credentials:
        raise MigrationError("请使用 --archive 指定备份文件，或不带参数运行交互向导")
    if args.inspect_only and args.execute:
        raise MigrationError("--inspect-only 与 --execute 不能同时使用")

    if args.repair_credentials:
        password = (
            configure_interactive_database(args)
            if interactive else os.getenv(args.password_env, "")
        )
        target_db = args.target_db
        quote_identifier(target_db)
        client = MySQLClient(
            args.mysql_bin, args.host, args.port, args.user,
            password, args.connect_timeout,
        )
        validate_target_schema(client, target_db)
        if interactive:
            run_interactive_credential_repair(client, target_db)
        else:
            keys_raw = os.getenv("DATA_ENCRYPTION_KEYS", "")
            version = normalize_encryption_version(
                os.getenv("DATA_ENCRYPTION_CURRENT_VERSION", "")
            )
            if not version:
                raise MigrationError("未配置 DATA_ENCRYPTION_CURRENT_VERSION")
            keys = parse_encryption_keys(keys_raw, version)
            summaries = repair_credential_fields(
                client, target_db, keys, version, apply=args.execute
            )
            print_repair_summaries(summaries)
        return 0

    archive = args.archive.expanduser().resolve()
    with tempfile.TemporaryDirectory(prefix="caiyun-legacy-") as temporary:
        info, sql_path = extract_and_inspect_archive(archive, Path(temporary))
        require_expected_source_tables(info)
        if interactive:
            print("\n备份检查通过：")
            print_archive_summary(info)
        if args.inspect_only:
            if not interactive:
                print_archive_summary(info)
            return 0

        password = (
            configure_interactive_database(args)
            if interactive else os.getenv(args.password_env, "")
        )
        target_db = args.target_db
        quote_identifier(target_db)

        if interactive and args.execute:
            print("\n正式导入会向新系统数据库写入并提交全部旧数据。")
            confirmation = prompt_value("输入 IMPORT 确认正式导入")
            if confirmation != "IMPORT":
                print("已取消正式导入，数据库未连接。")
                return 0

        prefix = args.staging_prefix
        quote_identifier(prefix + "x")  # 校验前缀字符合法（前缀 + 任意字母仍是合法标识符）
        for _, target_table in TABLE_MAP:
            if target_table.startswith(prefix):
                raise MigrationError(
                    f"前缀 {prefix!r} 与目标表 {target_table} 冲突，请通过 --staging-prefix 换一个"
                )

        client = MySQLClient(
            args.mysql_bin, args.host, args.port, args.user,
            password, args.connect_timeout,
        )
        validate_target_schema(client, target_db)

        # 把备份里的旧表名统一加上前缀，使其在目标库内以隔离的临时表形式存在，
        # 避免与目标库已有的同名正式表冲突；也免去 CREATE DATABASE 权限。
        prefixed_sql_path = Path(temporary) / "legacy_prefixed.sql"
        sql_text = sql_path.read_text(encoding="utf-8")
        prefixed_sql = rewrite_source_table_names(sql_text, prefix, EXPECTED_SOURCE_TABLES)
        prefixed_sql = (
            "SET FOREIGN_KEY_CHECKS=0;\n"
            + prefixed_sql
            + "\nSET FOREIGN_KEY_CHECKS=1;\n"
        )
        prefixed_info = inspect_sql_text(prefixed_sql, archive, info.sql_name, info.sha256)
        leftover = sorted(t for t in prefixed_info.tables if not t.startswith(prefix))
        if leftover:
            raise MigrationError("改写后仍存在未加前缀的表: " + ", ".join(leftover))
        missing_prefixed = sorted(
            {prefix + table for table in EXPECTED_SOURCE_TABLES}
            - set(prefixed_info.tables)
        )
        if missing_prefixed:
            raise MigrationError("改写后缺少前缀表: " + ", ".join(missing_prefixed))
        prefixed_sql_path.write_text(prefixed_sql, encoding="utf-8", newline="\n")

        created_staging = False
        try:
            # 上一次若在建表中途失败，先清理残留的 _legacy_* 表再重试。
            drop_staging_tables(client, target_db, prefix)
            created_staging = True
            print(f"[1/5] 导入旧备份到目标库 {target_db}（中转表 {prefix}*，结束后自动删除）")
            client.run_file(prefixed_sql_path, target_db)
            target_tables = query_database_tables(client, target_db)
            missing_staging = sorted(
                {prefix + table for table in EXPECTED_SOURCE_TABLES} - target_tables
            )
            if missing_staging:
                raise MigrationError("导入后缺少前缀表: " + ", ".join(missing_staging))

            counts = source_row_counts(client, target_db, prefix)
            original_account_count = counts["accounts"]
            offsets = target_offsets(client, target_db)
            counts["accounts"] = expected_account_insert_count(
                client, target_db, target_db, prefix + "accounts", offsets["users"],
            )
            skipped_accounts = original_account_count - counts["accounts"]
            if skipped_accounts:
                print(
                    f"账号去重: 保留 {counts['accounts']} 条，跳过并合并引用 {skipped_accounts} 条"
                )

            original_cloud_stat_count = counts["cloud_stats"]
            counts["cloud_stats"] = expected_cloud_stat_insert_count(
                client, target_db, target_db, prefix + "cloud_stats",
                prefix + "accounts", offsets,
            )
            skipped_cloud_stats = original_cloud_stat_count - counts["cloud_stats"]
            if skipped_cloud_stats:
                print(
                    f"云朵统计去重: 保留 {counts['cloud_stats']} 条，跳过重复 {skipped_cloud_stats} 条"
                )

            print("[2/5] 生成字段映射、主键偏移和外键重映射 SQL")
            merge_sql = build_merge_sql(
                client, info, target_db, target_db, offsets, counts, args.execute, prefix,
            )
            plan = MigrationPlan(
                archive=info, staging_prefix=prefix, target_db=target_db,
                run_tag=info.sha256[:12], source_counts=counts,
                offsets=offsets, merge_sql=merge_sql,
            )
            sql_out, report_out = write_report(
                args.output_dir.resolve(), plan,
                "execute-pending" if args.execute else "dry-run-pending",
            )

            print("[3/5] 执行完整事务" + ("并提交" if args.execute else "演练并回滚"))
            output = client.run_script(merge_sql)
            migrated_counts = parse_migrated_counts(output)
            validate_migrated_counts(migrated_counts, counts)
            sql_out, report_out = write_report(
                args.output_dir.resolve(), plan,
                "success" if args.execute else "dry-run-success",
                migrated_counts,
            )
            print("[4/5] 逐表行数校验通过")
            print(f"merge_sql={sql_out}")
            print(f"report={report_out}")
            if args.execute:
                print("[5/5] 数据已提交")
                print("[迁移后 1/3] 自动扫描账号敏感字段")
                keys, current_version = load_configured_encryption_keys()
                preview = repair_credential_fields(
                    client, target_db, keys, current_version, apply=False
                )
                print_repair_summaries(preview)
                changed_fields = sum(item.changed_fields for item in preview)
                if changed_fields:
                    print("[迁移后 2/3] 自动重加密并提交")
                    applied = repair_credential_fields(
                        client, target_db, keys, current_version, apply=True
                    )
                    print_repair_summaries(applied)
                else:
                    print("[迁移后 2/3] 无待重加密字段")
                print("[迁移后 3/3] 重新扫描验证")
                verified = repair_credential_fields(
                    client, target_db, keys, current_version, apply=False
                )
                remaining = sum(item.changed_fields for item in verified)
                if remaining:
                    raise MigrationError(
                        f"迁移后字段重加密校验失败，仍有 {remaining} 个字段待处理"
                    )
                print("迁移与字段重加密全部完成；可以重启 API 和 Worker。")
            else:
                print("[5/5] 演练已回滚；确认报告后重新运行并选择 3 正式导入")
            return 0
        finally:
            if created_staging and not args.keep_staging:
                try:
                    drop_staging_tables(client, target_db, prefix)
                except Exception as cleanup_error:
                    print(f"警告: 清理中转表失败: {cleanup_error}", file=sys.stderr)
            elif created_staging:
                print(f"staging_tables_preserved={prefix}* in {target_db}")


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except MigrationError as error:
        print(f"迁移失败: {error}", file=sys.stderr)
        raise SystemExit(1)
