package netproxy

import (
	"net/http"
	"sync/atomic"

	"llmrelay/backend/internal/domain"
)

const (
	ModeRoundRobin              = socks5RR
	ModeRateLimitSwitch         = socks5RateLimitSwitch
	ModeRateLimitSwitchNoDirect = socks5RateLimitSwitchNoDirect
)

func Client(stream bool) *http.Client { return getHTTPClient(stream) }

func ClientWithExit(stream bool) (*http.Client, string) {
	return getHTTPClientWithExit(stream)
}

func RateLimitAttemptCount() int { return socks5RateLimitAttemptCount() }

func RotateOnRateLimit() { rotateSocks5OnRateLimit() }

func RotateOnRateLimitFor(source string) { rotateSocks5OnRateLimitFor(source) }

func CurrentExitLabel() string { return currentSocks5ExitLabel() }

// Configure 原子地替换代理配置，并关闭旧配置创建的空闲连接。
func Configure(proxies []domain.Socks5Proxy, active string) {
	socks5Mu.Lock()
	closeSocks5ClientsLocked()
	socks5Proxies = append([]domain.Socks5Proxy(nil), proxies...)
	activeSocks5 = active
	atomic.StoreUint32(&socks5RRIndex, 0)
	atomic.StoreUint32(&socks5RateLimitIndex, 0)
	socks5Mu.Unlock()
}

func Snapshot() ([]domain.Socks5Proxy, string) {
	socks5Mu.RLock()
	defer socks5Mu.RUnlock()
	return append([]domain.Socks5Proxy(nil), socks5Proxies...), activeSocks5
}

// CacheSize 返回当前按代理和请求模式复用的 HTTP 客户端数量。
func CacheSize() int {
	socks5Mu.RLock()
	defer socks5Mu.RUnlock()
	return len(socks5ClientCache)
}
