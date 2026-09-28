package singbox

import "fmt"

// ProxyToOutbound converts a Clash Meta (mihomo) proxy mapping into a sing-box
// outbound mapping without a tag. It is the client-side counterpart of the
// share-link importer and is used to dial an external custom-node line as a
// chain exit. Unsupported types or missing required fields return an error so
// the caller can fall back to a direct outbound instead of failing the whole
// config.
func ProxyToOutbound(proxy map[string]any) (map[string]any, error) {
	clashType := SettingString(proxy, "type")
	server := SettingString(proxy, "server")
	port, ok := settingInt(proxy, "port")
	if server == "" || !ok || port < 1 || port > 65535 {
		return nil, fmt.Errorf("%w: custom proxy has no valid server/port", ErrUnrenderable)
	}
	switch clashType {
	case "ss":
		return shadowsocksProxyOutbound(proxy, server, int(port))
	case "vless":
		return vlessProxyOutbound(proxy, server, int(port))
	case "trojan":
		return trojanProxyOutbound(proxy, server, int(port))
	case "vmess":
		return vmessProxyOutbound(proxy, server, int(port))
	case "hysteria2":
		return hysteria2ProxyOutbound(proxy, server, int(port))
	case "anytls":
		return anytlsProxyOutbound(proxy, server, int(port))
	case "socks5":
		return socksProxyOutbound(proxy, server, int(port))
	case "http":
		return httpProxyOutbound(proxy, server, int(port))
	default:
		return nil, fmt.Errorf("%w: unsupported custom proxy type %q", ErrUnrenderable, clashType)
	}
}

// OutboundSupported reports whether a Clash proxy type can be rendered into a
// sing-box chain outbound. The API uses it to grey out unsupported entries.
func OutboundSupported(clashType string) bool {
	switch clashType {
	case "ss", "vless", "trojan", "vmess", "hysteria2", "anytls", "socks5", "http":
		return true
	default:
		return false
	}
}

func shadowsocksProxyOutbound(proxy map[string]any, server string, port int) (map[string]any, error) {
	method := SettingString(proxy, "cipher")
	password := SettingString(proxy, "password")
	if method == "" || password == "" {
		return nil, fmt.Errorf("%w: ss proxy requires cipher and password", ErrUnrenderable)
	}
	return map[string]any{
		"type":        ProtocolShadowsocks,
		"server":      server,
		"server_port": port,
		"method":      method,
		"password":    password,
	}, nil
}

func vlessProxyOutbound(proxy map[string]any, server string, port int) (map[string]any, error) {
	uuid := SettingString(proxy, "uuid")
	if uuid == "" {
		return nil, fmt.Errorf("%w: vless proxy requires uuid", ErrUnrenderable)
	}
	outbound := map[string]any{
		"type":        ProtocolVLESS,
		"server":      server,
		"server_port": port,
		"uuid":        uuid,
	}
	if flow := SettingString(proxy, "flow"); flow != "" {
		outbound["flow"] = flow
	}
	if tls := v2rayTLS(proxy); tls != nil {
		outbound["tls"] = tls
	}
	if transport := v2rayTransport(proxy); transport != nil {
		outbound["transport"] = transport
	}
	return outbound, nil
}

func trojanProxyOutbound(proxy map[string]any, server string, port int) (map[string]any, error) {
	password := SettingString(proxy, "password")
	if password == "" {
		return nil, fmt.Errorf("%w: trojan proxy requires password", ErrUnrenderable)
	}
	tls := map[string]any{"enabled": true}
	if serverName := proxyServerName(proxy); serverName != "" {
		tls["server_name"] = serverName
	}
	if proxyBool(proxy, "skip-cert-verify") {
		tls["insecure"] = true
	}
	outbound := map[string]any{
		"type":        "trojan",
		"server":      server,
		"server_port": port,
		"password":    password,
		"tls":         tls,
	}
	if transport := v2rayTransport(proxy); transport != nil {
		outbound["transport"] = transport
	}
	return outbound, nil
}

func vmessProxyOutbound(proxy map[string]any, server string, port int) (map[string]any, error) {
	uuid := SettingString(proxy, "uuid")
	if uuid == "" {
		return nil, fmt.Errorf("%w: vmess proxy requires uuid", ErrUnrenderable)
	}
	security := SettingString(proxy, "cipher")
	if security == "" {
		security = "auto"
	}
	outbound := map[string]any{
		"type":        "vmess",
		"server":      server,
		"server_port": port,
		"uuid":        uuid,
		"security":    security,
	}
	if alterID, ok := settingInt(proxy, "alterId"); ok {
		outbound["alter_id"] = int(alterID)
	} else if alterID, ok := settingInt(proxy, "alter-id"); ok {
		outbound["alter_id"] = int(alterID)
	}
	if tls := v2rayTLS(proxy); tls != nil {
		outbound["tls"] = tls
	}
	if transport := v2rayTransport(proxy); transport != nil {
		outbound["transport"] = transport
	}
	return outbound, nil
}

func hysteria2ProxyOutbound(proxy map[string]any, server string, port int) (map[string]any, error) {
	password := SettingString(proxy, "password")
	if password == "" {
		return nil, fmt.Errorf("%w: hysteria2 proxy requires password", ErrUnrenderable)
	}
	tls := map[string]any{"enabled": true}
	if serverName := proxyServerName(proxy); serverName != "" {
		tls["server_name"] = serverName
	}
	if proxyBool(proxy, "skip-cert-verify") {
		tls["insecure"] = true
	}
	outbound := map[string]any{
		"type":        ProtocolHysteria2,
		"server":      server,
		"server_port": port,
		"password":    password,
		"tls":         tls,
	}
	if obfs := SettingString(proxy, "obfs"); obfs == "salamander" {
		if obfsPassword := SettingString(proxy, "obfs-password"); obfsPassword != "" {
			outbound["obfs"] = map[string]any{"type": "salamander", "password": obfsPassword}
		}
	}
	return outbound, nil
}

func anytlsProxyOutbound(proxy map[string]any, server string, port int) (map[string]any, error) {
	password := SettingString(proxy, "password")
	if password == "" {
		return nil, fmt.Errorf("%w: anytls proxy requires password", ErrUnrenderable)
	}
	tls := map[string]any{"enabled": true}
	if serverName := proxyServerName(proxy); serverName != "" {
		tls["server_name"] = serverName
	}
	if proxyBool(proxy, "skip-cert-verify") {
		tls["insecure"] = true
	}
	return map[string]any{
		"type":        ProtocolAnyTLS,
		"server":      server,
		"server_port": port,
		"password":    password,
		"tls":         tls,
	}, nil
}

func socksProxyOutbound(proxy map[string]any, server string, port int) (map[string]any, error) {
	outbound := map[string]any{
		"type":        ProtocolSocks,
		"server":      server,
		"server_port": port,
	}
	if username := SettingString(proxy, "username"); username != "" {
		outbound["username"] = username
	}
	if password := SettingString(proxy, "password"); password != "" {
		outbound["password"] = password
	}
	return outbound, nil
}

// httpProxyOutbound converts a Clash `http` proxy into a plaintext HTTP proxy
// outbound, adding a TLS block when the proxy sets `tls: true`.
func httpProxyOutbound(proxy map[string]any, server string, port int) (map[string]any, error) {
	outbound := map[string]any{
		"type":        ProtocolHTTP,
		"server":      server,
		"server_port": port,
	}
	if username := SettingString(proxy, "username"); username != "" {
		outbound["username"] = username
	}
	if password := SettingString(proxy, "password"); password != "" {
		outbound["password"] = password
	}
	if proxyBool(proxy, "tls") {
		tls := map[string]any{"enabled": true}
		if serverName := proxyServerName(proxy); serverName != "" {
			tls["server_name"] = serverName
		}
		if proxyBool(proxy, "skip-cert-verify") {
			tls["insecure"] = true
		}
		outbound["tls"] = tls
	}
	return outbound, nil
}

// v2rayTLS builds the client TLS block shared by the vless/trojan/vmess
// outbounds. It returns nil when the proxy does not use TLS. Reality options
// map to tls.reality; the server name comes from "servername" (vless) or "sni".
func v2rayTLS(proxy map[string]any) map[string]any {
	if !proxyBool(proxy, "tls") {
		return nil
	}
	tls := map[string]any{"enabled": true}
	if serverName := proxyServerName(proxy); serverName != "" {
		tls["server_name"] = serverName
	}
	if proxyBool(proxy, "skip-cert-verify") {
		tls["insecure"] = true
	}
	if reality := SettingMap(proxy, "reality-opts"); reality != nil {
		realityOut := map[string]any{"enabled": true}
		if publicKey := SettingString(reality, "public-key"); publicKey != "" {
			realityOut["public_key"] = publicKey
		}
		if shortID := SettingString(reality, "short-id"); shortID != "" {
			realityOut["short_id"] = shortID
		}
		tls["reality"] = realityOut
	}
	return tls
}

// v2rayTransport maps the Clash transport options (network + ws-opts /
// grpc-opts) onto a sing-box v2ray transport. Plain TCP maps to nil.
func v2rayTransport(proxy map[string]any) map[string]any {
	switch SettingString(proxy, "network") {
	case "ws":
		ws := SettingMap(proxy, "ws-opts")
		transport := map[string]any{"type": "ws"}
		if path := SettingString(ws, "path"); path != "" {
			transport["path"] = path
		}
		if headers := SettingMap(ws, "headers"); len(headers) > 0 {
			transport["headers"] = headers
		}
		return transport
	case "grpc":
		grpc := SettingMap(proxy, "grpc-opts")
		transport := map[string]any{"type": "grpc"}
		if service := SettingString(grpc, "grpc-service-name"); service != "" {
			transport["service_name"] = service
		}
		return transport
	default:
		return nil
	}
}

func proxyServerName(proxy map[string]any) string {
	if serverName := SettingString(proxy, "servername"); serverName != "" {
		return serverName
	}
	return SettingString(proxy, "sni")
}

func proxyBool(proxy map[string]any, key string) bool {
	value, _ := proxy[key].(bool)
	return value
}
