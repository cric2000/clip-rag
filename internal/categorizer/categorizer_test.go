package categorizer

import "testing"

func TestClassify(t *testing.T) {
	tests := []struct{ text, want string }{
		{"git status", "Git"}, {"  git switch main  ", "Git"}, {"gh pr list", "Git"}, {"I use Git every day", "Text"}, {"Git is useful", "Text"}, {"git is useful", "Text"}, {"docker is a tool", "Text"},
		{"docker compose up -d", "Docker"}, {"kubectl get pods", "Kubernetes"}, {"helm list", "Kubernetes"}, {"sudo apt update", "Linux"}, {"sudo systemctl restart nginx", "Linux"}, {"journalctl -u nginx", "Linux"}, {"lsof -i :3000", "Linux"},
		{"SELECT * FROM users;", "SQL"}, {"SELECT id,\n name\nFROM users\nWHERE id = 1;", "SQL"}, {"CREATE TABLE users (id INT);", "SQL"}, {"select your favorite color", "Text"},
		{`{"name":"git status"}`, "JSON"}, {`[1,{"x":2}]`, "JSON"}, {`{"broken":}`, "Text"}, {`"text"`, "Text"},
		{"https://github.com/example/repo", "URLs"}, {"http://localhost:3000", "URLs"}, {"https://example.com\nregular prose", "Text"},
		{"package main\n\nfunc main() {\n fmt.Println(1)\n}", "Code"}, {"def hello():\n    return 1", "Code"}, {"const answer: number = 42;", "Code"}, {"export function x() { return 2; }", "Code"}, {"ordinary prose", "Text"}, {"", "Text"},
	}
	for _, tt := range tests {
		t.Run(tt.want+tt.text, func(t *testing.T) {
			if got := Classify(tt.text); got != tt.want {
				t.Fatalf("got %s want %s", got, tt.want)
			}
		})
	}
}
