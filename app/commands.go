package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	domain "github.com/slidebolt/sb-domain"
	messenger "github.com/slidebolt/sb-messenger-sdk"
	storage "github.com/slidebolt/sb-storage-sdk"
)

type storedPushToken struct {
	Token        string `json:"token"`
	RegisteredAt string `json:"registeredAt"`
}

func (a *App) handleCommand(addr messenger.Address, cmd any) {
	key := domain.EntityKey{Plugin: addr.Plugin, DeviceID: addr.DeviceID, ID: addr.EntityID}

	raw, err := a.store.Get(key)
	if err != nil {
		log.Printf("plugin-androidphone: command for unknown entity %s: %v", addr.Key(), err)
		return
	}

	var entity domain.Entity
	if err := json.Unmarshal(raw, &entity); err != nil {
		log.Printf("plugin-androidphone: parse entity %s: %v", addr.Key(), err)
		return
	}
	if entity.Type != "phone" {
		log.Printf("plugin-androidphone: ignoring command for non-phone entity %s (%s)", addr.Key(), entity.Type)
		return
	}

	switch c := cmd.(type) {
	case domain.PhoneRegisterPushToken:
		if err := a.registerPushToken(key, entity, c); err != nil {
			log.Printf("plugin-androidphone: register push token for %s: %v", addr.Key(), err)
		}
	case domain.PhoneSendNotification:
		if err := a.sendNotification(key, entity, c); err != nil {
			log.Printf("plugin-androidphone: send notification for %s: %v", addr.Key(), err)
		}
	case domain.PhoneSendDataMessage:
		if err := a.sendDataMessage(key, entity, c); err != nil {
			log.Printf("plugin-androidphone: send data message for %s: %v", addr.Key(), err)
		}
	default:
		log.Printf("plugin-androidphone: unknown command %T for %s", cmd, addr.Key())
	}
}

func (a *App) registerPushToken(key domain.EntityKey, entity domain.Entity, cmd domain.PhoneRegisterPushToken) error {
	payload, err := json.Marshal(storedPushToken{
		Token:        cmd.Token,
		RegisteredAt: a.nowUTC(),
	})
	if err != nil {
		return fmt.Errorf("marshal token payload: %w", err)
	}
	if err := a.store.WriteFile(storage.Internal, key, payload); err != nil {
		return fmt.Errorf("write internal token: %w", err)
	}

	return a.updatePhoneState(entity, func(state *domain.Phone) {
		state.Online = true
		state.PushTokenConfigured = true
		state.LastSeen = firstNonEmpty(cmd.LastSeen, a.nowUTC())
		state.Platform = firstNonEmpty(cmd.Platform, state.Platform)
		state.BatteryLevel = cmd.BatteryLevel
		state.IsCharging = cmd.IsCharging
		state.NotificationPermission = firstNonEmpty(cmd.NotificationPermission, state.NotificationPermission)
		state.DeviceModel = firstNonEmpty(cmd.DeviceModel, state.DeviceModel)
		state.AppVersion = firstNonEmpty(cmd.AppVersion, state.AppVersion)
		state.LastNotificationError = ""
	})
}

func (a *App) sendNotification(key domain.EntityKey, entity domain.Entity, cmd domain.PhoneSendNotification) error {
	return a.sendPush(key, entity, outboundPush{
		Notification: &pushNotification{
			Title: cmd.Title,
			Body:  cmd.Body,
			Image: cmd.ImageURL,
		},
		Data: cmd.Data,
	})
}

func (a *App) sendDataMessage(key domain.EntityKey, entity domain.Entity, cmd domain.PhoneSendDataMessage) error {
	return a.sendPush(key, entity, outboundPush{Data: cmd.Data})
}

func (a *App) sendPush(key domain.EntityKey, entity domain.Entity, msg outboundPush) error {
	token, err := a.readPushToken(key)
	if err != nil {
		a.setPhoneError(entity, err.Error())
		return err
	}
	if err := a.send(context.Background(), token, msg); err != nil {
		a.setPhoneError(entity, err.Error())
		return err
	}
	return a.updatePhoneState(entity, func(state *domain.Phone) {
		state.LastNotificationAt = a.nowUTC()
		state.LastNotificationError = ""
		state.Online = true
	})
}

func (a *App) readPushToken(key domain.EntityKey) (string, error) {
	raw, err := a.store.ReadFile(storage.Internal, key)
	if err != nil {
		return "", fmt.Errorf("read stored push token: %w", err)
	}
	var stored storedPushToken
	if err := json.Unmarshal(raw, &stored); err != nil {
		return "", fmt.Errorf("decode stored push token: %w", err)
	}
	if stored.Token == "" {
		return "", fmt.Errorf("stored push token is empty")
	}
	return stored.Token, nil
}

func (a *App) setPhoneError(entity domain.Entity, message string) {
	if err := a.updatePhoneState(entity, func(state *domain.Phone) {
		state.LastNotificationError = message
	}); err != nil {
		log.Printf("plugin-androidphone: persist error state for %s: %v", entity.Key(), err)
	}
}

func (a *App) updatePhoneState(entity domain.Entity, mutate func(*domain.Phone)) error {
	state, _ := entity.State.(domain.Phone)
	mutate(&state)
	entity.State = state
	return a.store.Save(entity)
}

func firstNonEmpty(v, fallback string) string {
	if v != "" {
		return v
	}
	return fallback
}
