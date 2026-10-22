package epiccontainer

import (
	"context"
	"errors"
	"io/fs"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRebuilder struct {
	mu            sync.Mutex
	callCount     int
	calledVersion string
	delay         time.Duration
	err           error
	started       chan struct{} // closed on first call
}

func (f *fakeRebuilder) Rebuild(_ fs.FS, version string) error {
	f.mu.Lock()
	f.callCount++
	f.calledVersion = version
	ch := f.started
	delay := f.delay
	err := f.err
	f.mu.Unlock()

	if ch != nil {
		select {
		case <-ch:
		default:
			close(ch)
		}
	}
	if delay > 0 {
		time.Sleep(delay)
	}
	return err
}

func (f *fakeRebuilder) calls() (int, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.callCount, f.calledVersion
}

type fakeProbe struct {
	values []string // first call returns values[0], next returns values[1], ...
	index  int
	mu     sync.Mutex
}

func (p *fakeProbe) Probe(_ context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.index >= len(p.values) {
		return "", nil
	}
	v := p.values[p.index]
	p.index++
	return v, nil
}

func newTestUpdater(rb ImageRebuilder, probe VersionProbe) *Updater {
	return &Updater{rebuild: rb, probe: probe}
}

func TestUpdater_Start_成功更新時更新狀態(t *testing.T) {
	fakeRB := &fakeRebuilder{}
	fakeP := &fakeProbe{values: []string{"2.1.100", "2.1.105"}}
	u := newTestUpdater(fakeRB.Rebuild, fakeP.Probe)

	err := u.Start(context.Background(), fstest.MapFS{}, "2.1.105")
	require.NoError(t, err)

	waitForDone(t, u)

	snap := u.Snapshot()
	assert.False(t, snap.InProgress)
	assert.Equal(t, "2.1.100", snap.PreviousVersion)
	assert.Equal(t, "2.1.105", snap.CurrentVersion)
	assert.Equal(t, "2.1.105", snap.TargetVersion)
	assert.Empty(t, snap.Error)

	count, version := fakeRB.calls()
	assert.Equal(t, 1, count)
	assert.Equal(t, "2.1.105", version)
}

func TestUpdater_Start_rebuild失敗時寫入Error(t *testing.T) {
	fakeRB := &fakeRebuilder{err: errors.New("docker build failed")}
	fakeP := &fakeProbe{values: []string{"2.1.100"}}
	u := newTestUpdater(fakeRB.Rebuild, fakeP.Probe)

	err := u.Start(context.Background(), fstest.MapFS{}, "2.1.105")
	require.NoError(t, err)

	waitForDone(t, u)

	snap := u.Snapshot()
	assert.False(t, snap.InProgress)
	assert.Contains(t, snap.Error, "docker build failed")
	assert.Equal(t, "2.1.100", snap.PreviousVersion)
	assert.Empty(t, snap.CurrentVersion, "current version should be empty when rebuild failed")
}

func TestUpdater_Start_重複呼叫時回ErrUpdateInProgress(t *testing.T) {
	started := make(chan struct{})
	fakeRB := &fakeRebuilder{delay: 200 * time.Millisecond, started: started}
	fakeP := &fakeProbe{values: []string{"2.1.100", "2.1.105"}}
	u := newTestUpdater(fakeRB.Rebuild, fakeP.Probe)

	err := u.Start(context.Background(), fstest.MapFS{}, "2.1.105")
	require.NoError(t, err)

	// wait for the first rebuild to actually start
	<-started

	err2 := u.Start(context.Background(), fstest.MapFS{}, "2.1.106")
	require.ErrorIs(t, err2, ErrUpdateInProgress)

	waitForDone(t, u)

	count, _ := fakeRB.calls()
	assert.Equal(t, 1, count, "second Start should not trigger rebuild")
}

func TestUpdater_InProgress_運行中時回true(t *testing.T) {
	started := make(chan struct{})
	fakeRB := &fakeRebuilder{delay: 100 * time.Millisecond, started: started}
	fakeP := &fakeProbe{values: []string{"2.1.100"}}
	u := newTestUpdater(fakeRB.Rebuild, fakeP.Probe)

	require.False(t, u.InProgress())

	err := u.Start(context.Background(), fstest.MapFS{}, "2.1.105")
	require.NoError(t, err)

	<-started
	assert.True(t, u.InProgress())

	waitForDone(t, u)
	assert.False(t, u.InProgress())
}

// waitForDone polls InProgress until false or the test times out.
func waitForDone(t *testing.T, u *Updater) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !u.InProgress() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("updater did not finish within deadline")
}
