package store

import "testing"

func TestPathToSlug(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/home/user/projects/piper", "home-user-projects-piper"},
		{"/home/user/projects/acme/server", "home-user-projects-acme-server"},
		{"/home/user/projects/acme/client", "home-user-projects-acme-client"},
		{"/home/user/My_Project", "home-user-my-project"},
		{"/tmp/test", "tmp-test"},
		{"/", ""},
		{"", ""},
		{"/path/with///multiple/slashes", "path-with-multiple-slashes"},
		{"/path/with spaces/dir", "path-with-spaces-dir"},
		{"/path/with.dots/dir", "path-with-dots-dir"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := PathToSlug(tt.path)
			if got != tt.want {
				t.Errorf("PathToSlug(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestPathToSlug_確定性(t *testing.T) {
	path := "/home/user/projects/acme/server"
	a := PathToSlug(path)
	b := PathToSlug(path)
	if a != b {
		t.Errorf("PathToSlug is not deterministic: %q != %q", a, b)
	}
}
