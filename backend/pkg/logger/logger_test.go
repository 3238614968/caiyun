package logger

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewRejectsInvalidFileConfig(t *testing.T) {
	if _, err := New(nil); err == nil {
		t.Fatal("New(nil) should fail")
	}
	_, err := New(&Config{Level: InfoLevel, OutputPath: filepath.Join(t.TempDir(), "app.log")})
	if err == nil || !strings.Contains(err.Error(), "LOG_MAX_SIZE") {
		t.Fatalf("invalid max size error = %v", err)
	}
}

func TestRotateCompressesBackupAndKeepsActiveFile(t *testing.T) {
	dir := t.TempDir()
	active := filepath.Join(dir, "app.log")
	const oldContent = "before rotation\n"
	if err := os.WriteFile(active, []byte(oldContent), 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(active, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	hook := &rotateHook{
		maxSize: 1, maxBackups: 5, maxAge: 30, compress: true,
		filename: active, file: file, size: int64(len(oldContent)),
	}
	if err := hook.rotate(); err != nil {
		t.Fatalf("rotate() error = %v", err)
	}
	defer hook.file.Close()

	activeInfo, err := os.Stat(active)
	if err != nil {
		t.Fatalf("active log missing after rotation: %v", err)
	}
	if activeInfo.Size() != 0 {
		t.Fatalf("active log size = %d, want 0", activeInfo.Size())
	}

	backups, err := filepath.Glob(filepath.Join(dir, "app.*.log.gz"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("compressed backups = %v, err = %v", backups, err)
	}
	compressed, err := os.Open(backups[0])
	if err != nil {
		t.Fatal(err)
	}
	reader, err := gzip.NewReader(compressed)
	if err != nil {
		_ = compressed.Close()
		t.Fatal(err)
	}
	payload, err := io.ReadAll(reader)
	_ = reader.Close()
	_ = compressed.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != oldContent {
		t.Fatalf("compressed content = %q, want %q", payload, oldContent)
	}
	if _, err := os.Stat(strings.TrimSuffix(backups[0], ".gz")); !os.IsNotExist(err) {
		t.Fatalf("plain rotated log should be removed, stat err = %v", err)
	}

	if _, err := hook.file.WriteString("active\n"); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(active); err != nil || string(got) != "active\n" {
		t.Fatalf("active log content = %q, err = %v", got, err)
	}
}

func TestNewAppliesMaxAgeWithoutDeletingActiveFile(t *testing.T) {
	dir := t.TempDir()
	active := filepath.Join(dir, "service.log")
	expired := filepath.Join(dir, "service.20200101T000000.000000000Z.log.gz")
	recent := filepath.Join(dir, "service.20990101T000000.000000000Z.log")
	for path, body := range map[string]string{active: "active", expired: "expired", recent: "recent"} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-10 * 24 * time.Hour)
	if err := os.Chtimes(active, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(expired, old, old); err != nil {
		t.Fatal(err)
	}

	instance, err := New(&Config{
		Level: InfoLevel, OutputPath: active, MaxSize: 1,
		MaxBackups: 10, MaxAge: 7, Compress: true,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer instance.Close()

	if _, err := os.Stat(expired); !os.IsNotExist(err) {
		t.Fatalf("expired backup should be removed, stat err = %v", err)
	}
	if got, err := os.ReadFile(active); err != nil || string(got) != "active" {
		t.Fatalf("active log was changed: content=%q err=%v", got, err)
	}
	if _, err := os.Stat(recent); err != nil {
		t.Fatalf("recent backup should remain: %v", err)
	}
}

func TestCleanOldBackupsCountsPlainAndCompressedFiles(t *testing.T) {
	dir := t.TempDir()
	active := filepath.Join(dir, "app.log")
	if err := os.WriteFile(active, []byte("active"), 0o644); err != nil {
		t.Fatal(err)
	}
	backups := []string{
		filepath.Join(dir, "app.1.log"),
		filepath.Join(dir, "app.2.log.gz"),
		filepath.Join(dir, "app.3.log"),
	}
	for i, path := range backups {
		if err := os.WriteFile(path, []byte("backup"), 0o644); err != nil {
			t.Fatal(err)
		}
		when := time.Now().Add(time.Duration(i-3) * time.Hour)
		if err := os.Chtimes(path, when, when); err != nil {
			t.Fatal(err)
		}
	}
	hook := &rotateHook{filename: active, maxBackups: 2}
	if err := hook.cleanOldBackups(); err != nil {
		t.Fatalf("cleanOldBackups() error = %v", err)
	}
	if _, err := os.Stat(backups[0]); !os.IsNotExist(err) {
		t.Fatalf("oldest backup should be removed, stat err = %v", err)
	}
	for _, path := range backups[1:] {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("newer backup %s should remain: %v", path, err)
		}
	}
	if _, err := os.Stat(active); err != nil {
		t.Fatalf("active log should remain: %v", err)
	}
}
