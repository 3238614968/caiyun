package tasks

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Storage 存储接口
// 用于在同一账号的一批任务之间共享中间态。
type Storage interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Delete(key string) error
}

// FileStore 文件存储
type FileStore struct {
	basePath string
	data     map[string]string
	mu       sync.RWMutex
	logger   Logger
}

// MemoryStore 内存存储
type MemoryStore struct {
	data map[string]string
	mu   sync.RWMutex
}

// ScopedStorage 为底层存储添加命名空间，避免不同账号串数据。
type ScopedStorage struct {
	base   Storage
	prefix string
}

// Logger 日志接口
type Logger interface {
	Debug(args ...interface{})
	Error(args ...interface{})
}

// NewFileStore 创建文件存储
func NewFileStore(basePath string, logger Logger) *FileStore {
	if abs, err := filepath.Abs(basePath); err == nil {
		basePath = abs
	}
	store := &FileStore{
		basePath: basePath,
		data:     make(map[string]string),
		logger:   logger,
	}

	if err := os.MkdirAll(basePath, 0755); err != nil && logger != nil {
		logger.Error("创建存储目录失败", err)
	}

	store.load()
	return store
}

// NewMemoryStore 创建内存存储
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: make(map[string]string)}
}

// NewScopedStorage 创建带命名空间的存储
func NewScopedStorage(base Storage, prefix string) Storage {
	if base == nil {
		return NewMemoryStore()
	}
	return &ScopedStorage{
		base:   base,
		prefix: strings.Trim(strings.TrimSpace(prefix), ":"),
	}
}

// load 加载数据
func (s *FileStore) load() {
	files, err := os.ReadDir(s.basePath)
	if err != nil {
		if s.logger != nil {
			s.logger.Debug("读取存储目录失败", err)
		}
		return
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		key, err := fileStoreKeyFromDiskName(file.Name())
		if err != nil {
			if s.logger != nil {
				s.logger.Debug("跳过非法存储文件名", file.Name(), err)
			}
			continue
		}

		filePath := filepath.Join(s.basePath, file.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			if s.logger != nil {
				s.logger.Debug("读取文件失败", filePath, err)
			}
			continue
		}

		s.data[key] = string(data)
	}
}

// Get 获取值
func (s *FileStore) Get(key string) (string, error) {
	if _, err := fileStoreDiskName(key); err != nil {
		return "", err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	value, ok := s.data[key]
	if !ok {
		return "", fmt.Errorf("未找到 key: %s", key)
	}
	return value, nil
}

// Set 设置值
func (s *FileStore) Set(key, value string) error {
	diskName, err := fileStoreDiskName(key)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.data[key] = value
	filePath := filepath.Join(s.basePath, diskName)
	if err := os.WriteFile(filePath, []byte(value), 0644); err != nil {
		return fmt.Errorf("保存文件失败: %w", err)
	}
	return nil
}

// Delete 删除值
func (s *FileStore) Delete(key string) error {
	diskName, err := fileStoreDiskName(key)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.data, key)
	filePath := filepath.Join(s.basePath, diskName)
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除文件失败: %w", err)
	}
	return nil
}

func fileStoreDiskName(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", fmt.Errorf("存储 key 不能为空")
	}
	if strings.ContainsRune(key, '\x00') || filepath.IsAbs(key) || filepath.VolumeName(key) != "" {
		return "", fmt.Errorf("非法存储 key: %q", key)
	}
	clean := filepath.Clean(key)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) || strings.Contains(clean, string(os.PathSeparator)+".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("非法存储 key: %q", key)
	}
	return base64.RawURLEncoding.EncodeToString([]byte(key)) + ".kv", nil
}

func fileStoreKeyFromDiskName(name string) (string, error) {
	if strings.TrimSpace(name) == "" || filepath.Base(name) != name {
		return "", fmt.Errorf("非法文件名")
	}
	if !strings.HasSuffix(name, ".kv") {
		return name, nil
	}
	raw := strings.TrimSuffix(name, ".kv")
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

// Get 获取值
func (s *MemoryStore) Get(key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	value, ok := s.data[key]
	if !ok {
		return "", fmt.Errorf("未找到 key: %s", key)
	}
	return value, nil
}

// Set 设置值
func (s *MemoryStore) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data == nil {
		s.data = make(map[string]string)
	}
	s.data[key] = value
	return nil
}

// Delete 删除值
func (s *MemoryStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}

// Get 获取带命名空间的值
func (s *ScopedStorage) Get(key string) (string, error) {
	return s.base.Get(s.scopeKey(key))
}

// Set 设置带命名空间的值
func (s *ScopedStorage) Set(key, value string) error {
	return s.base.Set(s.scopeKey(key), value)
}

// Delete 删除带命名空间的值
func (s *ScopedStorage) Delete(key string) error {
	return s.base.Delete(s.scopeKey(key))
}

func (s *ScopedStorage) scopeKey(key string) string {
	if s == nil || strings.TrimSpace(s.prefix) == "" {
		return key
	}
	return s.prefix + ":" + key
}

// 存储键常量
const (
	KeyUserID     = "user_id"
	KeyAICloudNum = "ai_cloud_num"
	KeyAISessions = "ai_sessions"
	KeyTempFiles  = "temp_files"
	KeyTempLinks  = "temp_share_links"
)

// ResetKeys 删除一组共享状态键。
func ResetKeys(store Storage, keys ...string) {
	if store == nil {
		return
	}
	for _, key := range keys {
		if strings.TrimSpace(key) == "" {
			continue
		}
		_ = store.Delete(key)
	}
}

// LoadAISessions 加载 AI 会话
func LoadAISessions(store Storage) ([]AISession, error) {
	if store == nil {
		return nil, nil
	}

	data, err := store.Get(KeyAISessions)
	if err != nil {
		return nil, nil
	}

	var sessions []AISession
	if err := json.Unmarshal([]byte(data), &sessions); err != nil {
		return nil, err
	}
	return sessions, nil
}

// SaveAISessions 保存 AI 会话
func SaveAISessions(store Storage, sessions []AISession) error {
	if store == nil {
		return nil
	}

	data, err := json.Marshal(sessions)
	if err != nil {
		return fmt.Errorf("序列化 AI 会话失败: %w", err)
	}
	return store.Set(KeyAISessions, string(data))
}

// LoadStringList 加载字符串数组（不存在时返回空数组）
func LoadStringList(store Storage, key string) ([]string, error) {
	if store == nil {
		return []string{}, nil
	}

	data, err := store.Get(key)
	if err != nil || data == "" {
		return []string{}, nil
	}

	var list []string
	if err := json.Unmarshal([]byte(data), &list); err != nil {
		return nil, err
	}
	return list, nil
}

// SaveStringList 保存字符串数组（空数组时删除键）
func SaveStringList(store Storage, key string, list []string) error {
	if store == nil {
		return nil
	}

	uniq := make([]string, 0, len(list))
	seen := make(map[string]bool)
	for _, item := range list {
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		uniq = append(uniq, item)
	}

	if len(uniq) == 0 {
		if err := store.Delete(key); err != nil {
			return nil
		}
		return nil
	}

	data, err := json.Marshal(uniq)
	if err != nil {
		return fmt.Errorf("序列化字符串数组失败: %w", err)
	}
	return store.Set(key, string(data))
}

// AppendStringList 追加字符串到数组存储（自动去重）
func AppendStringList(store Storage, key string, items ...string) error {
	if store == nil || len(items) == 0 {
		return nil
	}

	list, err := LoadStringList(store, key)
	if err != nil {
		return err
	}
	list = append(list, items...)
	return SaveStringList(store, key, list)
}

// RemoveStringList 删除数组中的指定元素
func RemoveStringList(store Storage, key string, items ...string) error {
	if store == nil || len(items) == 0 {
		return nil
	}

	list, err := LoadStringList(store, key)
	if err != nil {
		return err
	}

	removeSet := make(map[string]bool, len(items))
	for _, item := range items {
		if item != "" {
			removeSet[item] = true
		}
	}

	next := make([]string, 0, len(list))
	for _, item := range list {
		if !removeSet[item] {
			next = append(next, item)
		}
	}
	return SaveStringList(store, key, next)
}
