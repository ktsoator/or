package provider

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

func normalizeProfile(profile Profile) Profile {
	officialIndex := -1
	for index := range profile.Connections {
		connection := &profile.Connections[index]
		connection.ID = strings.TrimSpace(connection.ID)
		connection.Name = strings.TrimSpace(connection.Name)
		connection.BaseURL = strings.TrimSpace(connection.BaseURL)
		if connection.ID == OfficialConnectionID {
			officialIndex = index
			connection.Name = "Official"
			connection.BaseURL = ""
		}
	}
	if officialIndex < 0 {
		profile.Connections = append([]Connection{{
			ID:   OfficialConnectionID,
			Name: "Official",
		}}, profile.Connections...)
	}
	if profile.ActiveConnectionID == "" || FindConnection(profile, profile.ActiveConnectionID) == nil {
		profile.ActiveConnectionID = OfficialConnectionID
	}
	return profile
}

func mergeProfile(existing Profile, update Update) (Profile, error) {
	existing = normalizeProfile(existing)
	existingKeys := map[string]Key{}
	for _, connection := range existing.Connections {
		for _, key := range connection.Keys {
			existingKeys[key.ID] = key
		}
	}

	next := Profile{ActiveConnectionID: strings.TrimSpace(update.ActiveConnectionID)}
	seenConnections := map[string]struct{}{}
	seenKeys := map[string]struct{}{}
	for _, requested := range update.Connections {
		connection := Connection{
			ID:          strings.TrimSpace(requested.ID),
			Name:        strings.TrimSpace(requested.Name),
			BaseURL:     strings.TrimSpace(requested.BaseURL),
			ActiveKeyID: strings.TrimSpace(requested.ActiveKeyID),
		}
		if connection.ID == "" {
			connection.ID = newItemID("conn")
		}
		if _, duplicate := seenConnections[connection.ID]; duplicate {
			return Profile{}, fmt.Errorf("duplicate connection id %q", connection.ID)
		}
		seenConnections[connection.ID] = struct{}{}
		if connection.ID == OfficialConnectionID {
			connection.Name = "Official"
			connection.BaseURL = ""
		} else {
			if connection.Name == "" {
				return Profile{}, errors.New("custom connection name is required")
			}
			if connection.BaseURL == "" {
				return Profile{}, fmt.Errorf("base URL is required for %q", connection.Name)
			}
		}

		for _, requestedKey := range requested.Keys {
			key := Key{
				ID:     strings.TrimSpace(requestedKey.ID),
				Name:   strings.TrimSpace(requestedKey.Name),
				APIKey: strings.TrimSpace(requestedKey.APIKey),
			}
			if key.ID == "" {
				key.ID = newItemID("key")
			}
			if _, duplicate := seenKeys[key.ID]; duplicate {
				return Profile{}, fmt.Errorf("duplicate key id %q", key.ID)
			}
			seenKeys[key.ID] = struct{}{}
			if key.Name == "" {
				return Profile{}, errors.New("key name is required")
			}
			if key.APIKey == "" {
				key.APIKey = existingKeys[key.ID].APIKey
			}
			if key.APIKey == "" {
				return Profile{}, fmt.Errorf("API key is required for %q", key.Name)
			}
			connection.Keys = append(connection.Keys, key)
		}
		if connection.ActiveKeyID != "" && FindKey(connection, connection.ActiveKeyID) == nil {
			return Profile{}, fmt.Errorf("active key %q was not found", connection.ActiveKeyID)
		}
		next.Connections = append(next.Connections, connection)
	}
	requestedActiveConnectionID := next.ActiveConnectionID
	next = normalizeProfile(next)
	if requestedActiveConnectionID != "" && FindConnection(next, requestedActiveConnectionID) == nil {
		return Profile{}, fmt.Errorf("active connection %q was not found", requestedActiveConnectionID)
	}
	return next, nil
}

func validateStoredProfile(profile Profile) (Profile, error) {
	update := Update{ActiveConnectionID: profile.ActiveConnectionID}
	for _, connection := range profile.Connections {
		if strings.TrimSpace(connection.ID) == "" {
			return Profile{}, errors.New("connection id is required")
		}
		requested := ConnectionUpdate{
			ID:          connection.ID,
			Name:        connection.Name,
			BaseURL:     connection.BaseURL,
			ActiveKeyID: connection.ActiveKeyID,
		}
		for _, key := range connection.Keys {
			if strings.TrimSpace(key.ID) == "" {
				return Profile{}, errors.New("key id is required")
			}
			requested.Keys = append(requested.Keys, KeyUpdate(key))
		}
		update.Connections = append(update.Connections, requested)
	}
	return mergeProfile(Profile{}, update)
}

func FindConnection(profile Profile, id string) *Connection {
	for index := range profile.Connections {
		if profile.Connections[index].ID == id {
			return &profile.Connections[index]
		}
	}
	return nil
}

func FindKey(connection Connection, id string) *Key {
	if id == "" {
		return nil
	}
	for index := range connection.Keys {
		if connection.Keys[index].ID == id {
			return &connection.Keys[index]
		}
	}
	return nil
}

func cloneProfiles(profiles map[string]Profile) map[string]Profile {
	cloned := make(map[string]Profile, len(profiles))
	for id, profile := range profiles {
		cloned[id] = cloneProfile(normalizeProfile(profile))
	}
	return cloned
}

func cloneProfile(profile Profile) Profile {
	clone := profile
	clone.Connections = make([]Connection, len(profile.Connections))
	for index, connection := range profile.Connections {
		clone.Connections[index] = connection
		clone.Connections[index].Keys = append([]Key(nil), connection.Keys...)
	}
	return clone
}

func newItemID(prefix string) string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return prefix + "_" + hex.EncodeToString(raw[:])
	}
	return fmt.Sprintf("%s_%d_%d", prefix, os.Getpid(), time.Now().UnixNano())
}
