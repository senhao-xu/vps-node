package agentstate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type State struct {
	AgentID         int64     `json:"agent_id,omitempty"`
	AgentToken      string    `json:"agent_token,omitempty"`
	ServerID        int64     `json:"server_id,omitempty"`
	AppliedRevision int64     `json:"applied_revision,omitempty"`
	TrafficBatchSeq int64     `json:"traffic_batch_seq,omitempty"`
	LogBatchSeq     int64     `json:"log_batch_seq,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func Load(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &State{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read agent state: %w", err)
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse agent state %s: %w", path, err)
	}
	return &s, nil
}

func Save(path string, s *State) error {
	s.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".agent-state-*")
	if err != nil {
		return fmt.Errorf("create state temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod state file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write state file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync state file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace state file: %w", err)
	}
	return nil
}
