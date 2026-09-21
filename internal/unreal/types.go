// Copyright byteyang. All Rights Reserved.

package unreal

// InstanceInfo 描述一个已加载 NexusLink 的 UE 实例。
type InstanceInfo struct {
	Host          string
	Port          int
	WsPort        int
	ProjectName   string
	EngineVersion string
	// NetRole: DedicatedServer / ListenServer / Client / Standalone / Editor（PIE 期间会变成 Standalone）
	NetRole string
	// HostKind: Editor / Game / DedicatedServer。不随 PIE 变化；选实例优先看此项。
	HostKind string
	// HasPlayWorld 为 nil 表示 /status 未返回该字段。
	HasPlayWorld *bool
	ToolsListMode string
	AuthToken     string
	// AuthRequired 对应 /status.authRequired；旧版无此字段则为 false，跳过 WS auth。
	AuthRequired bool
}
