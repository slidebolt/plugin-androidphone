package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const firebaseMessagingScope = "https://www.googleapis.com/auth/firebase.messaging"

type pushNotification struct {
	Title string `json:"title,omitempty"`
	Body  string `json:"body,omitempty"`
	Image string `json:"image,omitempty"`
}

type outboundPush struct {
	Notification *pushNotification `json:"notification,omitempty"`
	Data         map[string]string `json:"data,omitempty"`
}

type pushSender interface {
	Send(ctx context.Context, registrationToken string, msg outboundPush) error
}

type fcmSender struct {
	projectID string
	baseURL   string
	client    *http.Client
	tokens    oauth2.TokenSource
}

func newFCMSender(cfg Config) (pushSender, error) {
	if cfg.ProjectID == "" || cfg.CredentialsJSON == "" {
		return nil, nil
	}

	jwtCfg, err := google.JWTConfigFromJSON([]byte(cfg.CredentialsJSON), firebaseMessagingScope)
	if err != nil {
		return nil, fmt.Errorf("parse credentials json: %w", err)
	}

	return &fcmSender{
		projectID: cfg.ProjectID,
		baseURL:   strings.TrimRight(cfg.BaseURL, "/"),
		client:    &http.Client{Timeout: 15 * time.Second},
		tokens:    jwtCfg.TokenSource(context.Background()),
	}, nil
}

func (s *fcmSender) Send(ctx context.Context, registrationToken string, msg outboundPush) error {
	accessToken, err := s.tokens.Token()
	if err != nil {
		return fmt.Errorf("get access token: %w", err)
	}

	body := map[string]any{
		"message": map[string]any{
			"token": registrationToken,
			"data":  msg.Data,
		},
	}
	if msg.Notification != nil {
		body["message"].(map[string]any)["notification"] = msg.Notification
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal fcm request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/projects/%s/messages:send", s.baseURL, s.projectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build fcm request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken.AccessToken)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("post fcm request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("fcm send failed: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
