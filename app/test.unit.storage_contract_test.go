package app

import (
	"encoding/json"
	"testing"

	domain "github.com/slidebolt/sb-domain"
	testkit "github.com/slidebolt/sb-testkit"
)

func TestOnStart_DoesNotSeedDemoEntities(t *testing.T) {
	env := testkit.NewTestEnv(t)
	env.Start("messenger")
	env.Start("storage")

	app := New()
	deps := map[string]json.RawMessage{
		"messenger": env.MessengerPayload(),
	}
	if _, err := app.OnStart(deps); err != nil {
		t.Fatalf("OnStart: %v", err)
	}
	t.Cleanup(func() { _ = app.OnShutdown() })

	entries, err := env.Storage().Search(PluginID + ".>")
	if err != nil {
		t.Fatalf("search entities: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("startup seeded %d entities, want 0", len(entries))
	}
}

func TestStorageContract_PhoneEntityRoundTrips(t *testing.T) {
	env := testkit.NewTestEnv(t)
	env.Start("messenger")
	env.Start("storage")

	entity := domain.Entity{
		ID:       "pixel8",
		Plugin:   PluginID,
		DeviceID: "pixel8",
		Type:     "phone",
		Name:     "Pixel 8",
		Commands: []string{"phone_register_push_token", "phone_send_notification"},
		State: domain.Phone{
			Platform:            "android",
			Online:              true,
			PushTokenConfigured: true,
			LastSeen:            "2026-03-26T16:00:00Z",
			BatteryLevel:        83.5,
		},
	}
	if err := env.Storage().Save(entity); err != nil {
		t.Fatalf("save entity: %v", err)
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
	if state.Platform != "android" || !state.PushTokenConfigured || state.BatteryLevel != 83.5 {
		t.Fatalf("state = %+v", state)
	}
}
