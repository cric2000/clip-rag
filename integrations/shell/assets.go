package shell

import _ "embed"

//go:embed cliprag.bash
var Bash string

//go:embed cliprag.zsh
var Zsh string
