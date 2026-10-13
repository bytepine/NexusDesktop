// Copyright byteyang. All Rights Reserved.

package unreal

// 三端协议时序。数值由工作区 Script/check_proxy_contract.py 对账，改动须三端一起改。
const (
	SCAN_PORT_START         = 45000
	SCAN_PORT_END           = 45100
	SCAN_CONCURRENCY        = 20
	PROBE_MAX_BYTES         = 65_536
	TOOLS_CALL_TIMEOUT_MS   = 120_000
	FULL_SCAN_EVERY_N_TICKS = 6
	WS_KEEPALIVE_BUSY_MS    = 5_000
	WS_KEEPALIVE_IDLE_MS    = 15_000
)

// MaintainAction 是一轮连接维护的动作。
type MaintainAction int

const (
	// MaintainSkip 有挂起请求，连接正在被使用，本轮不做探测。
	MaintainSkip MaintainAction = iota
	// MaintainProbe 只探测当前端口。
	MaintainProbe
	// MaintainFullScan 全端口扫描。
	MaintainFullScan
)

// MaintainDecision 是 decideMaintain 的结果。
type MaintainDecision struct {
	Action MaintainAction
	// NextCountdown：Skip 时为原倒计时；Probe 时为减一后的值；FullScan 时为 FULL_SCAN_EVERY_N_TICKS。
	NextCountdown int
}

// decideMaintain 决定本轮维护动作。wsOpen 须已含已绑定端口（connectedPort > 0）。
func decideMaintain(wsOpen bool, pending int, countdown int) MaintainDecision {
	if wsOpen && pending > 0 {
		return MaintainDecision{Action: MaintainSkip, NextCountdown: countdown}
	}
	if wsOpen && countdown > 0 {
		return MaintainDecision{Action: MaintainProbe, NextCountdown: countdown - 1}
	}
	return MaintainDecision{Action: MaintainFullScan, NextCountdown: FULL_SCAN_EVERY_N_TICKS}
}
