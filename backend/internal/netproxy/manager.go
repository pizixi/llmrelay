// Package netproxy 管理直连和 SOCKS5 HTTP 传输。
package netproxy

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"llmrelay/backend/internal/domain"
)

func newBaseTransport() *http.Transport {
	return &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		MaxConnsPerHost:       100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 120 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
}

var httpClient = &http.Client{
	Timeout:   600 * time.Second,
	Transport: newBaseTransport(),
}

var streamHTTPClient = &http.Client{
	Timeout:   0,
	Transport: newBaseTransport(),
}

// ======================== SOCKS5 代理 ========================

type Socks5Proxy = domain.Socks5Proxy

// 用于移除 Claude Code 系统消息计费头的正则表达式。
var reBillingHeader = regexp.MustCompile(`(?m)^x-anthropic-billing-header:\s*.*$`)

func socks5Dial(proxy Socks5Proxy) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, target string) (net.Conn, error) {
		conn, err := net.DialTimeout("tcp", proxy.Addr, 10*time.Second)
		if err != nil {
			return nil, fmt.Errorf("socks5 connect to %s: %w", proxy.Addr, err)
		}
		deadline := time.Now().Add(15 * time.Second)
		conn.SetDeadline(deadline)

		// 认证方法协商
		auth := byte(0x00) // no auth
		if proxy.Username != "" {
			auth = 0x02 // username/password
		}
		if _, err := conn.Write([]byte{0x05, 0x01, auth}); err != nil {
			conn.Close()
			return nil, fmt.Errorf("socks5 handshake write: %w", err)
		}
		buf := make([]byte, 2)
		if _, err := io.ReadFull(conn, buf); err != nil {
			conn.Close()
			return nil, fmt.Errorf("socks5 handshake read: %w", err)
		}
		if buf[0] != 0x05 {
			conn.Close()
			return nil, fmt.Errorf("socks5: not socks5 protocol")
		}

		// 用户名/密码认证
		if buf[1] == 0x02 {
			if proxy.Username == "" {
				conn.Close()
				return nil, fmt.Errorf("socks5: server requires auth but no credentials")
			}
			ulen := len(proxy.Username)
			plen := len(proxy.Password)
			authBuf := make([]byte, 3+ulen+plen)
			authBuf[0] = 0x01
			authBuf[1] = byte(ulen)
			copy(authBuf[2:], proxy.Username)
			authBuf[2+ulen] = byte(plen)
			copy(authBuf[3+ulen:], proxy.Password)
			if _, err := conn.Write(authBuf); err != nil {
				conn.Close()
				return nil, fmt.Errorf("socks5 auth write: %w", err)
			}
			authResp := make([]byte, 2)
			if _, err := io.ReadFull(conn, authResp); err != nil {
				conn.Close()
				return nil, fmt.Errorf("socks5 auth read: %w", err)
			}
			if authResp[1] != 0x00 {
				conn.Close()
				return nil, fmt.Errorf("socks5: auth failed")
			}
		} else if buf[1] != 0x00 {
			conn.Close()
			return nil, fmt.Errorf("socks5: unsupported auth method 0x%02x", buf[1])
		}

		// CONNECT 请求
		host, portStr, err := net.SplitHostPort(target)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("socks5: invalid target %s: %w", target, err)
		}
		port := 0
		fmt.Sscanf(portStr, "%d", &port)

		req := []byte{0x05, 0x01, 0x00} // VER, CMD=CONNECT, RSV
		ip := net.ParseIP(host)
		if ip != nil {
			if ip4 := ip.To4(); ip4 != nil {
				req = append(req, 0x01) // IPv4
				req = append(req, ip4...)
			} else {
				req = append(req, 0x04) // IPv6
				req = append(req, ip.To16()...)
			}
		} else {
			if len(host) > 255 {
				conn.Close()
				return nil, fmt.Errorf("socks5: hostname too long")
			}
			req = append(req, 0x03) // Domain
			req = append(req, byte(len(host)))
			req = append(req, []byte(host)...)
		}
		req = append(req, byte(port>>8), byte(port))

		if _, err := conn.Write(req); err != nil {
			conn.Close()
			return nil, fmt.Errorf("socks5 connect write: %w", err)
		}

		// 读取响应
		resp := make([]byte, 4)
		if _, err := io.ReadFull(conn, resp); err != nil {
			conn.Close()
			return nil, fmt.Errorf("socks5 connect read: %w", err)
		}
		if resp[1] != 0x00 {
			conn.Close()
			return nil, fmt.Errorf("socks5: connect failed, status 0x%02x", resp[1])
		}

		// 读取绑定地址
		switch resp[3] {
		case 0x01: // IPv4
			if _, err := io.ReadFull(conn, make([]byte, 4+2)); err != nil {
				conn.Close()
				return nil, fmt.Errorf("socks5: read bind ipv4: %w", err)
			}
		case 0x03: // Domain
			dlen := make([]byte, 1)
			if _, err := io.ReadFull(conn, dlen); err != nil {
				conn.Close()
				return nil, fmt.Errorf("socks5: read bind domain len: %w", err)
			}
			if _, err := io.ReadFull(conn, make([]byte, int(dlen[0])+2)); err != nil {
				conn.Close()
				return nil, fmt.Errorf("socks5: read bind domain: %w", err)
			}
		case 0x04: // IPv6
			if _, err := io.ReadFull(conn, make([]byte, 16+2)); err != nil {
				conn.Close()
				return nil, fmt.Errorf("socks5: read bind ipv6: %w", err)
			}
		default:
			conn.Close()
			return nil, fmt.Errorf("socks5: unknown address type 0x%02x", resp[3])
		}

		conn.SetDeadline(time.Time{})
		return conn, nil
	}
}

var (
	socks5Proxies []Socks5Proxy
	activeSocks5  string // 启用的代理 Addr；空表示直连；特殊值表示轮询/429 切换
	socks5Mu      sync.RWMutex
)

const (
	socks5RR                      = "__round_robin__"
	socks5RateLimitSwitch         = "__rate_limit_switch__"
	socks5RateLimitSwitchNoDirect = "__rate_limit_switch_no_direct__"
)

var (
	socks5RRIndex        uint32
	socks5RateLimitIndex uint32 // 0 表示直连，1..n 表示 socks5Proxies[n-1]
)

type socks5ClientCacheKey struct {
	Addr     string
	Username string
	Password string
	Stream   bool
}

var socks5ClientCache = map[socks5ClientCacheKey]*http.Client{}

func getHTTPClient(stream bool) *http.Client {
	client, _ := getHTTPClientWithExit(stream)
	return client
}

func getHTTPClientForProxy(stream bool, proxyAddress string) (*http.Client, string) {
	proxyAddress = strings.TrimSpace(proxyAddress)
	socks5Mu.Lock()
	defer socks5Mu.Unlock()
	if proxyAddress == "" {
		if stream {
			return streamHTTPClient, "direct"
		}
		return httpClient, "direct"
	}
	for _, proxy := range socks5Proxies {
		if proxy.Addr == proxyAddress || proxy.Name == proxyAddress {
			return getHTTPClientForSocks5Locked(stream, proxy)
		}
	}
	// Validation normally prevents this path. Falling back to direct keeps a
	// stale proxy reference from taking the whole gateway offline while an
	// administrator repairs the configuration.
	return directHTTPClient(stream)
}

func getHTTPClientWithExit(stream bool) (*http.Client, string) {
	socks5Mu.Lock()
	defer socks5Mu.Unlock()

	if activeSocks5 == "" {
		if stream {
			return streamHTTPClient, "direct"
		}
		return httpClient, "direct"
	}

	var proxy Socks5Proxy

	if activeSocks5 == socks5RR {
		if len(socks5Proxies) == 0 {
			if stream {
				return streamHTTPClient, "direct"
			}
			return httpClient, "direct"
		}
		idx := atomic.AddUint32(&socks5RRIndex, 1) % uint32(len(socks5Proxies))
		proxy = socks5Proxies[idx]
	} else if activeSocks5 == socks5RateLimitSwitch || activeSocks5 == socks5RateLimitSwitchNoDirect {
		var ok bool
		proxy, ok = currentRateLimitProxyLocked(activeSocks5 == socks5RateLimitSwitch)
		if !ok {
			if stream {
				return streamHTTPClient, "direct"
			}
			return httpClient, "direct"
		}
	} else {
		var found bool
		for i := range socks5Proxies {
			if socks5Proxies[i].Addr == activeSocks5 {
				proxy = socks5Proxies[i]
				found = true
				break
			}
		}
		if !found {
			if stream {
				return streamHTTPClient, "direct"
			}
			return httpClient, "direct"
		}
	}

	return getHTTPClientForSocks5Locked(stream, proxy)
}

func directHTTPClient(stream bool) (*http.Client, string) {
	if stream {
		return streamHTTPClient, "direct"
	}
	return httpClient, "direct"
}

func getHTTPClientForSocks5Locked(stream bool, proxy Socks5Proxy) (*http.Client, string) {
	cacheKey := socks5ClientCacheKey{
		Addr: proxy.Addr, Username: proxy.Username, Password: proxy.Password, Stream: stream,
	}
	if cached := socks5ClientCache[cacheKey]; cached != nil {
		return cached, socks5ProxyLabel(proxy)
	}

	dial := socks5Dial(proxy)
	client := &http.Client{
		Timeout: map[bool]time.Duration{true: 0, false: 600 * time.Second}[stream],
		Transport: &http.Transport{
			DialContext:           dial,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   20,
			MaxConnsPerHost:       100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 120 * time.Second,
			ExpectContinueTimeout: time.Second,
		},
	}

	socks5ClientCache[cacheKey] = client
	return client, socks5ProxyLabel(proxy)
}

func closeSocks5ClientsLocked() {
	for _, client := range socks5ClientCache {
		client.CloseIdleConnections()
	}
	socks5ClientCache = map[socks5ClientCacheKey]*http.Client{}
}

func socks5ProxyLabel(proxy Socks5Proxy) string {
	if proxy.Name != "" {
		return proxy.Name + " (" + proxy.Addr + ")"
	}
	if proxy.Addr != "" {
		return proxy.Addr
	}
	return "direct"
}

func socks5ProxyLabelByAddrLocked(addr string) string {
	for i := range socks5Proxies {
		if socks5Proxies[i].Addr == addr {
			return socks5ProxyLabel(socks5Proxies[i])
		}
	}
	if addr != "" {
		return addr
	}
	return "direct"
}

func currentRateLimitProxyLocked(includeDirect bool) (Socks5Proxy, bool) {
	total := len(socks5Proxies)
	if includeDirect {
		total++
	}
	if total <= 0 {
		return Socks5Proxy{}, false
	}
	idx := int(atomic.LoadUint32(&socks5RateLimitIndex)) % total
	if includeDirect && idx == 0 {
		return Socks5Proxy{}, false
	}
	if includeDirect {
		return socks5Proxies[idx-1], true
	}
	return socks5Proxies[idx], true
}

func socks5ExitLabelLocked(idx int, includeDirect bool) string {
	if includeDirect && idx <= 0 {
		return "direct"
	}
	proxyIdx := idx
	if includeDirect {
		proxyIdx = idx - 1
	}
	if proxyIdx < 0 || proxyIdx >= len(socks5Proxies) {
		return "direct"
	}
	proxy := socks5Proxies[proxyIdx]
	if proxy.Name != "" {
		return proxy.Name + " (" + proxy.Addr + ")"
	}
	return proxy.Addr
}

func currentSocks5ExitLabel() string {
	socks5Mu.RLock()
	defer socks5Mu.RUnlock()
	if activeSocks5 == "" {
		return "direct"
	}
	if activeSocks5 != socks5RateLimitSwitch && activeSocks5 != socks5RateLimitSwitchNoDirect {
		if activeSocks5 == socks5RR {
			return "round-robin"
		}
		return socks5ProxyLabelByAddrLocked(activeSocks5)
	}
	includeDirect := activeSocks5 == socks5RateLimitSwitch
	total := len(socks5Proxies)
	if includeDirect {
		total++
	}
	if total <= 0 {
		return "direct"
	}
	idx := int(atomic.LoadUint32(&socks5RateLimitIndex)) % total
	return socks5ExitLabelLocked(idx, includeDirect)
}

func socks5RateLimitAttemptCount() int {
	socks5Mu.RLock()
	defer socks5Mu.RUnlock()
	switch activeSocks5 {
	case socks5RateLimitSwitch:
		return len(socks5Proxies) + 1
	case socks5RateLimitSwitchNoDirect:
		if len(socks5Proxies) == 0 {
			return 1
		}
		return len(socks5Proxies)
	default:
		return 1
	}
}

func rotateSocks5OnRateLimit() {
	rotateSocks5OnRateLimitFor("upstream")
}

func rotateSocks5OnRateLimitFor(source string) {
	socks5Mu.Lock()
	defer socks5Mu.Unlock()
	source = strings.TrimSpace(source)
	if source == "" {
		source = "upstream"
	}
	includeDirect := activeSocks5 == socks5RateLimitSwitch
	if activeSocks5 != socks5RateLimitSwitch && activeSocks5 != socks5RateLimitSwitchNoDirect {
		return
	}
	total := len(socks5Proxies)
	if includeDirect {
		total++
	}
	if total <= 1 {
		if includeDirect {
			log.Printf("[限流代理切换] %s 返回 429，仅有直连出口可用", source)
		} else {
			log.Printf("[限流代理切换] %s 返回 429，没有可用的 SOCKS5 出口", source)
		}
		return
	}
	oldIdx := int(atomic.LoadUint32(&socks5RateLimitIndex)) % total
	nextIdx := (oldIdx + 1) % total
	atomic.StoreUint32(&socks5RateLimitIndex, uint32(nextIdx))
	log.Printf("[限流代理切换] %s 返回 429：%s -> %s", source, socks5ExitLabelLocked(oldIdx, includeDirect), socks5ExitLabelLocked(nextIdx, includeDirect))
}
