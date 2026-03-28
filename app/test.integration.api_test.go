package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	apiapp "github.com/slidebolt/sb-api/app"
	domain "github.com/slidebolt/sb-domain"
	storage "github.com/slidebolt/sb-storage-sdk"
	testkit "github.com/slidebolt/sb-testkit"
)

func TestIntegration_HTTPAPIEntityAndCommandFlow(t *testing.T) {
	now := time.Date(2026, 3, 26, 17, 0, 0, 0, time.UTC)
	sender := newFakeSender()

	env := testkit.NewTestEnv(t)
	env.Start("messenger")
	env.Start("storage")

	deps := map[string]json.RawMessage{
		"messenger": env.MessengerPayload(),
	}

	api := apiapp.New(apiapp.Config{
		ListenAddr: "127.0.0.1:0",
		HTTPURL:    "127.0.0.1",
	})
	apiPayload, err := api.OnStart(deps)
	if err != nil {
		t.Fatalf("start api: %v", err)
	}
	t.Cleanup(func() { _ = api.OnShutdown() })

	app := New()
	app.sender = sender
	app.now = func() time.Time { return now }
	if _, err := app.OnStart(deps); err != nil {
		t.Fatalf("start plugin: %v", err)
	}
	t.Cleanup(func() { _ = app.OnShutdown() })

	var apiInfo struct {
		HTTPPort int `json:"http_port"`
	}
	if err := json.Unmarshal(apiPayload, &apiInfo); err != nil {
		t.Fatalf("parse api payload: %v", err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", apiInfo.HTTPPort)

	// Bootstrap an API token (auth middleware requires it).
	bearerToken := bootstrapAPIToken(t, baseURL)

	entity := domain.Entity{
		ID:       "pixel8",
		Plugin:   PluginID,
		DeviceID: "pixel8",
		Type:     "phone",
		Name:     "Pixel 8",
		Commands: []string{"phone_register_push_token", "phone_send_notification", "phone_send_data_message"},
		State:    domain.Phone{Platform: "android"},
	}
	mustJSONRequest(t, http.MethodPut, baseURL+"/entities/"+PluginID+"/pixel8/pixel8", entity, http.StatusNoContent, bearerToken)

	register := domain.PhoneRegisterPushToken{
		Token:                  "api-token-123",
		Platform:               "android",
		LastSeen:               "2026-03-26T17:00:00Z",
		BatteryLevel:           91,
		NotificationPermission: "granted",
	}
	mustJSONRequest(t, http.MethodPost, baseURL+"/entities/"+PluginID+"/pixel8/pixel8/command/phone_register_push_token", register, http.StatusNoContent, bearerToken)

	waitForCondition(t, func() bool {
		raw, err := env.Storage().ReadFile(storage.Internal, domain.EntityKey{Plugin: PluginID, DeviceID: "pixel8", ID: "pixel8"})
		return err == nil && len(raw) > 0
	})

	mustJSONRequest(t, http.MethodPost, baseURL+"/entities/"+PluginID+"/pixel8/pixel8/command/phone_send_notification", domain.PhoneSendNotification{
		Title: "Doorbell",
		Body:  "Front door",
		Data:  map[string]string{"route": "camera/front"},
	}, http.StatusNoContent, bearerToken)
	waitFor(t, sender.notifyC)

	calls := sender.snapshot()
	if len(calls) != 1 || calls[0].Token != "api-token-123" {
		t.Fatalf("sender calls = %+v", calls)
	}

	noTokenEntity := domain.Entity{
		ID:       "pixel7",
		Plugin:   PluginID,
		DeviceID: "pixel7",
		Type:     "phone",
		Name:     "Pixel 7",
		Commands: []string{"phone_send_notification"},
		State:    domain.Phone{Platform: "android"},
	}
	mustJSONRequest(t, http.MethodPut, baseURL+"/entities/"+PluginID+"/pixel7/pixel7", noTokenEntity, http.StatusNoContent, bearerToken)
	mustJSONRequest(t, http.MethodPost, baseURL+"/entities/"+PluginID+"/pixel7/pixel7/command/phone_send_notification", domain.PhoneSendNotification{
		Title: "Should fail in plugin",
	}, http.StatusNoContent, bearerToken)

	waitForCondition(t, func() bool {
		raw, err := env.Storage().Get(domain.EntityKey{Plugin: PluginID, DeviceID: "pixel7", ID: "pixel7"})
		if err != nil {
			return false
		}
		var got domain.Entity
		if json.Unmarshal(raw, &got) != nil {
			return false
		}
		state, ok := got.State.(domain.Phone)
		return ok && state.LastNotificationError != ""
	})
}

// bootstrapAPIToken creates an API token via the bootstrap endpoint (POST
// /tokens is allowed without auth when no tokens exist yet) and returns
// the bearer secret for use in subsequent requests.
func bootstrapAPIToken(t *testing.T, baseURL string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"name":   "integration-test",
		"scopes": []string{"read", "control", "write", "admin"},
	})
	req, err := http.NewRequest(http.MethodPost, baseURL+"/tokens", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build bootstrap token request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /tokens: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /tokens: status=%d body=%s", resp.StatusCode, respBody)
	}
	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode bootstrap token response: %v", err)
	}
	if result.Token == "" {
		t.Fatal("bootstrap token response missing token field")
	}
	return result.Token
}

func mustJSONRequest(t *testing.T, method, url string, body any, wantStatus int, bearerToken string) {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != wantStatus {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s %s: status=%d want=%d body=%s", method, url, resp.StatusCode, wantStatus, respBody)
	}
}
