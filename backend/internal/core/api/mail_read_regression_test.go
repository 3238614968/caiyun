package api

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
)

func TestRead139MailMessageRejectsUnconfirmedResponses(t *testing.T) {
	for _, body := range []string{"<html>session expired</html>", `{}`, `{"code":"FAILED"}`} {
		t.Run(body, func(t *testing.T) {
			client := corehttp.NewClient(corehttp.WithTransport(rewardRetryTransport(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/RmWeb/mail" || req.URL.Query().Get("func") != "mbox:readMessage" {
					return nil, errors.New("unexpected request")
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
			})))
			if err := NewCaiyunAPI(client).Read139MailMessage(&Mail139Session{SID: "session", RMKey: "rmkey"}, "mail-id"); err == nil {
				t.Fatalf("unconfirmed response accepted: %s", body)
			}
		})
	}
}
