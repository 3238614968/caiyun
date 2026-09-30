package services

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"
)

type exchangeWarmSession struct {
	accountID uint
	prepared  *exchangePreparedSession
	expiresAt time.Time
}

func cloneExchangeWarmSession(base *exchangePreparedSession) *exchangePreparedSession {
	jar, _ := cookiejar.New(nil)
	endpoint, _ := url.Parse("https://m.mcloud.139.com")
	if base.http.client.Jar != nil {
		jar.SetCookies(endpoint, base.http.client.Jar.Cookies(endpoint))
	}
	client := &http.Client{Timeout: base.http.client.Timeout, Transport: base.http.client.Transport, Jar: jar}
	session := *base.http
	session.client = client
	session.captchaTID = ""
	return &exchangePreparedSession{auth: base.auth, http: &session, sourceAuth: base.sourceAuth}
}

func (s *ExchangeScheduler) storeWarmSession(taskID, accountID uint, prepared *exchangePreparedSession) {
	s.queueMutex.Lock()
	defer s.queueMutex.Unlock()
	if s.warmSessions == nil {
		s.warmSessions = make(map[uint]exchangeWarmSession)
	}
	for key, item := range s.warmSessions {
		if time.Now().After(item.expiresAt) {
			delete(s.warmSessions, key)
		}
	}
	s.warmSessions[taskID] = exchangeWarmSession{accountID: accountID, prepared: prepared, expiresAt: time.Now().Add(2 * time.Minute)}
}

func (s *ExchangeScheduler) takeWarmSession(taskID, accountID uint, auth string) *exchangePreparedSession {
	s.queueMutex.Lock()
	defer s.queueMutex.Unlock()
	entry, ok := s.warmSessions[taskID]
	delete(s.warmSessions, taskID)
	if !ok || entry.accountID != accountID || time.Now().After(entry.expiresAt) {
		return &exchangePreparedSession{}
	}
	if entry.prepared.sourceAuth != "" && sanitizeAuthValue(entry.prepared.sourceAuth) != sanitizeAuthValue(auth) {
		return &exchangePreparedSession{}
	}
	return entry.prepared
}

func warmExchangePortal(ctx context.Context, session *exchangeHTTPSession, auth *exchangeAuthContext) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, session.referer, nil)
	if err != nil {
		return
	}
	for key, value := range buildExchangeHeaders(auth, session, nil) {
		req.Header.Set(key, value)
	}
	resp, err := session.client.Do(req)
	if err == nil && resp != nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
	}
}
