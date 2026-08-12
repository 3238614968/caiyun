package services

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"net"
	"net/smtp"
	"strings"
	"time"

	"caiyun/internal/models"
	"caiyun/internal/security/authcache"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// SendPasswordResetCode 向用户注册邮箱发送密码重置验证码。
func (s *AuthService) SendPasswordResetCode(username, email string) error {
	if !s.isEmailServiceEnabled() {
		return ErrEmailServiceDisabled
	}

	user, err := s.findPasswordResetUser(username, email)
	if err != nil {
		// 不暴露用户名/邮箱是否存在，避免找回入口被用于枚举账号。
		return nil
	}

	key := passwordResetKey(user.Username, user.Email)
	now := time.Now()
	if s.resetCodeCache == nil {
		return ErrEmailServiceDisabled
	}

	var existing passwordResetCodeRecord
	if err := s.resetCodeCache.Get(key, &existing); err == nil && now.Sub(existing.SentAt) < s.resetConfig.SendCooldown {
		return ErrResetCodeTooFrequent
	}

	code, err := generateResetCode()
	if err != nil {
		return err
	}
	codeHash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	if err := s.sendPasswordResetEmail(user.Email, user.Username, code); err != nil {
		return err
	}

	record := &passwordResetCodeRecord{
		CodeHash:  string(codeHash),
		ExpiresAt: now.Add(s.resetConfig.CodeTTL),
		SentAt:    now,
		Attempts:  0,
	}
	return s.resetCodeCache.Set(key, record, s.resetConfig.CodeTTL)
}

// ResetPasswordWithCode 通过邮箱验证码重置密码。
func (s *AuthService) ResetPasswordWithCode(username, email, code, newPassword string) error {
	user, err := s.findPasswordResetUser(username, email)
	if err != nil {
		return ErrInvalidRecoveryInfo
	}
	key := passwordResetKey(user.Username, user.Email)
	release, err := s.acquirePasswordResetLock(key)
	if err != nil {
		return err
	}
	defer release()

	if err := s.verifyPasswordResetCode(user.Username, user.Email, code); err != nil {
		return err
	}
	if err := validatePasswordStrength(user.Username, newPassword); err != nil {
		return err
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := s.userRepo.UpdatePasswordAndRevokeSessions(user.ID, string(hashedPassword)); err != nil {
		return err
	}
	// 重置密码后立即失效缓存中的用户快照。
	authcache.Delete(user.ID)

	if s.resetCodeCache != nil {
		_ = s.resetCodeCache.Del(passwordResetKey(user.Username, user.Email))
	}
	return nil
}

const passwordResetLockTTL = 2 * time.Minute

func passwordResetLockKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return "caiyun:password_reset_lock:" + hex.EncodeToString(sum[:])
}

func (s *AuthService) acquirePasswordResetLock(key string) (func(), error) {
	s.resetCodeMu.Lock()

	lockCache, ok := s.resetCodeCache.(passwordResetLockCache)
	if !ok {
		return func() { s.resetCodeMu.Unlock() }, nil
	}

	lockKey := passwordResetLockKey(key)
	lockValue := uuid.NewString()
	acquired, err := lockCache.SetNX(lockKey, lockValue, passwordResetLockTTL)
	if err != nil || !acquired {
		s.resetCodeMu.Unlock()
		return nil, ErrInvalidResetCode
	}

	return func() {
		_, _ = lockCache.DelIfValue(lockKey, lockValue)
		s.resetCodeMu.Unlock()
	}, nil
}

func (s *AuthService) findPasswordResetUser(username, email string) (*models.User, error) {
	user, err := s.userRepo.FindByUsername(strings.TrimSpace(username))
	if err != nil {
		return nil, ErrInvalidRecoveryInfo
	}
	if user.Email == "" || !strings.EqualFold(strings.TrimSpace(user.Email), strings.TrimSpace(email)) {
		return nil, ErrInvalidRecoveryInfo
	}
	return user, nil
}

func (s *AuthService) verifyPasswordResetCode(username, email, code string) error {
	key := passwordResetKey(username, email)
	now := time.Now()
	normalizedCode := strings.TrimSpace(code)
	if normalizedCode == "" {
		return ErrInvalidResetCode
	}
	if s.resetCodeCache == nil {
		return ErrInvalidResetCode
	}

	var record passwordResetCodeRecord
	if err := s.resetCodeCache.Get(key, &record); err != nil || now.After(record.ExpiresAt) || record.Attempts >= s.resetConfig.MaxAttempts {
		_ = s.resetCodeCache.Del(key)
		return ErrInvalidResetCode
	}
	record.Attempts++
	codeHash := record.CodeHash

	if err := bcrypt.CompareHashAndPassword([]byte(codeHash), []byte(normalizedCode)); err != nil {
		remainingTTL := time.Until(record.ExpiresAt)
		if remainingTTL <= 0 || record.Attempts >= s.resetConfig.MaxAttempts {
			_ = s.resetCodeCache.Del(key)
		} else {
			_ = s.resetCodeCache.Set(key, &record, remainingTTL)
		}
		return ErrInvalidResetCode
	}
	return nil
}

func generateResetCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func (s *AuthService) sendPasswordResetEmail(to, username, code string) error {
	smtpConfig := s.resetConfig.SMTP
	from := strings.TrimSpace(smtpConfig.From)
	fromName := strings.TrimSpace(smtpConfig.FromName)
	if fromName == "" {
		fromName = "移动云盘"
	}

	// 过滤用户名中的控制字符（CR/LF 等），避免邮件头/正文注入。
	safeUsername := sanitizeEmailText(username)

	subject := "移动云盘密码重置验证码"
	body := fmt.Sprintf("你好，%s：\n\n你的移动云盘密码重置验证码为：%s\n验证码 %d 分钟内有效，请勿转发给他人。\n\n如果不是你本人操作，请忽略本邮件。",
		safeUsername, code, int(s.resetConfig.CodeTTL.Minutes()))
	message := buildEmailMessage(from, fromName, to, subject, body)
	return sendSMTPMail(smtpConfig, from, []string{to}, []byte(message))
}

// sanitizeEmailText 过滤字符串中的控制字符（特别是 CR/LF），防止 SMTP 头/正文注入。
func sanitizeEmailText(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r < 0x20 {
			return -1
		}
		return r
	}, s)
}

func buildEmailMessage(from, fromName, to, subject, body string) string {
	encodedSubject := "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(subject)) + "?="
	encodedFromName := "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(fromName)) + "?="
	return fmt.Sprintf("From: %s <%s>\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s",
		encodedFromName, from, to, encodedSubject, body)
}

func sendSMTPMail(config SMTPConfig, from string, to []string, msg []byte) error {
	addr := net.JoinHostPort(config.Host, config.Port)
	var auth smtp.Auth
	if config.Username != "" {
		auth = smtp.PlainAuth("", config.Username, config.Password, config.Host)
	}

	const smtpTimeout = 10 * time.Second
	deadline := time.Now().Add(smtpTimeout)

	if config.UseTLS {
		dialer := &net.Dialer{Timeout: smtpTimeout}
		conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{
			ServerName: config.Host,
			MinVersion: tls.VersionTLS12,
		})
		if err != nil {
			return err
		}
		_ = conn.SetDeadline(deadline)
		defer conn.Close()

		client, err := smtp.NewClient(conn, config.Host)
		if err != nil {
			return err
		}
		defer client.Quit()
		if auth != nil {
			if err := client.Auth(auth); err != nil {
				return err
			}
		}
		if err := client.Mail(from); err != nil {
			return err
		}
		for _, recipient := range to {
			if err := client.Rcpt(recipient); err != nil {
				return err
			}
		}
		writer, err := client.Data()
		if err != nil {
			return err
		}
		if _, err := writer.Write(msg); err != nil {
			_ = writer.Close()
			return err
		}
		return writer.Close()
	}

	dialer := &net.Dialer{Timeout: smtpTimeout}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return err
	}
	_ = conn.SetDeadline(deadline)
	defer conn.Close()

	client, err := smtp.NewClient(conn, config.Host)
	if err != nil {
		return err
	}
	defer client.Quit()

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{
			ServerName: config.Host,
			MinVersion: tls.VersionTLS12,
		}); err != nil {
			return err
		}
	}
	if auth != nil {
		if ok, _ := client.Extension("AUTH"); ok {
			if err := client.Auth(auth); err != nil {
				return err
			}
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, recipient := range to {
		if err := client.Rcpt(recipient); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(msg); err != nil {
		_ = writer.Close()
		return err
	}
	return writer.Close()
}
