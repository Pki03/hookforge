package database

import "time"

type Endpoint struct {
	ID                 string    `json:"id"`
	URL                string    `json:"url"`
	Secret             string    `json:"-"`
	SlackWebhookURL    string    `json:"-"`
	RateLimitPerSecond int       `json:"rate_limit_per_second"`
	RateLimitBurst     int       `json:"rate_limit_burst"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type EndpointResponse struct {
	ID                 string    `json:"id"`
	URL                string    `json:"url"`
	Secret             string    `json:"secret,omitempty"`
	RateLimitPerSecond int       `json:"rate_limit_per_second"`
	RateLimitBurst     int       `json:"rate_limit_burst"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type DeliveryAttempt struct {
	ID           string     `json:"id"`
	EventID      string     `json:"event_id"`
	AttemptNum   int        `json:"attempt_num"`
	StatusCode   *int       `json:"status_code,omitempty"`
	ResponseBody *string    `json:"response_body,omitempty"`
	ErrorMessage *string    `json:"error_message,omitempty"`
	DurationMs   int        `json:"duration_ms"`
	AttemptedAt  time.Time  `json:"attempted_at"`
}

type Event struct {
	ID          string     `json:"id"`
	EndpointID  string     `json:"endpoint_id"`
	Payload     []byte     `json:"payload"`
	Status      string     `json:"status"`
	Attempts    int        `json:"attempts"`
	MaxRetries  int        `json:"max_retries"`
	NextRetryAt *time.Time `json:"next_retry_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}
