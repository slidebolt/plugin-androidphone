package app

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	domain "github.com/slidebolt/sb-domain"
	messenger "github.com/slidebolt/sb-messenger-sdk"
	storage "github.com/slidebolt/sb-storage-sdk"
	testkit "github.com/slidebolt/sb-testkit"
)

type fakeSender struct {
	mu      sync.Mutex
	calls   []fakeSendCall
	err     error
	notifyC chan struct{}
}

type fakeSendCall struct {
	Token string
	Msg   outboundPush
}

func newFakeSender() *fakeSender {
	return &fakeSender{notifyC: make(chan struct{}, 10)}
}

func (f *fakeSender) Send(_ context.Context, token string, msg outboundPush) error {
	f.mu.Lock()
	f.calls = append(f.calls, fakeSendCall{Token: token, Msg: msg})
	f.mu.Unlock()
	f.notifyC <- struct{}{}
	return f.err
}

func (f *fakeSender) snapshot() []fakeSendCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]fakeSendCall, len(f.calls))
	copy(out, f.calls)
	return out
}

func startApp(t *testing.T, sender pushSender, now time.Time) (*App, *testkit.TestEnv, *messenger.Commands) {
	t.Helper()
	env := testkit.NewTestEnv(t)
	env.Start("messenger")
	env.Start("storage")

	app := New()
	app.sender = sender
	app.now = func() time.Time { return now }

	deps := map[string]json.RawMessage{
		"messenger": env.MessengerPayload(),
	}
	if _, err := app.OnStart(deps); err != nil {
		t.Fatalf("OnStart: %v", err)
	}
	t.Cleanup(func() { _ = app.OnShutdown() })

	return app, env, messenger.NewCommands(env.Messenger(), domain.LookupCommand)
}

func savePhoneEntity(t *testing.T, env *testkit.TestEnv) domain.Entity {
	t.Helper()
	entity := domain.Entity{
		ID:       "pixel8",
		Plugin:   PluginID,
		DeviceID: "pixel8",
		Type:     "phone",
		Name:     "Pixel 8",
		Commands: []string{"phone_register_push_token", "phone_send_notification", "phone_send_data_message"},
		State:    domain.Phone{Platform: "android"},
	}
	if err := env.Storage().Save(entity); err != nil {
		t.Fatalf("save phone entity: %v", err)
	}
	return entity
}

func waitFor(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for async command handler")
	}
}

func waitForCondition(t *testing.T, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}

func TestHandleCommand_RegisterPushTokenStoresInternalToken(t *testing.T) {
	now := time.Date(2026, 3, 26, 16, 0, 0, 0, time.UTC)
	_, env, cmds := startApp(t, nil, now)
	entity := savePhoneEntity(t, env)

	if err := cmds.Send(entity, domain.PhoneRegisterPushToken{
		Token:                  "token-123",
		Platform:               "android",
		LastSeen:               "2026-03-26T15:59:00Z",
		BatteryLevel:           87.5,
		IsCharging:             true,
		NotificationPermission: "granted",
		DeviceModel:            "Pixel 8",
		AppVersion:             "1.2.3",
	}); err != nil {
		t.Fatalf("send command: %v", err)
	}

	waitForCondition(t, func() bool {
		_, err := env.Storage().ReadFile(storage.Internal, domain.EntityKey{Plugin: PluginID, DeviceID: "pixel8", ID: "pixel8"})
		return err == nil
	})

	internal, err := env.Storage().ReadFile(storage.Internal, domain.EntityKey{Plugin: PluginID, DeviceID: "pixel8", ID: "pixel8"})
	if err != nil {
		t.Fatalf("read internal token: %v", err)
	}
	var token storedPushToken
	if err := json.Unmarshal(internal, &token); err != nil {
		t.Fatalf("unmarshal internal token: %v", err)
	}
	if token.Token != "token-123" {
		t.Fatalf("stored token = %q", token.Token)
	}

	raw, err := env.Storage().Get(domain.EntityKey{Plugin: PluginID, DeviceID: "pixel8", ID: "pixel8"})
	if err != nil {
		t.Fatalf("get entity: %v", err)
	}
	var got domain.Entity
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal entity: %v", err)
	}
	state, ok := got.State.(domain.Phone)
	if !ok {
		t.Fatalf("state type = %T", got.State)
	}
	if !state.PushTokenConfigured || !state.Online {
		t.Fatalf("state = %+v", state)
	}
	if state.NotificationPermission != "granted" || state.AppVersion != "1.2.3" {
		t.Fatalf("state = %+v", state)
	}
}

func TestHandleCommand_SendNotificationUsesStoredToken(t *testing.T) {
	now := time.Date(2026, 3, 26, 16, 5, 0, 0, time.UTC)
	sender := newFakeSender()
	_, env, cmds := startApp(t, sender, now)
	entity := savePhoneEntity(t, env)

	internal, _ := json.Marshal(storedPushToken{Token: "token-xyz", RegisteredAt: now.Format(time.RFC3339)})
	if err := env.Storage().WriteFile(storage.Internal, domain.EntityKey{Plugin: PluginID, DeviceID: "pixel8", ID: "pixel8"}, internal); err != nil {
		t.Fatalf("write internal token: %v", err)
	}

	if err := cmds.Send(entity, domain.PhoneSendNotification{
		Title: "Doorbell",
		Body:  "Someone is at the front door",
		Data:  map[string]string{"route": "camera/front"},
	}); err != nil {
		t.Fatalf("send command: %v", err)
	}
	waitFor(t, sender.notifyC)

	calls := sender.snapshot()
	if len(calls) != 1 {
		t.Fatalf("sender calls = %d, want 1", len(calls))
	}
	if calls[0].Token != "token-xyz" {
		t.Fatalf("sender token = %q", calls[0].Token)
	}
	if calls[0].Msg.Notification == nil || calls[0].Msg.Notification.Title != "Doorbell" {
		t.Fatalf("sender message = %+v", calls[0].Msg)
	}

	// Wait for the handler to persist the updated state after sending the
	// notification — the state update happens asynchronously after the
	// fakeSender signals notifyC.
	waitForCondition(t, func() bool {
		raw, err := env.Storage().Get(domain.EntityKey{Plugin: PluginID, DeviceID: "pixel8", ID: "pixel8"})
		if err != nil {
			return false
		}
		var got domain.Entity
		if json.Unmarshal(raw, &got) != nil {
			return false
		}
		state, ok := got.State.(domain.Phone)
		return ok && state.LastNotificationAt != ""
	})

	raw, err := env.Storage().Get(domain.EntityKey{Plugin: PluginID, DeviceID: "pixel8", ID: "pixel8"})
	if err != nil {
		t.Fatalf("get entity: %v", err)
	}
	var got domain.Entity
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal entity: %v", err)
	}
	state := got.State.(domain.Phone)
	if state.LastNotificationAt != now.Format(time.RFC3339) {
		t.Fatalf("lastNotificationAt = %q", state.LastNotificationAt)
	}
	if state.LastNotificationError != "" {
		t.Fatalf("lastNotificationError = %q", state.LastNotificationError)
	}
}

func TestHandleCommand_SendNotificationWithoutTokenSetsError(t *testing.T) {
	now := time.Date(2026, 3, 26, 16, 10, 0, 0, time.UTC)
	sender := newFakeSender()
	_, env, cmds := startApp(t, sender, now)
	entity := savePhoneEntity(t, env)

	if err := cmds.Send(entity, domain.PhoneSendNotification{Title: "Test"}); err != nil {
		t.Fatalf("send command: %v", err)
	}

	waitForCondition(t, func() bool {
		raw, err := env.Storage().Get(domain.EntityKey{Plugin: PluginID, DeviceID: "pixel8", ID: "pixel8"})
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

	if len(sender.snapshot()) != 0 {
		t.Fatalf("expected sender not to be called")
	}

	raw, err := env.Storage().Get(domain.EntityKey{Plugin: PluginID, DeviceID: "pixel8", ID: "pixel8"})
	if err != nil {
		t.Fatalf("get entity: %v", err)
	}
	var got domain.Entity
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal entity: %v", err)
	}
	state := got.State.(domain.Phone)
	if state.LastNotificationError == "" {
		t.Fatal("expected last notification error to be set")
	}
}
