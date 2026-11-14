package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	contract "github.com/slidebolt/sb-contract"
	domain "github.com/slidebolt/sb-domain"
	messenger "github.com/slidebolt/sb-messenger-sdk"
	storage "github.com/slidebolt/sb-storage-sdk"
)

const PluginID = "plugin-androidphone"

type App struct {
	msg    messenger.Messenger
	store  storage.Storage
	cmds   *messenger.Commands
	subs   []messenger.Subscription
	sender pushSender
	now    func() time.Time
}

func New() *App {
	return &App{now: time.Now}
}

func (a *App) Hello() contract.HelloResponse {
	return contract.HelloResponse{
		ID:              PluginID,
		Kind:            contract.KindPlugin,
		ContractVersion: contract.ContractVersion,
		Definitions:     contract.DescribePlugin("Android Phone", "Connects Android phones and translates notifications and audio playback into shared contracts.", "phone"),
		DependsOn:       []string{"messenger", "storage"},
		Setup: &contract.SetupDefinition{Mode: contract.SetupSelectable, Category: "Notifications", Fields: []contract.SetupField{
			{Key: "fcm_project_id", Environment: "ANDROIDPHONE_FCM_PROJECT_ID", Name: "Firebase project ID", Description: "Optional Firebase Cloud Messaging project used for outbound phone notifications.", Control: contract.SetupText},
			{Key: "fcm_credentials_file", Environment: "ANDROIDPHONE_FCM_CREDENTIALS_JSON", Name: "Firebase credentials file", Description: "Service-account JSON file for Firebase Cloud Messaging.", Control: contract.SetupFile, Sensitive: true, Export: "file_contents"},
			{Key: "fcm_base_url", Environment: "ANDROIDPHONE_FCM_BASE_URL", Name: "Firebase API URL", Description: "Firebase Messaging API base URL.", Control: contract.SetupText, Format: "uri", Advanced: true, Default: json.RawMessage(`"https://fcm.googleapis.com"`)},
		}, Constraints: []contract.SetupConstraint{{Kind: "all_or_none", Fields: []string{"fcm_project_id", "fcm_credentials_file"}}}},
	}
}

func (a *App) OnStart(deps map[string]json.RawMessage) (json.RawMessage, error) {
	msg, err := messenger.Connect(deps)
	if err != nil {
		return nil, fmt.Errorf("connect messenger: %w", err)
	}
	a.msg = msg

	store, err := storage.Connect(deps)
	if err != nil {
		return nil, fmt.Errorf("connect storage: %w", err)
	}
	a.store = store

	if a.sender == nil {
		cfg := loadConfigFromEnv()
		sender, err := newFCMSender(cfg)
		if err != nil {
			return nil, fmt.Errorf("configure fcm sender: %w", err)
		}
		a.sender = sender
	}
	if a.now == nil {
		a.now = time.Now
	}

	a.cmds = messenger.NewCommands(msg, domain.LookupCommand)
	sub, err := a.cmds.Receive(PluginID+".>", a.handleCommand)
	if err != nil {
		return nil, fmt.Errorf("subscribe commands: %w", err)
	}
	a.subs = append(a.subs, sub)

	log.Println("plugin-androidphone: started")
	return nil, nil
}

func (a *App) OnShutdown() error {
	for _, sub := range a.subs {
		sub.Unsubscribe()
	}
	if a.store != nil {
		a.store.Close()
	}
	if a.msg != nil {
		a.msg.Close()
	}
	return nil
}

func (a *App) nowUTC() string {
	return a.now().UTC().Format(time.RFC3339)
}

func (a *App) send(ctx context.Context, token string, msg outboundPush) error {
	if a.sender == nil {
		return fmt.Errorf("fcm sender is not configured")
	}
	return a.sender.Send(ctx, token, msg)
}
