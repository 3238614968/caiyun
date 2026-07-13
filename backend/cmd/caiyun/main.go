package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	apiapp "caiyun/internal/app/api"
	migratorapp "caiyun/internal/app/migrator"
	reencryptapp "caiyun/internal/app/reencrypt"
	workerapp "caiyun/internal/app/worker"
	"caiyun/internal/version"
)

const allShutdownTimeout = 30 * time.Second

type usageError struct{ message string }

func (e *usageError) Error() string { return e.message }

func main() {
	os.Exit(realMain(os.Args[1:], os.Stdout, os.Stderr))
}

func realMain(args []string, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := runCommand(ctx, args, stdout, stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintf(stderr, "caiyun: %v\n", err)
		var usageErr *usageError
		if errors.As(err, &usageErr) {
			printUsage(stderr)
			return 2
		}
		return 1
	}
	return 0
}

func runCommand(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return &usageError{message: "缺少子命令"}
	}
	command := strings.ToLower(strings.TrimSpace(args[0]))
	subArgs := args[1:]
	switch command {
	case "api":
		return apiapp.Run(ctx, subArgs)
	case "worker":
		return workerapp.Run(ctx, subArgs)
	case "migrate", "migrator":
		return migratorapp.Run(ctx, subArgs)
	case "reencrypt":
		return reencryptapp.Run(ctx, subArgs)
	case "all":
		if len(subArgs) != 0 {
			return &usageError{message: "all 不支持位置参数；请通过环境变量分别配置 API 与 Worker"}
		}
		return runAll(ctx, stdout, stderr)
	case "version":
		if len(subArgs) != 0 {
			return &usageError{message: "version 不支持参数"}
		}
		return json.NewEncoder(stdout).Encode(version.Get())
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	default:
		return &usageError{message: fmt.Sprintf("未知子命令 %q", args[0])}
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `用法: caiyun <command> [options]

命令:
  api         启动 HTTP API
  worker      启动后台任务 Worker
  migrate     执行数据库迁移和结构校验
  reencrypt   扫描或轮换字段加密版本
  all         由当前二进制监督 api 与 worker 两个子进程
  version     输出构建版本信息
  help        显示帮助

生产环境建议使用同一个 caiyun 制品分别运行 "api" 和 "worker"，并在发布前单独运行 "migrate"。`)
}

type childResult struct {
	name string
	err  error
}

type supervisedProcess interface {
	Start() error
	Wait() error
	Signal(os.Signal) error
	Kill() error
}

type execProcess struct{ cmd *exec.Cmd }

func (p *execProcess) Start() error                  { return p.cmd.Start() }
func (p *execProcess) Wait() error                   { return p.cmd.Wait() }
func (p *execProcess) Signal(signal os.Signal) error { return p.cmd.Process.Signal(signal) }
func (p *execProcess) Kill() error                   { return p.cmd.Process.Kill() }

type processFactory func(role string) supervisedProcess

// runAll is a convenience supervisor for single-host installations. It starts
// isolated API and worker child processes from the exact same executable, so a
// fault or process-global setting in one role cannot corrupt the other role.
func runAll(ctx context.Context, stdout, stderr io.Writer) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("定位当前二进制失败: %w", err)
	}

	return superviseRoles(ctx, []string{"api", "worker"}, func(role string) supervisedProcess {
		cmd := exec.Command(executable, role)
		cmd.Env = os.Environ()
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		cmd.Stdin = os.Stdin
		return &execProcess{cmd: cmd}
	}, allShutdownTimeout)
}

// superviseRoles contains the process lifecycle policy used by the all command.
// Keeping process creation injectable makes partial-start and signal races
// deterministic in unit tests without launching real services.
func superviseRoles(ctx context.Context, roles []string, factory processFactory, shutdownTimeout time.Duration) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if factory == nil {
		return errors.New("子进程工厂不能为空")
	}

	commands := make([]supervisedProcess, 0, len(roles))
	results := make(chan childResult, len(roles))
	for _, role := range roles {
		child := factory(role)
		if child == nil {
			shutdownChildren(commands, results, len(commands), shutdownTimeout)
			return fmt.Errorf("创建 %s 子进程失败: 工厂返回 nil", role)
		}
		if err := child.Start(); err != nil {
			// A partially started all-mode must stop and reap every preceding
			// child before returning, otherwise a failed worker start can leave
			// an orphan API process serving stale code.
			shutdownChildren(commands, results, len(commands), shutdownTimeout)
			return fmt.Errorf("启动 %s 子进程失败: %w", role, err)
		}
		commands = append(commands, child)
		go func(name string, process supervisedProcess) {
			results <- childResult{name: name, err: process.Wait()}
		}(role, child)
	}

	select {
	case first := <-results:
		shutdownChildren(commands, results, len(commands)-1, shutdownTimeout)
		// Signal delivery and child exit can win the select in either order.
		// Once the parent context is cancelled, termination errors are part of
		// normal graceful shutdown and must not produce exit code 1.
		if ctx.Err() != nil {
			return nil
		}
		if first.err == nil {
			return fmt.Errorf("%s 子进程意外退出", first.name)
		}
		return fmt.Errorf("%s 子进程退出: %w", first.name, first.err)
	case <-ctx.Done():
		shutdownChildren(commands, results, len(commands), shutdownTimeout)
		return nil
	}
}

func stopChildren(commands []supervisedProcess) {
	for _, process := range commands {
		if process == nil {
			continue
		}
		if runtime.GOOS == "windows" {
			_ = process.Kill()
			continue
		}
		_ = process.Signal(os.Interrupt)
	}
}

func killChildren(commands []supervisedProcess) {
	for _, process := range commands {
		if process != nil {
			_ = process.Kill()
		}
	}
}

func shutdownChildren(commands []supervisedProcess, results <-chan childResult, count int, timeout time.Duration) {
	if count <= 0 {
		return
	}
	stopChildren(commands)
	if timeout <= 0 {
		timeout = allShutdownTimeout
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	remaining := count
	for remaining > 0 {
		select {
		case <-results:
			remaining--
		case <-timer.C:
			killChildren(commands)
			// Kill is only the escalation mechanism; Wait must still complete to
			// release the OS process handle and reap every child.
			for remaining > 0 {
				<-results
				remaining--
			}
		}
	}
}
