package config

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Revision is a UUIDv7 Hub revision. Its byte order is the UUID wire order,
// which is also the lexicographic order of UUIDv7 timestamps and counters.
type Revision [16]byte

// Snapshot is the authoritative Hub configuration and its freshness revision.
type Snapshot struct {
	Revision Revision
	Config   map[string]string
}

// ParseRevision parses and validates a canonical UUIDv7 string.
func ParseRevision(value string) (Revision, error) {
	var revision Revision
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return revision, fmt.Errorf("invalid UUIDv7 revision format")
	}

	compact := make([]byte, 0, 32)
	for i := 0; i < len(value); i++ {
		if value[i] != '-' {
			compact = append(compact, value[i])
		}
	}
	if _, err := hex.Decode(revision[:], compact); err != nil {
		return Revision{}, fmt.Errorf("invalid UUIDv7 revision: %w", err)
	}
	if revision[6]>>4 != 7 {
		return Revision{}, fmt.Errorf("revision is not UUIDv7")
	}
	if revision[8]&0xc0 != 0x80 {
		return Revision{}, fmt.Errorf("revision has invalid UUID variant")
	}
	return revision, nil
}

func (r Revision) String() string {
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(r[0:4]),
		hex.EncodeToString(r[4:6]),
		hex.EncodeToString(r[6:8]),
		hex.EncodeToString(r[8:10]),
		hex.EncodeToString(r[10:16]),
	)
}

// Compare compares two UUIDv7 revisions in Hub freshness order.
func (r Revision) Compare(other Revision) int {
	return bytes.Compare(r[:], other[:])
}

func parseSnapshotEnvelope(body []byte) (Snapshot, error) {
	var envelope struct {
		Revision string            `json:"revision"`
		Config   map[string]string `json:"config"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return Snapshot{}, fmt.Errorf("parse snapshot envelope: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return Snapshot{}, fmt.Errorf("parse snapshot envelope: trailing JSON data")
	}
	if envelope.Config == nil {
		return Snapshot{}, fmt.Errorf("snapshot config must be an object")
	}
	revision, err := ParseRevision(envelope.Revision)
	if err != nil {
		return Snapshot{}, err
	}
	config, err := cloneAndValidateConfig(envelope.Config)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Revision: revision, Config: config}, nil
}

func parseConfigMap(body []byte) (map[string]string, error) {
	var config map[string]string
	if err := json.Unmarshal(body, &config); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if config == nil {
		return nil, fmt.Errorf("config must be an object")
	}
	return cloneAndValidateConfig(config)
}

func cloneAndValidateConfig(config map[string]string) (map[string]string, error) {
	cloned := make(map[string]string, len(config))
	for key, value := range config {
		if !isValidConfigName(key) {
			return nil, fmt.Errorf("invalid environment variable name %q", key)
		}
		if strings.IndexByte(value, 0) >= 0 {
			return nil, fmt.Errorf("environment variable %q contains NUL", key)
		}
		cloned[key] = value
	}
	return cloned, nil
}

func isValidConfigName(name string) bool {
	if len(name) == 0 {
		return false
	}
	isLetter := func(c byte) bool {
		return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
	}
	if !isLetter(name[0]) && name[0] != '_' {
		return false
	}
	for i := 1; i < len(name); i++ {
		if !isLetter(name[i]) && (name[i] < '0' || name[i] > '9') && name[i] != '_' {
			return false
		}
	}
	return true
}
