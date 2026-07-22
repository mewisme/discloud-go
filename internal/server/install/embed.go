// Package install holds installer script templates served by the API.
package install

import _ "embed"

//go:embed install.sh
var ScriptSH string

//go:embed install.ps1
var ScriptPS1 string
