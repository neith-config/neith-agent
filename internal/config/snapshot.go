package config

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	if err := json.Unmarshal(body, &envelope); err != nil {
		return Snapshot{}, fmt.Errorf("parse snapshot envelope: %w", err)
	}
	if envelope.Config == nil {
		return Snapshot{}, fmt.Errorf("snapshot config must be an object")
	}
	revision, err := ParseRevision(envelope.Revision)
	if err != nil {
		return Snapshot{}, err
	}
	config := make(map[string]string, len(envelope.Config))
	for key, value := range envelope.Config {
		config[key] = value
	}
	return Snapshot{Revision: revision, Config: config}, nil
}
