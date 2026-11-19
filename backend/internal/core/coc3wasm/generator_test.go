package coc3wasm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type sequenceReader struct {
	next  byte
	count int
}

func (r *sequenceReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = r.next
		r.next++
	}
	r.count += len(p)
	return len(p), nil
}

func wheelFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "tasks", "assets", "coc3", "wheel_bg.wasm"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestOfficialWheelMatchesJavaScriptReference(t *testing.T) {
	wasm := wheelFixture(t)
	data, err := os.ReadFile(filepath.Join("testdata", "reference.json"))
	if err != nil {
		t.Fatal(err)
	}
	var reference struct {
		WasmSHA256 string `json:"wasmSHA256"`
		Vectors    []struct {
			Clock       int64     `json:"clock"`
			Seed        byte      `json:"seed"`
			RandomBytes int       `json:"randomBytes"`
			Pair        [2]string `json:"pair"`
		} `json:"vectors"`
	}
	if err := json.Unmarshal(data, &reference); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(wasm)) != reference.WasmSHA256 {
		t.Fatal("regenerate the reference for the updated official module")
	}
	if len(reference.Vectors) < 2 {
		t.Fatal("reference vectors are missing")
	}
	t.Setenv("COC3_HOST_ENV_SENTINEL", "must-not-be-included-in-the-token")
	for _, vector := range reference.Vectors {
		t.Run(fmt.Sprint(vector.Seed), func(t *testing.T) {
			entropy := &sequenceReader{next: vector.Seed}
			value, err := generate(context.Background(), wasm, environment{now: func() time.Time { return time.UnixMilli(vector.Clock) }, random: entropy})
			if err != nil {
				t.Fatal(err)
			}
			var pair [2]string
			if err := json.Unmarshal([]byte(value), &pair); err != nil {
				t.Fatal(err)
			}
			if pair != vector.Pair || entropy.count != vector.RandomBytes {
				t.Fatalf("Go/JavaScript mismatch: aurora=%v content=%v entropy=%d/%d", pair[0] == vector.Pair[0], pair[1] == vector.Pair[1], entropy.count, vector.RandomBytes)
			}
		})
	}
}

func TestGeneratorWorksWithoutExecutableSearchPath(t *testing.T) {
	wasm := wheelFixture(t)
	t.Setenv("PATH", t.TempDir())
	value, err := Generate(context.Background(), wasm)
	if err != nil {
		t.Fatal(err)
	}
	var pair []string
	if err := json.Unmarshal([]byte(value), &pair); err != nil || len(pair) != 2 {
		t.Fatal("invalid pair")
	}
	for _, encoded := range pair {
		if decoded, err := base64.StdEncoding.Strict().DecodeString(encoded); err != nil || len(decoded) == 0 {
			t.Fatal("invalid base64 token")
		}
	}
}

func TestGeneratorInstancesAreIsolated(t *testing.T) {
	wasm := wheelFixture(t)
	const workers = 8
	var wg sync.WaitGroup
	values := make(chan string, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, err := Generate(context.Background(), wasm)
			if err != nil {
				t.Error(err)
				return
			}
			values <- value
		}()
	}
	wg.Wait()
	close(values)
	seen := map[string]bool{}
	for value := range values {
		if seen[value] {
			t.Error("independent instances reused a token")
		}
		seen[value] = true
	}
	if len(seen) != workers {
		t.Fatalf("got %d tokens", len(seen))
	}
}

func TestGeneratorRejectsMalformedModuleAndCanceledContext(t *testing.T) {
	if _, err := Generate(context.Background(), []byte("not wasm")); err == nil {
		t.Fatal("invalid module accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Generate(ctx, wheelFixture(t)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

type failedEntropy struct{}

var errEntropy = errors.New("entropy unavailable")

func (failedEntropy) Read([]byte) (int, error) { return 0, errEntropy }

func TestGeneratorPropagatesEntropyFailureAndRejectsUnknownHostCalls(t *testing.T) {
	wasm := wheelFixture(t)
	if _, err := generate(context.Background(), wasm, environment{now: time.Now, random: failedEntropy{}}); !errors.Is(err, errEntropy) {
		t.Fatalf("entropy error lost: %v", err)
	}
	// Simulate an upstream ABI change in memory, leaving the vendor asset intact.
	unknown := bytes.Replace(wasm, []byte("__wbg_now_16f0c993d5dd6c27"), []byte("__wbg_net_16f0c993d5dd6c27"), 1)
	if _, err := Generate(context.Background(), unknown); err == nil {
		t.Fatal("unsupported host call was silently accepted")
	}
}

func TestGeneratorInterruptsGuestLoopAndLimitsMemory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := Generate(ctx, loopingWheelModule()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("guest did not respect deadline: %v", err)
	}
	// A minimal module declaring 257 pages exceeds the configured 256-page limit.
	tooMuchMemory := []byte{0, 97, 115, 109, 1, 0, 0, 0, 5, 4, 1, 0, 0x81, 2}
	if _, err := Generate(context.Background(), tooMuchMemory); err == nil {
		t.Fatal("oversized guest memory accepted")
	}
}

// Equivalent WAT: a wheel with the required signatures and memory, whose
// __wbindgen_start body is (loop (br 0)). No imported host function can hang.
func loopingWheelModule() []byte {
	module := []byte{0, 97, 115, 109, 1, 0, 0, 0}
	section := func(id byte, data []byte) {
		module = append(module, id, byte(len(data)))
		module = append(module, data...)
	}
	section(1, []byte{5, 0x60, 0, 0, 0x60, 0, 1, 0x7f, 0x60, 2, 0x7f, 0x7f, 1, 0x7f, 0x60, 1, 0x7f, 1, 0x7f, 0x60, 4, 0x7f, 0x7f, 0x7f, 0x7f, 0})
	section(3, []byte{5, 0, 1, 2, 3, 4})
	section(5, []byte{1, 0, 1})
	exports := []byte{6, 6, 'm', 'e', 'm', 'o', 'r', 'y', 2, 0}
	for index, name := range []string{"__wbindgen_start", "wheel_create_wheel", "__wbindgen_export", "__wbindgen_add_to_stack_pointer", "wheel_roto_simple"} {
		exports = append(exports, byte(len(name)))
		exports = append(exports, name...)
		exports = append(exports, 0, byte(index))
	}
	// Export payload length can exceed 127, hence its unsigned LEB128 encoding.
	module = append(module, 7, byte(len(exports))|0x80, byte(len(exports)>>7))
	module = append(module, exports...)
	section(10, []byte{5, 7, 0, 3, 0x40, 0x0c, 0, 0x0b, 0x0b, 4, 0, 0x41, 0, 0x0b, 4, 0, 0x41, 0, 0x0b, 4, 0, 0x41, 0, 0x0b, 2, 0, 0x0b})
	return module
}
