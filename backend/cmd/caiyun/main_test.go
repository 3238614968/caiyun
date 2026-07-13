package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"caiyun/internal/version"
)

func TestVersionCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := realMain([]string{"version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("realMain(version) code = %d, stderr=%q", code, stderr.String())
	}
	var got version.Info
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode version output: %v", err)
	}
	if got.GoVersion == "" {
		t.Fatal("go_version is empty")
	}
}

func TestUnknownCommandReturnsUsageExitCode(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := realMain([]string{"unknown"}, &stdout, &stderr); code != 2 {
		t.Fatalf("realMain(unknown) code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "未知子命令") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestHelpCommandDoesNotRequireInfrastructure(t *testing.T) {
	var stdout bytes.Buffer
	if err := runCommand(context.Background(), []string{"help"}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("runCommand(help) error = %v", err)
	}
	if !strings.Contains(stdout.String(), "caiyun <command>") {
		t.Fatalf("help output = %q", stdout.String())
	}
}

type fakeSupervisedProcess struct {
	startErr error
	waitErr  error

	releaseOnce sync.Once
	release     chan struct{}
	stopped     chan struct{}
	waitDone    chan struct{}
}

func newFakeSupervisedProcess(startErr, waitErr error) *fakeSupervisedProcess {
	return &fakeSupervisedProcess{
		startErr: startErr,
		waitErr:  waitErr,
		release:  make(chan struct{}),
		stopped:  make(chan struct{}),
		waitDone: make(chan struct{}),
	}
}

func (p *fakeSupervisedProcess) Start() error { return p.startErr }

func (p *fakeSupervisedProcess) Wait() error {
	<-p.release
	close(p.waitDone)
	return p.waitErr
}

func (p *fakeSupervisedProcess) Signal(os.Signal) error {
	p.stop()
	return nil
}

func (p *fakeSupervisedProcess) Kill() error {
	p.stop()
	return nil
}

func (p *fakeSupervisedProcess) stop() {
	p.releaseOnce.Do(func() {
		close(p.stopped)
		close(p.release)
	})
}

func TestSuperviseRolesReapsFirstChildWhenSecondStartFails(t *testing.T) {
	api := newFakeSupervisedProcess(nil, errors.New("api terminated"))
	worker := newFakeSupervisedProcess(errors.New("worker start failed"), nil)

	err := superviseRoles(context.Background(), []string{"api", "worker"}, func(role string) supervisedProcess {
		if role == "api" {
			return api
		}
		return worker
	}, time.Second)
	if err == nil || !strings.Contains(err.Error(), "启动 worker 子进程失败") {
		t.Fatalf("superviseRoles error = %v", err)
	}

	select {
	case <-api.stopped:
	default:
		t.Fatal("already-started API child was not stopped")
	}
	select {
	case <-api.waitDone:
	default:
		t.Fatal("already-started API child was not reaped with Wait")
	}
}

func TestSuperviseRolesCancellationTreatsChildTerminationAsNormal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	api := newFakeSupervisedProcess(nil, errors.New("signal: interrupt"))
	worker := newFakeSupervisedProcess(nil, errors.New("signal: interrupt"))
	allStarted := make(chan struct{})
	started := 0

	result := make(chan error, 1)
	go func() {
		result <- superviseRoles(ctx, []string{"api", "worker"}, func(role string) supervisedProcess {
			started++
			if started == 2 {
				close(allStarted)
			}
			if role == "api" {
				return api
			}
			return worker
		}, time.Second)
	}()

	select {
	case <-allStarted:
	case <-time.After(time.Second):
		t.Fatal("children did not start")
	}
	cancel()

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("normal cancellation returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("supervisor did not finish after cancellation")
	}

	for name, process := range map[string]*fakeSupervisedProcess{"api": api, "worker": worker} {
		select {
		case <-process.waitDone:
		default:
			t.Fatalf("%s child was not reaped", name)
		}
	}
}
