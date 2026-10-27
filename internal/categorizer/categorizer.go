package categorizer

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
)

var Categories = []string{"Git", "Linux", "Docker", "Kubernetes", "SQL", "JSON", "URLs", "Code", "Text"}
var sql = regexp.MustCompile(`(?is)^(select\s+.+\s+from\s+|insert\s+into\s+|update\s+\S+\s+set\s+|delete\s+from\s+|create\s+(table|index|view|database)\s+|alter\s+table\s+|drop\s+(table|index|view|database)\s+|with\s+\w+\s+as\s*\()`)
var code = regexp.MustCompile(`(?m)^(package\s+\w+\s*$|func\s+\w+\s*\(|def\s+\w+\s*\([^\n]*\)\s*:|class\s+\w+[^\n]*[{: ]\s*$|(?:export\s+)?(?:async\s+)?function\s+\w+\s*\(|(?:export\s+)?(?:const|let|var)\s+\w+\s*(?::[^=\n]+)?\s*=|import\s+(?:.*\s+from\s+['"]|\(|['"])|#include\s*[<"]|public\s+(?:static\s+)?(?:class|void)\s+)`)

// Classify uses explicit precedence: valid JSON container, URL, developer shell
// command, SQL, recognizable code, then Text. The original text is untouched.
func Classify(content string) string {
	s := strings.TrimSpace(content)
	if (strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[")) && json.Valid([]byte(s)) {
		return "JSON"
	}
	if u, e := url.Parse(s); e == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && !strings.ContainsAny(s, " \t\r\n") {
		return "URLs"
	}
	fields := strings.Fields(s)
	if len(fields) > 0 && !strings.Contains(s, "\n") {
		if fields[0] == "sudo" && len(fields) > 1 {
			fields = fields[1:]
		}
		if len(fields) > 1 {
			allowed := map[string]string{
				"git":     "status switch checkout add commit diff log show push pull fetch branch merge rebase reset restore clean clone init tag stash remote rev-parse cherry-pick revert worktree blame grep help config reflog bisect submodule",
				"gh":      "pr issue repo run workflow auth release api gist browse search status help",
				"docker":  "ps compose logs run exec build images image container start stop restart inspect pull push rm rmi volume network system info version login logout stats top cp context help",
				"podman":  "ps compose logs run exec build images image container start stop restart inspect pull push rm rmi volume network system info version login logout stats top cp help",
				"kubectl": "get describe apply delete edit logs exec port-forward config rollout scale expose create patch top cluster-info explain version help",
				"helm":    "list install upgrade uninstall repo search template lint show status history rollback package pull push dependency version help",
			}
			if commands, ok := allowed[fields[0]]; ok && (strings.HasPrefix(fields[1], "-") || strings.Contains(" "+commands+" ", " "+fields[1]+" ")) {
				switch fields[0] {
				case "git", "gh":
					return "Git"
				case "docker", "podman":
					return "Docker"
				default:
					return "Kubernetes"
				}
			}
		}
		// Restrict common words (find, cat, etc.) to avoid ordinary prose matches.
		switch fields[0] {
		case "apt", "apt-get", "systemctl", "journalctl", "lsof", "ss", "chmod", "chown", "ps", "kill", "killall", "df", "du", "lsblk", "ip", "curl", "wget", "grep", "rg", "sed", "awk", "ssh", "tar":
			if len(fields) > 1 {
				return "Linux"
			}
		}
	}
	if sql.MatchString(s) {
		return "SQL"
	}
	if code.MatchString(s) {
		return "Code"
	}
	return "Text"
}
