package worker

import (
	"testing"
)

func FuzzSSRFCheck(f *testing.F) {
	seeds := []string{
		"https://example.com/webhook",
		"http://localhost:8080/hook",
		"http://127.0.0.1:9000/test",
		"https://192.168.1.1/admin",
		"http://10.0.0.1:5432",
		"ftp://evil.com",
		"javascript:alert(1)",
		"http://[::1]:8080",
		"https://hooks.slack.com/services/T00/B00/xxx",
		"",
		"not-a-url",
		"http://169.254.169.254/latest/meta-data/",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, rawURL string) {
		ssrfCheck(rawURL)
	})
}
