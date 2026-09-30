package singbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	singJSON "github.com/sagernet/sing/common/json"
	"github.com/sagernet/sing/service"
)

type Runtime struct {
	mu         sync.Mutex
	inst       *box.Box
	tracker    *ConnTracker
	lastConfig []byte
	lastUsers  []UserRef
}

func NewRuntime() *Runtime {
	return &Runtime{}
}

func (r *Runtime) Start(configJSON []byte, users []UserRef) error {
	prepared, err := prepareConfig(bytes.Clone(configJSON))
	if err != nil {
		return err
	}
	userSnapshot := cloneUserRefs(users)
	tracker := NewConnTracker(userSnapshot)
	inst, err := buildInstance(prepared, tracker)
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	previousTracker := r.tracker
	_ = r.stopLocked()
	if err := inst.Start(); err != nil {
		_ = inst.Close()
		applyErr := fmt.Errorf("start sing-box: %w", err)
		if len(r.lastConfig) == 0 {
			return applyErr
		}
		if previousTracker == nil {
			previousTracker = NewConnTracker(r.lastUsers)
		}
		r.tracker = previousTracker
		restored, restoreErr := buildInstance(r.lastConfig, previousTracker)
		if restoreErr == nil {
			restoreErr = restored.Start()
			if restoreErr != nil {
				_ = restored.Close()
			}
		}
		if restoreErr != nil {
			return errors.Join(applyErr, fmt.Errorf("restore previous sing-box config: %w", restoreErr))
		}
		r.inst = restored
		return applyErr
	}
	r.inst = inst
	r.tracker = tracker
	r.lastConfig = prepared
	r.lastUsers = userSnapshot
	return nil
}

func buildInstance(configJSON []byte, tracker *ConnTracker) (*box.Box, error) {
	ctx := include.Context(context.Background())
	opts, err := singJSON.UnmarshalExtendedContext[option.Options](ctx, configJSON)
	if err != nil {
		return nil, fmt.Errorf("parse sing-box config: %w", err)
	}
	inst, err := box.New(box.Options{Context: ctx, Options: opts})
	if err != nil {
		return nil, fmt.Errorf("build sing-box instance: %w", err)
	}
	router := service.FromContext[adapter.Router](ctx)
	if router == nil {
		_ = inst.Close()
		return nil, errors.New("sing-box router unavailable")
	}
	router.AppendTracker(tracker)
	return inst, nil
}

func cloneUserRefs(users []UserRef) []UserRef {
	cloned := append([]UserRef(nil), users...)
	for i := range cloned {
		cloned[i].Nodes = append([]NodeRef(nil), users[i].Nodes...)
	}
	return cloned
}

func (r *Runtime) Reload(configJSON []byte, users []UserRef) error {
	return r.Start(configJSON, users)
}

func (r *Runtime) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	err := r.stopLocked()
	r.tracker = nil
	r.lastConfig = nil
	r.lastUsers = nil
	return err
}

func (r *Runtime) stopLocked() error {
	if r.inst == nil {
		return nil
	}
	err := r.inst.Close()
	r.inst = nil
	r.tracker = nil
	return err
}

func (r *Runtime) Snapshot() Snapshot {
	r.mu.Lock()
	tracker := r.tracker
	r.mu.Unlock()
	if tracker == nil {
		return Snapshot{Traffic: map[Pair]Traffic{}}
	}
	return tracker.Snapshot()
}

func (r *Runtime) DrainVisits() []Visit {
	r.mu.Lock()
	tracker := r.tracker
	r.mu.Unlock()
	if tracker == nil {
		return nil
	}
	return tracker.drainVisits()
}

func Validate(configJSON []byte) error {
	prepared, err := prepareConfig(configJSON)
	if err != nil {
		return err
	}
	ctx := include.Context(context.Background())
	_, err = singJSON.UnmarshalExtendedContext[option.Options](ctx, prepared)
	return err
}

func prepareConfig(configJSON []byte) ([]byte, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(configJSON, &raw); err != nil {
		return nil, fmt.Errorf("decode sing-box config: %w", err)
	}
	if _, ok := raw["experimental"]; !ok {
		return configJSON, nil
	}
	delete(raw, "experimental")
	return json.Marshal(raw)
}
