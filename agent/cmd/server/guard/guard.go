package guard

import "embed"

// Lua holds the OpenResty guard modules and the vendored lua-resty-t1k
// client (Apache-2.0) that talks to the SafeLine detector.
//
//go:embed lua
var Lua embed.FS
