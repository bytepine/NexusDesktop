// Copyright byteyang. All Rights Reserved.

package unreal

import (
	"net"
	"strconv"
	"strings"
)

const LoopbackHost = "127.0.0.1"

// RemoteUnreal 是显式配置的远程 UE（不扫端口区间）。
type RemoteUnreal struct {
	Host      string
	McpPort   int
	AuthToken string
}

func NormalizeHost(host string) string {
	h := strings.TrimSpace(host)
	if h == "" || strings.EqualFold(h, "localhost") || h == "::1" {
		return LoopbackHost
	}
	return h
}

func InstanceKey(host string, port int) string {
	return NormalizeHost(host) + ":" + strconv.Itoa(port)
}

// ParseRemoteText 每行 host:mcpPort [token...]；token 可省略。
func ParseRemoteText(text string) []RemoteUnreal {
	var out []RemoteUnreal
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		addr := t
		token := ""
		if sp := strings.IndexByte(t, ' '); sp > 0 {
			addr = strings.TrimSpace(t[:sp])
			token = strings.TrimSpace(t[sp+1:])
		}
		colon := strings.LastIndexByte(addr, ':')
		if colon <= 0 {
			continue
		}
		host := NormalizeHost(addr[:colon])
		port, err := strconv.Atoi(addr[colon+1:])
		if err != nil || host == LoopbackHost || port < 1024 || port > 65535 {
			continue
		}
		out = append(out, RemoteUnreal{Host: host, McpPort: port, AuthToken: token})
	}
	return out
}

// LanIPv4 是一块已启用、非 loopback、非链路本地的 IPv4。
type LanIPv4 struct {
	Name    string
	Address string
}

// ListLanIPv4 列出可写入跨机 url 的网卡 IPv4。
func ListLanIPv4() []LanIPv4 {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []LanIPv4
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok || ipNet.IP.IsLoopback() || ipNet.IP.IsLinkLocalUnicast() {
				continue
			}
			ip4 := ipNet.IP.To4()
			if ip4 == nil {
				continue
			}
			out = append(out, LanIPv4{Name: iface.Name, Address: ip4.String()})
		}
	}
	return out
}

func FirstLanIPv4() string {
	list := ListLanIPv4()
	if len(list) == 0 {
		return ""
	}
	return list[0].Address
}

// CopyHostChoices 返回复制 mcp.json 用的 host。
// auto 非空则无需选择；choices 非空时由 UI 挑选（含「本机」）。
func CopyHostChoices(listenLan bool) (auto string, choices []LanIPv4) {
	if !listenLan {
		return LoopbackHost, nil
	}
	lan := ListLanIPv4()
	if len(lan) == 0 {
		return LoopbackHost, nil
	}
	if len(lan) == 1 {
		return lan[0].Address, nil
	}
	return "", append([]LanIPv4{{Name: "本机", Address: LoopbackHost}}, lan...)
}

func LanAddrLabel(a LanIPv4) string {
	if a.Name == "" {
		return a.Address
	}
	return a.Name + " " + a.Address
}

func McpDisplayHost(listenLan bool) string {
	if !listenLan {
		return LoopbackHost
	}
	if ip := FirstLanIPv4(); ip != "" {
		return ip
	}
	return LoopbackHost
}
