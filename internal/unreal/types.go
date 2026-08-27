// Copyright byteyang. All Rights Reserved.

package unreal

// InstanceInfo 描述一个已加载 NexusLink 的 UE 实例。
type InstanceInfo struct {
	Host          string
	Port          int
	WsPort        int
	ProjectName   string
	EngineVersion string
	// NetRole: DedicatedServer / ListenServer / Client / Standalone / Editor
	NetRole       string
	ToolsListMode string
	AuthToken     string
	// AuthRequired 对应 /status.authRequired；旧版无此字段则为 false，跳过 WS auth。
	AuthRequired bool
}
