package deploy

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestInstallerReleaseSelectionAndRequiredCredentials(t *testing.T) {
	for _, tc := range []struct {
		name, version, mirror, arch, wantURL string
		credentials                          bool
	}{
		{name: "latest-amd64", arch: "x86_64", credentials: true, wantURL: "https://github.com/senhao-xu/vps-node/releases/latest/download/panel-agent-latest-linux-amd64.tar.gz"},
		{name: "pinned-arm64", version: "1.2.3", arch: "aarch64", credentials: true, wantURL: "https://github.com/senhao-xu/vps-node/releases/download/v1.2.3/panel-agent-1.2.3-linux-arm64.tar.gz"},
		{name: "mirror-386", version: "test", mirror: "https://mirror.example/agent", arch: "i686", credentials: true, wantURL: "https://mirror.example/agent/panel-agent-test-linux-386.tar.gz"},
		{name: "missing-credentials", arch: "x86_64"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			script := strings.Replace(AgentInstallScript, "ETC_DIR=/etc/panel-agent", "ETC_DIR="+filepath.Join(dir, "config"), 1)
			fixtures := map[string]string{
				"installer": script,
				"uname":     "#!/bin/sh\nprintf '%s\\n' '" + tc.arch + "'\n",
				"id":        "#!/bin/sh\nprintf '0\\n'\n",
				"curl":      "#!/bin/sh\nexit 42\n",
			}
			for name, contents := range fixtures {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0700); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.Command("/bin/sh", filepath.Join(dir, "installer"))
			cmd.Env = []string{"PATH=" + dir + ":/usr/bin:/bin", "PANEL_VERSION=" + tc.version, "PANEL_DOWNLOAD_BASE=" + tc.mirror}
			if tc.credentials {
				cmd.Env = append(cmd.Env, "PANEL_URL=https://panel.example", "SERVER_ID=1", "AGENT_KEY=test-key")
			}
			output, err := cmd.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) {
				t.Fatalf("expected controlled abort, got %v", err)
			}
			if tc.credentials {
				if exit.ExitCode() != 42 || !strings.Contains(string(output), "downloading "+tc.wantURL) {
					t.Fatalf("exit=%d output=%s", exit.ExitCode(), output)
				}
			} else if exit.ExitCode() != 1 || !strings.Contains(string(output), "required for a fresh installation") || strings.Contains(string(output), "downloading") {
				t.Fatalf("missing credentials: exit=%d output=%s", exit.ExitCode(), output)
			}
		})
	}
}

func TestInstallerPreservesExistingConfigAndRepairsPermissions(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "fresh"
		if existing {
			name = "upgrade"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			binDir := filepath.Join(dir, "bin")
			configDir := filepath.Join(dir, "config")
			for _, path := range []string{binDir, configDir} {
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			configPath := filepath.Join(configDir, "agent.yaml")
			previous := []byte("panel_url: https://old.example\nserver_id: 7\nagent_key: original-key\n")
			if existing {
				if err := os.WriteFile(configPath, previous, 0644); err != nil {
					t.Fatal(err)
				}
			}
			script := strings.ReplaceAll(AgentInstallScript, "ETC_DIR=/etc/panel-agent", "ETC_DIR="+configDir)
			script = strings.ReplaceAll(script, "BIN_DIR=/usr/local/bin", "BIN_DIR="+binDir)
			script = strings.ReplaceAll(script, "/etc/systemd/system", filepath.Join(dir, "systemd"))
			fixtures := map[string]string{
				"installer": script,
				"uname":     "#!/bin/sh\nprintf 'x86_64\\n'\n",
				"id":        "#!/bin/sh\nprintf '0\\n'\n",
				"curl":      "#!/bin/sh\nexit 0\n",
				"tar":       "#!/bin/sh\nprintf '#!/bin/sh\\nexit 0\\n' > \"$4/panel-agent\"\n",
				"chown":     "#!/bin/sh\nprintf 'repair:%s:%s\\n' \"$1\" \"$2\"\n",
				"systemctl": "#!/bin/sh\nexit 0\n",
			}
			for file, contents := range fixtures {
				if err := os.WriteFile(filepath.Join(dir, file), []byte(contents), 0700); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.Command("/bin/sh", filepath.Join(dir, "installer"))
			cmd.Env = []string{"PATH=" + dir + ":/usr/bin:/bin"}
			if !existing {
				cmd.Env = append(cmd.Env, "PANEL_URL=https://new.example/path", "SERVER_ID=8", "AGENT_KEY=key'with: punctuation")
			}
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("installer: %v\n%s", err, output)
			}
			contents, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatal(err)
			}
			if existing {
				if string(contents) != string(previous) {
					t.Fatal("upgrade changed existing credentials")
				}
			} else {
				var config struct {
					PanelURL string `yaml:"panel_url"`
					ServerID int    `yaml:"server_id"`
					AgentKey string `yaml:"agent_key"`
				}
				if err := yaml.Unmarshal(contents, &config); err != nil {
					t.Fatal(err)
				}
				if config.PanelURL != "https://new.example/path" || config.ServerID != 8 || config.AgentKey != "key'with: punctuation" {
					t.Fatalf("fresh config credentials not preserved: %+v", config)
				}
			}
			info, err := os.Stat(configPath)
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatalf("config permissions not repaired: %v %v", info, err)
			}
			if !strings.Contains(string(output), "repair:panel-agent:panel-agent:"+configPath) {
				t.Fatal("config ownership not repaired")
			}
		})
	}
}
