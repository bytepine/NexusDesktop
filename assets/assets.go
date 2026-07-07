// Package assets 嵌入应用静态资源。
package assets

import _ "embed"

//go:embed icon.png
var IconPNG []byte
