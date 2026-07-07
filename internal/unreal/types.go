// Copyright byteyang. All Rights Reserved.

package unreal

// InstanceInfo 描述一个已加载 NexusLink 的 UE 实例。
type InstanceInfo struct {
	Port          int
	WsPort        int
	ProjectName   string
	EngineVersion string
	// NetRole: DedicatedServer / ListenServer / Client / Standalone / Editor
	NetRole       string
	ToolsListMode string
}
