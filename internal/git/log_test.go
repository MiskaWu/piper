package git

import (
	"errors"
	"testing"
)

func TestLog(t *testing.T) {
	tests := []struct {
		name     string
		output   []byte
		err      error
		wantLen  int
		wantErr  bool
		wantHash string // first entry hash, if any
		wantSubj string // first entry subject, if any
	}{
		{
			name:     "正常解析多行 git log oneline 輸出",
			output:   []byte("a1b2c3d fix: 修正登入問題\ne4f5g6h feat: 新增搜尋功能\n"),
			wantLen:  2,
			wantHash: "a1b2c3d",
			wantSubj: "fix: 修正登入問題",
		},
		{
			name:    "空輸出時回傳空 slice",
			output:  []byte(""),
			wantLen: 0,
		},
		{
			name:    "非 git repo 時回傳空 slice 無 error",
			output:  []byte("fatal: not a git repository (or any of the parent directories): .git"),
			err:     errors.New("exit status 128"),
			wantLen: 0,
		},
		{
			name:    "git 指令錯誤時回傳 error",
			output:  []byte("error: something went wrong"),
			err:     errors.New("exit status 1"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			cmd := &mockCommander{output: tt.output, err: tt.err}
			checker := NewChecker(cmd)
			opts := LogOptions{
				Dir:   "/some/project",
				Since: "2026-03-24",
				Until: "2026-03-25",
			}

			// Act
			got, err := checker.Log(opts)

			// Assert
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tt.wantLen {
				t.Fatalf("expected %d entries, got %d", tt.wantLen, len(got))
			}
			if tt.wantHash != "" && got[0].Hash != tt.wantHash {
				t.Errorf("expected hash %q, got %q", tt.wantHash, got[0].Hash)
			}
			if tt.wantSubj != "" && got[0].Subject != tt.wantSubj {
				t.Errorf("expected subject %q, got %q", tt.wantSubj, got[0].Subject)
			}
		})
	}
}
