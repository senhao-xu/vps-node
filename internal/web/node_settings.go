package web

import (
	"crypto/ecdh"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"vps-node/internal/repo"
	"vps-node/internal/secrets"
)

var shadowsocksCiphers = map[string]bool{
	"2022-blake3-aes-128-gcm":       true,
	"2022-blake3-aes-256-gcm":       true,
	"2022-blake3-chacha20-poly1305": true,
}

type settingField struct {
	secret   bool
	validate func(value any) (any, error)
}

var nodeSettingsSchemas = map[string]map[string]settingField{
	repo.ProtocolShadowsocks: {
		"cipher":        {validate: cipherField},
		"obfs":          {validate: optionalStringField("settings.obfs", 64)},
		"obfs_settings": {validate: objectField("settings.obfs_settings")},
		"plugin":        {validate: optionalStringField("settings.plugin", 128)},
		"plugin_opts":   {validate: optionalStringField("settings.plugin_opts", 1024)},
		"password":      {secret: true, validate: secretStringField("settings.password")},
	},
	repo.ProtocolVLESS: {
		"tls":                          {validate: vlessTLSField},
		"tls_settings":                 {validate: objectField("settings.tls_settings")},
		"reality_settings.server_name": {validate: requiredStringField("settings.reality_settings.server_name", 253)},
		"reality_settings.server_port": {validate: portField("settings.reality_settings.server_port")},
		"reality_settings.public_key":  {validate: optionalStringField("settings.reality_settings.public_key", 128)},
		"reality_settings.short_id":    {validate: shortIDField},
		"reality_settings.allow_insecure": {
			validate: boolField("settings.reality_settings.allow_insecure"),
		},
		"flow":             {validate: flowField},
		"network":          {validate: networkField},
		"network_settings": {validate: objectField("settings.network_settings")},
		"multiplex":        {validate: objectField("settings.multiplex")},
		"utls":             {validate: objectField("settings.utls")},
		"private_key":      {secret: true, validate: secretStringField("settings.private_key")},
	},
	repo.ProtocolHysteria2: {
		"version":            {validate: hy2VersionField},
		"bandwidth.up":       {validate: nonNegativeIntField("settings.bandwidth.up")},
		"bandwidth.down":     {validate: nonNegativeIntField("settings.bandwidth.down")},
		"obfs.open":          {validate: boolField("settings.obfs.open")},
		"obfs.type":          {validate: obfsTypeField},
		"obfs.password":      {validate: optionalStringField("settings.obfs.password", 64)},
		"tls.server_name":    {validate: requiredStringField("settings.tls.server_name", 253)},
		"tls.allow_insecure": {validate: boolField("settings.tls.allow_insecure")},
		"hop_interval":       {validate: hopIntervalField},
		"password":           {secret: true, validate: secretStringField("settings.password")},
		"certificate":        {secret: true, validate: pemField("settings.certificate")},
		"private_key":        {secret: true, validate: pemField("settings.private_key")},
	},
	repo.ProtocolAnyTLS: {
		"padding_scheme":  {validate: stringOrListField("settings.padding_scheme")},
		"tls.server_name": {validate: requiredStringField("settings.tls.server_name", 253)},
		"tls.allow_insecure": {
			validate: boolField("settings.tls.allow_insecure"),
		},
		"password":    {secret: true, validate: secretStringField("settings.password")},
		"certificate": {secret: true, validate: pemField("settings.certificate")},
		"private_key": {secret: true, validate: pemField("settings.private_key")},
	},
}

func (h *Handler) buildNodeSettings(protocol string, raw json.RawMessage, currentSettings string, currentSecret []byte) (string, []byte, error) {
	schema, ok := nodeSettingsSchemas[protocol]
	if !ok {
		return "", nil, errInvalid("invalid protocol")
	}
	plain := map[string]any{}
	if currentSettings != "" {
		if err := json.Unmarshal([]byte(currentSettings), &plain); err != nil {
			return "", nil, fmt.Errorf("decode current node settings: %w", err)
		}
	}
	secretFields := map[string]any{}
	if len(currentSecret) > 0 {
		secretJSON, err := secrets.Decrypt(h.appKey, currentSecret)
		if err != nil {
			return "", nil, fmt.Errorf("decrypt current node settings: %w", err)
		}
		if err := json.Unmarshal(secretJSON, &secretFields); err != nil {
			return "", nil, fmt.Errorf("decode current node secrets: %w", err)
		}
	}

	parsed := map[string]any{}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return "", nil, errValidation("settings must be a JSON object")
		}
	}
	if protocol == repo.ProtocolHysteria2 || protocol == repo.ProtocolAnyTLS {
		_, certSet := parsed["certificate"]
		_, keySet := parsed["private_key"]
		if certSet != keySet {
			return "", nil, errValidation(protocol + " certificate and private_key must be supplied together")
		}
	}

	leaves := map[string]any{}
	if err := flattenSettings(protocol, parsed, schema, leaves); err != nil {
		return "", nil, err
	}
	for path, value := range leaves {
		applySetting(plain, secretFields, path, value, schema[path].secret)
	}
	if err := normaliseNodeSettings(protocol, plain, secretFields, leaves); err != nil {
		return "", nil, err
	}
	if err := validateProtocolSettings(protocol, plain, secretFields); err != nil {
		return "", nil, err
	}

	settingsBytes, err := json.Marshal(plain)
	if err != nil {
		return "", nil, err
	}
	if len(secretFields) == 0 {
		return string(settingsBytes), nil, nil
	}
	secretBytes, err := json.Marshal(secretFields)
	if err != nil {
		return "", nil, err
	}
	secretEnc, err := secrets.Encrypt(h.appKey, secretBytes)
	if err != nil {
		return "", nil, err
	}
	return string(settingsBytes), secretEnc, nil
}

func flattenSettings(protocol string, input map[string]any, schema map[string]settingField, out map[string]any) error {
	var walk func(prefix string, value any) error
	walk = func(prefix string, value any) error {
		if field, ok := schema[prefix]; ok {
			normalized, err := field.validate(value)
			if err != nil {
				return err
			}
			out[prefix] = normalized
			return nil
		}
		if value == nil {
			return errValidation(fmt.Sprintf("unknown settings field %q for %s", prefix, protocol))
		}
		obj, ok := value.(map[string]any)
		if !ok {
			return errValidation(fmt.Sprintf("unknown settings field %q for %s", prefix, protocol))
		}
		for key, child := range obj {
			next := key
			if prefix != "" {
				next = prefix + "." + key
			}
			if err := walk(next, child); err != nil {
				return err
			}
		}
		return nil
	}
	for key, value := range input {
		if err := walk(key, value); err != nil {
			return err
		}
	}
	return nil
}

func applySetting(plain, secretFields map[string]any, path string, value any, isSecret bool) {
	target := plain
	if isSecret {
		target = secretFields
	}
	parts := strings.Split(path, ".")
	current := target
	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
			return
		}
		next, ok := current[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[part] = next
		}
		current = next
	}
}

func normaliseNodeSettings(protocol string, plain, secretFields map[string]any, leaves map[string]any) error {
	if protocol != repo.ProtocolVLESS {
		return nil
	}
	privateKey, _ := secretFields["private_key"].(string)
	if privateKey == "" {
		return nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(privateKey)
	if err != nil || len(decoded) != 32 {
		return errValidation("settings.private_key must be a valid X25519 private key")
	}
	key, err := ecdh.X25519().NewPrivateKey(decoded)
	if err != nil {
		return errValidation("settings.private_key must be a valid X25519 private key")
	}
	publicKey := base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())
	if supplied, ok := leaves["reality_settings.public_key"].(string); ok && supplied != "" && supplied != publicKey {
		return errValidation("settings.reality_settings.public_key does not match private_key")
	}
	applySetting(plain, secretFields, "reality_settings.public_key", publicKey, false)
	return nil
}

func validateProtocolSettings(protocol string, plain, secretFields map[string]any) error {
	switch protocol {
	case repo.ProtocolShadowsocks:
		cipher, _ := plain["cipher"].(string)
		if !shadowsocksCiphers[cipher] {
			return errValidation("settings.cipher is required and must be a supported shadowsocks cipher")
		}
	case repo.ProtocolVLESS:
		privateKey, _ := secretFields["private_key"].(string)
		decoded, err := base64.RawURLEncoding.DecodeString(privateKey)
		if err != nil || len(decoded) != 32 {
			return errValidation("settings.private_key must be a valid X25519 private key")
		}
		if _, err := ecdh.X25519().NewPrivateKey(decoded); err != nil {
			return errValidation("settings.private_key must be a valid X25519 private key")
		}
		serverName, _ := nestingString(plain, "reality_settings", "server_name")
		if serverName == "" || len(serverName) > 253 {
			return errValidation("settings.reality_settings.server_name must contain at least one server name")
		}
		if shortID, _ := nestingString(plain, "reality_settings", "short_id"); shortID != "" {
			decoded, err := hex.DecodeString(shortID)
			if err != nil || len(decoded) > 8 {
				return errValidation("settings.reality_settings.short_id must be an even-length hexadecimal string of at most 16 characters")
			}
		}
	case repo.ProtocolHysteria2, repo.ProtocolAnyTLS:
		return validateTLSMaterial(protocol, plain, secretFields)
	default:
		return errInvalid("invalid protocol")
	}
	return nil
}

func validateTLSMaterial(protocol string, plain, secretFields map[string]any) error {
	name, _ := nestingString(plain, "tls", "server_name")
	certificate, certOK := secretFields["certificate"].(string)
	privateKey, keyOK := secretFields["private_key"].(string)
	if name == "" || !certOK || !keyOK || certificate == "" || privateKey == "" {
		return errValidation(protocol + " requires tls.server_name, certificate and private_key")
	}
	pair, err := tls.X509KeyPair([]byte(certificate), []byte(privateKey))
	if err != nil || len(pair.Certificate) == 0 {
		return errValidation(protocol + " certificate and private_key must match")
	}
	cert, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil || cert.NotBefore.After(time.Now()) || cert.NotAfter.Before(time.Now()) || cert.VerifyHostname(name) != nil {
		return errValidation(protocol + " certificate does not cover tls.server_name")
	}
	return nil
}

func nestingString(m map[string]any, path ...string) (string, bool) {
	var current any = m
	for _, key := range path {
		obj, ok := current.(map[string]any)
		if !ok {
			return "", false
		}
		current, ok = obj[key]
		if !ok {
			return "", false
		}
	}
	s, ok := current.(string)
	return s, ok
}

func cipherField(value any) (any, error) {
	s, ok := value.(string)
	if !ok || !shadowsocksCiphers[s] {
		return nil, errValidation("settings.cipher must be a supported shadowsocks cipher")
	}
	return s, nil
}

func optionalStringField(field string, maxLen int) func(any) (any, error) {
	return func(value any) (any, error) {
		s, ok := value.(string)
		if !ok || len(s) > maxLen {
			return nil, errValidation(fmt.Sprintf("%s must be a string of at most %d characters", field, maxLen))
		}
		return s, nil
	}
}

func requiredStringField(field string, maxLen int) func(any) (any, error) {
	return func(value any) (any, error) {
		s, ok := value.(string)
		if !ok || s == "" || len(s) > maxLen {
			return nil, errValidation(fmt.Sprintf("%s must be a non-empty string of at most %d characters", field, maxLen))
		}
		return s, nil
	}
}

func secretStringField(field string) func(any) (any, error) {
	return func(value any) (any, error) {
		s, ok := value.(string)
		if !ok || len(s) < 1 || len(s) > 256 {
			return nil, errValidation(field + " must be a string of 1-256 characters")
		}
		return s, nil
	}
}

func pemField(field string) func(any) (any, error) {
	return func(value any) (any, error) {
		s, ok := value.(string)
		if !ok || s == "" || len(s) > 256*1024 {
			return nil, errValidation(field + " must be a non-empty PEM string")
		}
		return s, nil
	}
}

func objectField(field string) func(any) (any, error) {
	return func(value any) (any, error) {
		obj, ok := value.(map[string]any)
		if !ok {
			return nil, errValidation(field + " must be an object")
		}
		return obj, nil
	}
}

func boolField(field string) func(any) (any, error) {
	return func(value any) (any, error) {
		b, ok := value.(bool)
		if !ok {
			return nil, errValidation(field + " must be a boolean")
		}
		return b, nil
	}
}

func nonNegativeIntField(field string) func(any) (any, error) {
	return func(value any) (any, error) {
		f, ok := value.(float64)
		if !ok || f < 0 || f > math.MaxInt64 || f != math.Trunc(f) {
			return nil, errValidation(field + " must be a non-negative integer")
		}
		return int64(f), nil
	}
}

func portField(field string) func(any) (any, error) {
	return func(value any) (any, error) {
		f, ok := value.(float64)
		if !ok || f < 1 || f > 65535 || f != math.Trunc(f) {
			return nil, errValidation(field + " must be an integer between 1 and 65535")
		}
		return int64(f), nil
	}
}

func vlessTLSField(value any) (any, error) {
	f, ok := value.(float64)
	if !ok || f != 2 {
		return nil, errValidation("settings.tls must be 2 (reality) for vless")
	}
	return int64(2), nil
}

func hy2VersionField(value any) (any, error) {
	f, ok := value.(float64)
	if !ok || f != 2 {
		return nil, errValidation("settings.version must be 2 for hysteria2")
	}
	return int64(2), nil
}

func flowField(value any) (any, error) {
	s, ok := value.(string)
	if !ok || (s != "" && s != "xtls-rprx-vision") {
		return nil, errValidation("settings.flow must be empty or xtls-rprx-vision")
	}
	return s, nil
}

func networkField(value any) (any, error) {
	s, ok := value.(string)
	if !ok || (s != "" && s != "tcp") {
		return nil, errValidation("settings.network must be empty or tcp")
	}
	return s, nil
}

func obfsTypeField(value any) (any, error) {
	s, ok := value.(string)
	if !ok || (s != "" && s != "salamander") {
		return nil, errValidation("settings.obfs.type must be empty or salamander")
	}
	return s, nil
}

func shortIDField(value any) (any, error) {
	s, ok := value.(string)
	if !ok || len(s) > 16 {
		return nil, errValidation("settings.reality_settings.short_id must be a hexadecimal string of at most 16 characters")
	}
	if s == "" {
		return s, nil
	}
	decoded, err := hex.DecodeString(s)
	if err != nil || len(decoded) > 8 {
		return nil, errValidation("settings.reality_settings.short_id must be an even-length hexadecimal string of at most 16 characters")
	}
	return s, nil
}

func stringOrListField(field string) func(any) (any, error) {
	return func(value any) (any, error) {
		switch v := value.(type) {
		case string:
			return v, nil
		case []any:
			out := make([]string, 0, len(v))
			for _, item := range v {
				s, ok := item.(string)
				if !ok {
					return nil, errValidation(field + " must be a string or an array of strings")
				}
				out = append(out, s)
			}
			return out, nil
		default:
			return nil, errValidation(field + " must be a string or an array of strings")
		}
	}
}

var hopPortsPattern = regexp.MustCompile(`^(\d+)-(\d+)$`)

func hopIntervalField(value any) (any, error) {
	s, ok := value.(string)
	if !ok {
		return nil, errValidation("settings.hop_interval must be a start-end string such as 30000-40000")
	}
	if s == "" {
		return s, nil
	}
	if err := validateHopPorts(s); err != nil {
		return nil, err
	}
	return s, nil
}

func validateHopPorts(s string) error {
	m := hopPortsPattern.FindStringSubmatch(s)
	if m == nil {
		return errValidation("settings.hop_interval format must be start-end, for example 30000-40000")
	}
	start, errStart := strconv.Atoi(m[1])
	end, errEnd := strconv.Atoi(m[2])
	if errStart != nil || errEnd != nil || start < 1 || end > 65535 || start > end {
		return errValidation("settings.hop_interval ports must be within 1-65535 with start not greater than end")
	}
	return nil
}
