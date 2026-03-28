package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/oauth2"
)

func TestFCMSender_SendBuildsHTTPV1Request(t *testing.T) {
	var gotAuth string
	var gotPath string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"name":"projects/demo/messages/123"}`))
	}))
	defer srv.Close()

	sender := &fcmSender{
		projectID: "demo-project",
		baseURL:   srv.URL,
		client:    srv.Client(),
		tokens:    oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "test-token"}),
	}

	err := sender.Send(context.Background(), "device-token", outboundPush{
		Notification: &pushNotification{Title: "Doorbell", Body: "Front door"},
		Data:         map[string]string{"route": "camera/front"},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if gotAuth != "Bearer test-token" {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if gotPath != "/v1/projects/demo-project/messages:send" {
		t.Fatalf("path = %q", gotPath)
	}

	msg, ok := gotBody["message"].(map[string]any)
	if !ok {
		t.Fatalf("message body = %#v", gotBody["message"])
	}
	if msg["token"] != "device-token" {
		t.Fatalf("message token = %#v", msg["token"])
	}
	notification, ok := msg["notification"].(map[string]any)
	if !ok {
		t.Fatalf("notification = %#v", msg["notification"])
	}
	if notification["title"] != "Doorbell" || notification["body"] != "Front door" {
		t.Fatalf("notification = %#v", notification)
	}
	data, ok := msg["data"].(map[string]any)
	if !ok {
		t.Fatalf("data = %#v", msg["data"])
	}
	if data["route"] != "camera/front" {
		t.Fatalf("data route = %#v", data["route"])
	}
}
