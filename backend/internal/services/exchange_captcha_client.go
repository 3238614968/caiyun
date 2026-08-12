// Exchange captcha client isolates slide acquisition, decoding and solver retries
// from the exchange command orchestration.
package services

import (
	"caiyun/internal/core/sms"
	"caiyun/internal/utils"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type exchangeSlidePayload struct {
	puzzle      string
	picture     string
	picWidth    int
	picHeight   int
	puzzleWidth int
}

func obtainExchangeSlideOffset(session *exchangeHTTPSession, authCtx *exchangeAuthContext) (int, string, error) {
	return obtainExchangeSlideOffsetContext(context.Background(), session, authCtx)
}

func obtainExchangeSlideOffsetContext(ctx context.Context, session *exchangeHTTPSession, authCtx *exchangeAuthContext) (int, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var lastErr error
	for attempt := 1; attempt <= exchangeSlideMaxAttempt; attempt++ {
		payload, err := fetchExchangeSlideContext(ctx, session, authCtx)
		if err != nil {
			lastErr = err
			if err := sleepExchangeContext(ctx, time.Duration(attempt)*200*time.Millisecond); err != nil {
				return 0, "", err
			}
			continue
		}

		result, err := sms.SolveSlideContext(ctx, payload.puzzle, payload.picture)
		if err != nil {
			lastErr = err
			if err := sleepExchangeContext(ctx, time.Duration(attempt)*200*time.Millisecond); err != nil {
				return 0, "", err
			}
			continue
		}
		if result == nil {
			lastErr = fmt.Errorf("识别接口返回为空")
			continue
		}
		info := fmt.Sprintf("滑块识别offset=%d", result.Offset)
		if result.Confidence > 0 {
			info += fmt.Sprintf(" confidence=%.4f", result.Confidence)
		}
		if result.Method != "" {
			info += " method=" + result.Method
		}
		return result.Offset, info, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("识别失败")
	}
	return 0, "", lastErr
}

func fetchExchangeSlide(session *exchangeHTTPSession, authCtx *exchangeAuthContext) (*exchangeSlidePayload, error) {
	return fetchExchangeSlideContext(context.Background(), session, authCtx)
}

func fetchExchangeSlideContext(ctx context.Context, session *exchangeHTTPSession, authCtx *exchangeAuthContext) (*exchangeSlidePayload, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if session == nil || session.client == nil {
		return nil, fmt.Errorf("HTTP 会话为空")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://m.mcloud.139.com/ycloud/auth-service/slide/getSlide", strings.NewReader(""))
	if err != nil {
		return nil, err
	}
	req.Host = "m.mcloud.139.com"
	for key, value := range buildExchangeHeaders(authCtx, session, map[string]string{
		"Content-Type": "application/x-www-form-urlencoded;charset=UTF-8",
	}) {
		req.Header[key] = []string{value}
	}

	resp, err := session.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("获取滑块验证码请求失败: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := utils.ReadLimitedBody(resp.Body, utils.DefaultMaxResponseBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("读取滑块验证码响应失败: %w", err)
	}
	body := string(bodyBytes)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("获取滑块验证码 HTTP %d: %s", resp.StatusCode, summarizeExchangeBody(body))
	}

	payload, err := decodeExchangeSlideResponse(bodyBytes)
	if err != nil {
		return nil, fmt.Errorf("解析滑块验证码响应失败: %w | body=%s", err, summarizeExchangeBody(body))
	}
	return payload, nil
}

func sleepExchangeContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func decodeExchangeSlideResponse(bodyBytes []byte) (*exchangeSlidePayload, error) {
	var raw struct {
		Code    int    `json:"code"`
		Msg     string `json:"msg"`
		Message string `json:"message"`
		Result  struct {
			Puzzle      string      `json:"puzzle"`
			Picture     string      `json:"picture"`
			PicWidth    interface{} `json:"picWidth"`
			PicHeight   interface{} `json:"picHeight"`
			PuzzleWidth interface{} `json:"puzzleWidth"`
		} `json:"result"`
	}
	if err := json.Unmarshal(bodyBytes, &raw); err != nil {
		return nil, err
	}
	if raw.Code != 0 {
		msg := strings.TrimSpace(raw.Msg)
		if msg == "" {
			msg = strings.TrimSpace(raw.Message)
		}
		if msg == "" {
			msg = summarizeExchangeBody(string(bodyBytes))
		}
		return nil, fmt.Errorf("获取滑块验证码失败: code=%d msg=%s", raw.Code, msg)
	}
	if strings.TrimSpace(raw.Result.Puzzle) == "" || strings.TrimSpace(raw.Result.Picture) == "" {
		return nil, fmt.Errorf("获取滑块验证码失败: 图片数据为空")
	}
	return &exchangeSlidePayload{
		puzzle:      raw.Result.Puzzle,
		picture:     raw.Result.Picture,
		picWidth:    exchangeIntFromAny(raw.Result.PicWidth, 680),
		picHeight:   exchangeIntFromAny(raw.Result.PicHeight, 400),
		puzzleWidth: exchangeIntFromAny(raw.Result.PuzzleWidth, 96),
	}, nil
}

func exchangeIntFromAny(value interface{}, fallback int) int {
	switch v := value.(type) {
	case nil:
		return fallback
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return int(n)
		}
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return fallback
		}
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
