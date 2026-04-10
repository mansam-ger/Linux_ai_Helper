package sysdb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"eugen/internal/config"
)

const DBFileName = "eugen_db.json"

// DBPath returns the full path to the database file inside the data directory.
func DBPath() string {
	return filepath.Join(config.DataDir, DBFileName)
}

// SystemData holds gathered system context
type SystemData struct {
	HardwareInfo string   `json:"hardware"`
	NetworkInfo  string   `json:"network"`
	Services     string   `json:"services"`
	CustomNotes  []string `json:"custom_notes,omitempty"`
}

// CheckDBExists returns true if the db file is present.
func CheckDBExists() bool {
	info, err := os.Stat(DBPath())
	if os.IsNotExist(err) || info.IsDir() {
		return false
	}
	return true
}

// LoadDB reads the JSON database into the struct.
func LoadDB() (*SystemData, error) {
	b, err := os.ReadFile(DBPath())
	if err != nil {
		return nil, fmt.Errorf("failed to read db file: %w", err)
	}

	var data SystemData
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, fmt.Errorf("failed to parse db json: %w", err)
	}
	return &data, nil
}

// SaveDB writes the struct to disk in JSON inside the data directory.
func SaveDB(data *SystemData) error {
	// Ensure the data directory exists
	if err := config.EnsureDataDir(); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal db: %w", err)
	}
	
	if err := os.WriteFile(DBPath(), b, 0644); err != nil {
		return fmt.Errorf("failed to write db file: %w", err)
	}
	return nil
}

// AddCustomNote appends a custom string to the database's CustomNotes array.
func AddCustomNote(note string) error {
	var data *SystemData
	if CheckDBExists() {
		var err error
		data, err = LoadDB()
		if err != nil {
			return err
		}
	} else {
		data = &SystemData{}
	}

	data.CustomNotes = append(data.CustomNotes, note)
	return SaveDB(data)
}

// ResetDB deletes the local eugen database.
func ResetDB() error {
	if !CheckDBExists() {
		return nil
	}
	return os.Remove(DBPath())
}
