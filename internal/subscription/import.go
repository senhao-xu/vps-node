package subscription

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// ParseShareURI converts a single share link (ss / vless / hysteria2 / anytls
// / trojan / vmess) into a Clash Meta (mihomo) proxy mapping. It is a pure
// function: no IO, no panel state. Callers decide whether a parse failure
// skips the entry (subscription rendering) or only warns (admin form
// validation).
func ParseShareURI(raw string) (map[string]any, error) {
	line := strings.TrimSpace(raw)
	if line == "" {
		return nil, errors.New("empty share link")
	}
	scheme, _, found := strings.Cut(line, "://")
	if !found {
		// Deliberately no line content: a scheme-less entry can still be
		// base64-encoded credentials, and this error reaches the logs.
		return nil, fmt.Errorf("share link has no scheme")
	}
	switch strings.ToLower(scheme) {
	case "ss":
		return parseShadowsocksURI(line)
	case "vless":
		return parseVLESSURI(line)
	case "hysteria2", "hy2":
		return parseHysteria2URI(line)
	case "anytls":
		return parseAnyTLSURI(line)
	case "trojan":
		return parseTrojanURI(line)
	case "vmess":
		return parseVMessURI(line)
	default:
		return nil, fmt.Errorf("unsupported share link scheme %q", scheme)
	}
}

func parseShadowsocksURI(line string) (map[string]any, error) {
	rest := strings.TrimPrefix(line, "ss://")
	name := ""
	if i := strings.LastIndex(rest, "#"); i >= 0 {
		name, _ = url.PathUnescape(rest[i+1:])
		rest = rest[:i]
	}
	// Plugin parameters (SIP002 query string) are accepted but not carried
	// into the Clash proxy.
	if i := strings.Index(rest, "?"); i >= 0 {
		rest = rest[:i]
	}

	var userinfo, hostport string
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		userinfo, hostport = rest[:at], rest[at+1:]
	} else {
		// Legacy form: the whole "method:password@host:port" is base64 encoded.
		decoded, err := decodeBase64(rest)
		if err != nil {
			return nil, fmt.Errorf("invalid ss link encoding: %w", err)
		}
		parts := strings.SplitN(string(decoded), "@", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid ss link: missing host")
		}
		userinfo, hostport = parts[0], parts[1]
	}
	method, password, err := splitSSUserinfo(userinfo)
	if err != nil {
		return nil, err
	}
	server, port, err := splitHostPort(hostport)
	if err != nil {
		return nil, fmt.Errorf("invalid ss link: %w", err)
	}
	return map[string]any{
		"name":     fallbackName(name, server, port),
		"type":     "ss",
		"server":   server,
		"port":     port,
		"cipher":   method,
		"password": password,
		"udp":      true,
	}, nil
}

// splitSSUserinfo accepts both the SIP002 base64url-encoded userinfo and the
// plain "method:password" form.
func splitSSUserinfo(userinfo string) (string, string, error) {
	if !strings.Contains(userinfo, ":") {
		decoded, err := decodeBase64(userinfo)
		if err != nil {
			return "", "", fmt.Errorf("invalid ss userinfo encoding: %w", err)
		}
		userinfo = string(decoded)
	}
	method, password, found := strings.Cut(userinfo, ":")
	if !found || method == "" {
		return "", "", fmt.Errorf("invalid ss userinfo: missing method or password")
	}
	if decoded, err := url.PathUnescape(password); err == nil {
		password = decoded
	}
	return method, password, nil
}

func parseVLESSURI(line string) (map[string]any, error) {
	u, err := url.Parse(line)
	if err != nil {
		return nil, fmt.Errorf("invalid vless link: %w", err)
	}
	server, port, err := splitHostPort(u.Host)
	if err != nil {
		return nil, fmt.Errorf("invalid vless link: %w", err)
	}
	uuid := u.User.Username()
	if uuid == "" {
		return nil, fmt.Errorf("invalid vless link: missing uuid")
	}
	q := u.Query()
	proxy := map[string]any{
		"name":    fallbackName(u.Fragment, server, port),
		"type":    "vless",
		"server":  server,
		"port":    port,
		"uuid":    uuid,
		"udp":     true,
		"network": queryDefault(q, "type", "tcp"),
	}
	security := strings.ToLower(q.Get("security"))
	switch security {
	case "reality":
		proxy["tls"] = true
		proxy["servername"] = q.Get("sni")
		reality := map[string]any{"public-key": q.Get("pbk")}
		if sid := q.Get("sid"); sid != "" {
			reality["short-id"] = sid
		}
		proxy["reality-opts"] = reality
	case "tls":
		proxy["tls"] = true
		if sni := q.Get("sni"); sni != "" {
			proxy["servername"] = sni
		}
	default:
		proxy["tls"] = false
	}
	if flow := q.Get("flow"); flow != "" {
		proxy["flow"] = flow
	}
	if fp := q.Get("fp"); fp != "" {
		proxy["client-fingerprint"] = fp
	}
	applyTransport(proxy, q)
	return proxy, nil
}

func parseHysteria2URI(line string) (map[string]any, error) {
	u, err := url.Parse(line)
	if err != nil {
		return nil, fmt.Errorf("invalid hysteria2 link: %w", err)
	}
	server, port, err := splitHostPort(u.Host)
	if err != nil {
		return nil, fmt.Errorf("invalid hysteria2 link: %w", err)
	}
	password := u.User.Username()
	if password == "" {
		return nil, fmt.Errorf("invalid hysteria2 link: missing password")
	}
	q := u.Query()
	proxy := map[string]any{
		"name":     fallbackName(u.Fragment, server, port),
		"type":     "hysteria2",
		"server":   server,
		"port":     port,
		"password": password,
		"sni":      q.Get("sni"),
	}
	if queryBool(q, "insecure") {
		proxy["skip-cert-verify"] = true
	}
	if obfs := q.Get("obfs"); obfs != "" {
		proxy["obfs"] = obfs
		if obfsPassword := q.Get("obfs-password"); obfsPassword != "" {
			proxy["obfs-password"] = obfsPassword
		}
	}
	if ports := q.Get("mport"); ports != "" {
		proxy["ports"] = ports
	}
	return proxy, nil
}

func parseAnyTLSURI(line string) (map[string]any, error) {
	u, err := url.Parse(line)
	if err != nil {
		return nil, fmt.Errorf("invalid anytls link: %w", err)
	}
	server, port, err := splitHostPort(u.Host)
	if err != nil {
		return nil, fmt.Errorf("invalid anytls link: %w", err)
	}
	password := u.User.Username()
	if password == "" {
		return nil, fmt.Errorf("invalid anytls link: missing password")
	}
	q := u.Query()
	proxy := map[string]any{
		"name":     fallbackName(u.Fragment, server, port),
		"type":     "anytls",
		"server":   server,
		"port":     port,
		"password": password,
		"sni":      q.Get("sni"),
		"udp":      true,
	}
	if queryBool(q, "insecure") {
		proxy["skip-cert-verify"] = true
	}
	return proxy, nil
}

func parseTrojanURI(line string) (map[string]any, error) {
	u, err := url.Parse(line)
	if err != nil {
		return nil, fmt.Errorf("invalid trojan link: %w", err)
	}
	server, port, err := splitHostPort(u.Host)
	if err != nil {
		return nil, fmt.Errorf("invalid trojan link: %w", err)
	}
	password := u.User.Username()
	if password == "" {
		return nil, fmt.Errorf("invalid trojan link: missing password")
	}
	q := u.Query()
	sni := q.Get("sni")
	if sni == "" {
		sni = q.Get("peer")
	}
	proxy := map[string]any{
		"name":     fallbackName(u.Fragment, server, port),
		"type":     "trojan",
		"server":   server,
		"port":     port,
		"password": password,
		"sni":      sni,
		"udp":      true,
		"network":  queryDefault(q, "type", "tcp"),
	}
	if queryBool(q, "allowInsecure") || queryBool(q, "insecure") {
		proxy["skip-cert-verify"] = true
	}
	if fp := q.Get("fp"); fp != "" {
		proxy["client-fingerprint"] = fp
	}
	applyTransport(proxy, q)
	return proxy, nil
}

// vmessShare is the JSON document inside a vmess:// share link.
type vmessShare struct {
	PS   string `json:"ps"`
	Add  string `json:"add"`
	Port any    `json:"port"`
	ID   string `json:"id"`
	Aid  any    `json:"aid"`
	Scy  string `json:"scy"`
	Net  string `json:"net"`
	Host string `json:"host"`
	Path string `json:"path"`
	TLS  string `json:"tls"`
	SNI  string `json:"sni"`
	FP   string `json:"fp"`
}

func parseVMessURI(line string) (map[string]any, error) {
	raw := strings.TrimPrefix(line, "vmess://")
	decoded, err := decodeBase64(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid vmess link encoding: %w", err)
	}
	var doc vmessShare
	if err := json.Unmarshal(decoded, &doc); err != nil {
		return nil, fmt.Errorf("invalid vmess link payload: %w", err)
	}
	if doc.Add == "" || doc.ID == "" {
		return nil, fmt.Errorf("invalid vmess link: missing server or uuid")
	}
	port, err := looseInt(doc.Port)
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid vmess link: bad port %v", doc.Port)
	}
	alterID, err := looseInt(doc.Aid)
	if err != nil {
		alterID = 0
	}
	cipher := doc.Scy
	if cipher == "" {
		cipher = "auto"
	}
	network := doc.Net
	if network == "" {
		network = "tcp"
	}
	proxy := map[string]any{
		"name":       fallbackName(doc.PS, doc.Add, port),
		"type":       "vmess",
		"server":     doc.Add,
		"port":       port,
		"uuid":       doc.ID,
		"alterId":    alterID,
		"cipher":     cipher,
		"udp":        true,
		"network":    network,
		"tls":        doc.TLS == "tls",
		"servername": doc.SNI,
	}
	if doc.FP != "" {
		proxy["client-fingerprint"] = doc.FP
	}
	switch network {
	case "tcp":
	case "ws":
		ws := map[string]any{}
		if doc.Path != "" {
			ws["path"] = doc.Path
		}
		if doc.Host != "" {
			ws["headers"] = map[string]any{"Host": doc.Host}
		}
		proxy["ws-opts"] = ws
	case "grpc":
		if doc.Path != "" {
			proxy["grpc-opts"] = map[string]any{"grpc-service-name": doc.Path}
		}
	default:
		return nil, fmt.Errorf("unsupported vmess network %q", network)
	}
	return proxy, nil
}

// applyTransport maps the "type" query parameter (tcp / ws / grpc) plus its
// companions (path, host, serviceName) onto a Clash proxy's transport options.
func applyTransport(proxy map[string]any, q url.Values) {
	switch proxy["network"] {
	case "ws":
		ws := map[string]any{}
		if path := q.Get("path"); path != "" {
			ws["path"] = path
		}
		if host := q.Get("host"); host != "" {
			ws["headers"] = map[string]any{"Host": host}
		}
		proxy["ws-opts"] = ws
	case "grpc":
		if service := q.Get("serviceName"); service != "" {
			proxy["grpc-opts"] = map[string]any{"grpc-service-name": service}
		}
	}
}

func splitHostPort(hostport string) (string, int, error) {
	server, rawPort, err := net.SplitHostPort(hostport)
	if err != nil {
		return "", 0, fmt.Errorf("bad address %q", hostport)
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil || port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("bad port %q", rawPort)
	}
	if server == "" {
		return "", 0, fmt.Errorf("missing server address")
	}
	return server, port, nil
}

func fallbackName(name, server string, port int) string {
	if name != "" {
		return name
	}
	return net.JoinHostPort(server, strconv.Itoa(port))
}

func queryDefault(q url.Values, key, fallback string) string {
	if v := q.Get(key); v != "" {
		return v
	}
	return fallback
}

func queryBool(q url.Values, key string) bool {
	v := strings.ToLower(q.Get(key))
	return v == "1" || v == "true"
}

func looseInt(v any) (int, error) {
	switch n := v.(type) {
	case nil:
		return 0, nil
	case float64:
		return int(n), nil
	case string:
		if n == "" {
			return 0, nil
		}
		return strconv.Atoi(n)
	case json.Number:
		asInt, err := n.Int64()
		return int(asInt), err
	default:
		return 0, fmt.Errorf("not a number: %v", v)
	}
}

func decodeBase64(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	for _, enc := range []*base64.Encoding{
		base64.RawURLEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.StdEncoding,
	} {
		if decoded, err := enc.DecodeString(s); err == nil {
			return decoded, nil
		}
	}
	return nil, errors.New("not base64")
}
