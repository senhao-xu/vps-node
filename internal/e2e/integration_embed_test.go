//go:build integration && with_quic && with_utls

package e2e

import (
	"strings"
	"testing"

	"vps-node/internal/agentclient"
	kernelsingbox "vps-node/internal/kernel/singbox"
)

func userRefs(users []agentclient.User) []kernelsingbox.UserRef {
	out := make([]kernelsingbox.UserRef, 0, len(users))
	for _, u := range users {
		ref := kernelsingbox.UserRef{ID: u.ID, Nodes: make([]kernelsingbox.NodeRef, 0, len(u.Nodes))}
		for _, n := range u.Nodes {
			ref.Nodes = append(ref.Nodes, kernelsingbox.NodeRef{ID: n.ID, Protocol: n.Protocol, Port: n.Port})
		}
		out = append(out, ref)
	}
	return out
}

func TestEmbeddedSingBoxRuntimeStartStop(t *testing.T) {
	env, _, registerToken := seedProtocols(t)
	_, cfgResp := fetchRenderedConfig(t, env, registerToken)

	runtime := kernelsingbox.NewRuntime()
	if err := runtime.Start(cfgResp.Config.Singbox, userRefs(cfgResp.Users)); err != nil {
		if strings.Contains(err.Error(), "address already in use") {
			t.Skipf("node listen ports already in use in this environment: %v", err)
		}
		t.Fatalf("start embedded sing-box: %v", err)
	}
	defer func() {
		if err := runtime.Stop(); err != nil {
			t.Fatalf("stop embedded sing-box: %v", err)
		}
	}()
	if snapshot := runtime.Snapshot(); snapshot.Traffic == nil {
		t.Fatal("snapshot traffic map must not be nil")
	}
}
