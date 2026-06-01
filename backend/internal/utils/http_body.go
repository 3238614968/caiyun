package utils

import (
	"errors"
	"fmt"
	"io"
)

const DefaultMaxResponseBodyBytes int64 = 5 << 20 // 5 MiB

var ErrResponseBodyTooLarge = errors.New("响应体超过大小限制")

// ReadLimitedBody 读取外部 HTTP 响应体，并在超过 maxBytes 时返回错误。
func ReadLimitedBody(reader io.Reader, maxBytes int64) ([]byte, error) {
	if reader == nil {
		return nil, nil
	}
	if maxBytes <= 0 {
		maxBytes = DefaultMaxResponseBodyBytes
	}

	body, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("%w: limit=%d", ErrResponseBodyTooLarge, maxBytes)
	}
	return body, nil
}
