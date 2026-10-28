package shell

import "embed"

// Bash line editing is pinned to upstream c70c0e4. Core runtime files and their
// licenses are embedded so installed helpers need no network or extra checkout.
//
//go:embed vendor/blesh/ble.sh vendor/blesh/lib vendor/blesh/licenses vendor/blesh/contrib/scheme/default.bash
var BashRuntime embed.FS

const BashRuntimeVersion = "c70c0e4-35d184067e8f"
