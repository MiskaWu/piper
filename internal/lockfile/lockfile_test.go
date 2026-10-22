package lockfile

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestAcquire_成功取得lock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "piper.lock")
	lf := New(path)

	err := lf.Acquire(8622)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	defer lf.Release()

	// lock 檔案應該存在
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("expected lock file to exist")
	}

	// 內容應包含 PID 和 port
	data, _ := os.ReadFile(path)
	content := string(data)
	if !strings.Contains(content, strconv.Itoa(os.Getpid())) {
		t.Errorf("expected PID in lock file, got %q", content)
	}
	if !strings.Contains(content, "8622") {
		t.Errorf("expected port in lock file, got %q", content)
	}
}

func TestAcquire_已鎖時回傳ErrLocked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "piper.lock")

	// 第一個實例取得 lock
	lf1 := New(path)
	if err := lf1.Acquire(8622); err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}
	defer lf1.Release()

	// 第二個實例應該被拒絕（同 process 但新 fd → flock 視為不同）
	// 注意：flock 在同一 process 開不同 fd 時，Linux 仍允許取得鎖。
	// 所以我們用子 process 測試真正的跨 process 排斥。
	cmd := exec.Command("flock", "--nonblock", "--exclusive", path, "-c", "echo locked")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected flock to fail, but got output: %s", output)
	}
}

func TestAcquire_子process鎖定後本process被拒絕(t *testing.T) {
	path := filepath.Join(t.TempDir(), "piper.lock")

	// 啟動子 process 持有 lock（sleep 10s），用 flock 命令
	holder := exec.Command("flock", "--exclusive", path, "-c", "sleep 10")
	if err := holder.Start(); err != nil {
		t.Fatalf("failed to start holder: %v", err)
	}
	defer holder.Process.Kill()

	// 等子 process 拿到 lock
	// 用 busy wait 嘗試取鎖，最多等 1 秒
	var acquireErr error
	for i := 0; i < 20; i++ {
		lf := New(path)
		acquireErr = lf.Acquire(8623)
		if acquireErr != nil {
			break
		}
		// 如果意外拿到了（子 process 還沒 ready），釋放再試
		lf.Release()
		cmd := exec.Command("sleep", "0.05")
		cmd.Run()
	}

	if acquireErr == nil {
		t.Fatal("expected acquire to fail, got nil")
	}

	var le *ErrLocked
	if !errors.As(acquireErr, &le) {
		t.Fatalf("expected ErrLocked, got %T: %v", acquireErr, acquireErr)
	}
}

func TestAcquire_前一個process死後可重新取得(t *testing.T) {
	path := filepath.Join(t.TempDir(), "piper.lock")

	// 啟動子 process 持有 lock，然後立刻結束
	cmd := exec.Command("flock", "--exclusive", path, "-c",
		fmt.Sprintf("echo '%d 8622' > %s", os.Getpid(), path))
	if err := cmd.Run(); err != nil {
		t.Fatalf("holder process failed: %v", err)
	}
	// 子 process 已結束 → flock 被 kernel 釋放

	// 現在應該能成功取得 lock
	lf := New(path)
	err := lf.Acquire(8623)
	if err != nil {
		t.Fatalf("expected to acquire lock after holder died, got %v", err)
	}
	defer lf.Release()
}

func TestRelease_關閉fd後其他process可取得(t *testing.T) {
	path := filepath.Join(t.TempDir(), "piper.lock")
	lf := New(path)

	if err := lf.Acquire(8622); err != nil {
		t.Fatal(err)
	}

	lf.Release()

	// 釋放後應該能重新取得
	lf2 := New(path)
	if err := lf2.Acquire(8623); err != nil {
		t.Fatalf("expected to acquire after release, got %v", err)
	}
	defer lf2.Release()
}

func TestRelease_重複呼叫不panic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "piper.lock")
	lf := New(path)

	if err := lf.Acquire(8622); err != nil {
		t.Fatal(err)
	}

	lf.Release()
	lf.Release() // should not panic
}

func TestRelease_未acquire時不panic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "piper.lock")
	lf := New(path)
	lf.Release() // should not panic
}

func TestAcquire_檔案不刪除可重用(t *testing.T) {
	path := filepath.Join(t.TempDir(), "piper.lock")

	// 第一次取得並釋放
	lf1 := New(path)
	if err := lf1.Acquire(8622); err != nil {
		t.Fatal(err)
	}
	lf1.Release()

	// 檔案應該還在
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("lock file should persist after release")
	}

	// 第二次重用同一個檔案
	lf2 := New(path)
	if err := lf2.Acquire(8623); err != nil {
		t.Fatalf("expected to reuse lock file, got %v", err)
	}
	defer lf2.Release()

	// 內容應該更新為新的 port
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "8623") {
		t.Errorf("expected updated port 8623, got %q", string(data))
	}
}
