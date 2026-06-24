package handler

import (
	"encoding/json"
	"testing"
)

func FuzzValidateTargetURL(f *testing.F) {
	seeds := []string{
		"https://example.com/webhook",
		"http://localhost:8080/hook",
		"https://hooks.slack.com/services/T00/B00/xxx",
		"invalid-url",
		"ftp://not-allowed.com",
		"javascript:alert(1)",
		"http://",
		"",
		"http://192.168.1.1/webhook",
		"https://10.0.0.5:9000/hook",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, url string) {
		err := validateTargetURL(url)
		if err == nil {

			if len(url) > 0 && url != "" {
				t.Logf("accepted URL: %s", url)
			}
		}
	})
}

func FuzzCreateEndpointReq(f *testing.F) {
	f.Add(`{"url":"https://example.com/hook"}`)
	f.Add(`{"url":"invalid"}`)
	f.Add(`{"url":"","slack_webhook_url":"https://hooks.slack.com"}`)
	f.Add(`{"url":"https://example.com","email":"not-an-email"}`)
	f.Add(`{"url":"https://example.com","allowed_event_types":["user.created","order.paid"]}`)
	f.Add(`not-json-at-all`)
	f.Add(``)
	f.Add(`{"url":"https://example.com","email":"user@example.com"}`)

	f.Fuzz(func(t *testing.T, body string) {
		var req createEndpointReq
		err := json.Unmarshal([]byte(body), &req)
		if err == nil {
			if req.URL != "" {
				validateTargetURL(req.URL)
			}
		}
	})
}
