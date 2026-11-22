//go:build linux

package inputmanager

// Root-only timing harness for the virtual keyboard. It measures the real
// evdev intervals produced by InputControlKeyboardTextWithDelay.
//
// On the desktop, from the repository root:
//
//	go test -c ./src/inputmanager -o /tmp/inputmanager-timing.test
//	sudo env OPENLINKHUB_UINPUT_TEST=1 /tmp/inputmanager-timing.test \
//	    -test.run TestUinputTypingTiming -test.v
//
// It creates a virtual keyboard and types "aaaaaaaaaa" *only* after grabbing
// the new event device with EVIOCGRAB, so the compositor does not receive the
// keys. It skips when uinput is unavailable or the grab fails.

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"OpenLinkHub/src/config"
	"OpenLinkHub/src/logger"
	"golang.org/x/sys/unix"
)

func TestMain(m *testing.M) {
	if os.Geteuid() == 0 && os.Getenv("OPENLINKHUB_UINPUT_TEST") == "1" {
		dir, err := os.MkdirTemp("", "olh-uinput-test-")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		_ = os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0o600)
		_ = os.Chdir(dir)
		config.Init()
		logger.Init()
		code := m.Run()
		_ = os.RemoveAll(dir)
		os.Exit(code)
	}
	os.Exit(m.Run())
}

// EVIOCGRAB is _IOW('E', 0x90, int); x/sys/unix does not expose it on every
// version. Grabbing the event device keeps the injected keys out of the
// compositor.
const eviocGrab = 0x40044590

// snapshotKeyboardEvents returns the event devices that already exist. The
// daemon may be running and own a virtual keyboard with the same name, so the
// test must measure the device it just created, not the daemon's.
func snapshotKeyboardEvents() map[string]bool {
	existing := map[string]bool{}
	names, _ := filepath.Glob("/sys/class/input/event*/device/name")
	for _, namePath := range names {
		raw, err := os.ReadFile(namePath)
		if err == nil && strings.TrimSpace(string(raw)) == "OpenLinkHub Virtual Keyboard" {
			existing[filepath.Dir(filepath.Dir(namePath))] = true
		}
	}
	return existing
}

func findVirtualKeyboardNode(t *testing.T, before map[string]bool) string {
	t.Helper()
	names, _ := filepath.Glob("/sys/class/input/event*/device/name")
	for _, namePath := range names {
		raw, err := os.ReadFile(namePath)
		if err != nil || strings.TrimSpace(string(raw)) != "OpenLinkHub Virtual Keyboard" {
			continue
		}
		eventDir := filepath.Dir(filepath.Dir(namePath))
		if before[eventDir] {
			continue
		}
		devRaw, err := os.ReadFile(filepath.Join(eventDir, "dev"))
		if err != nil {
			t.Fatal(err)
		}
		var major, minor uint32
		if _, err := fmt.Sscanf(strings.TrimSpace(string(devRaw)), "%d:%d", &major, &minor); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll("/dev/input", 0o755); err != nil {
			t.Fatal(err)
		}
		index := strings.TrimPrefix(filepath.Base(eventDir), "event")
		node := "/dev/input/event" + index
		if _, err := os.Stat(node); err != nil {
			if err := syscall.Mknod(node, syscall.S_IFCHR|0o660, int(unix.Mkdev(major, minor))); err != nil {
				t.Fatalf("mknod %s: %v", node, err)
			}
			_ = os.Chmod(node, 0o660)
		}
		return node
	}
	return ""
}

type pressReader struct {
	presses <-chan time.Time
	stop    func()
}

func openPressReader(t *testing.T, node string) pressReader {
	t.Helper()
	fd, err := unix.Open(node, unix.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		t.Fatalf("open %s: %v", node, err)
	}
	// Keep the injected keys away from the compositor. If this fails we must
	// not type at all.
	if err := unix.IoctlSetInt(fd, eviocGrab, 1); err != nil {
		_ = unix.Close(fd)
		t.Skipf("cannot grab %s (refusing to inject keys into the session): %v", node, err)
	}

	presses := make(chan time.Time, 64)
	stopCh := make(chan struct{})
	done := make(chan struct{})
	timevalSize := int(unsafe.Sizeof(unix.Timeval{}))
	eventSize := timevalSize + 8
	go func() {
		defer close(done)
		buf := make([]byte, eventSize*64)
		for {
			select {
			case <-stopCh:
				return
			default:
			}
			n, err := unix.Read(fd, buf)
			if err == unix.EAGAIN {
				time.Sleep(time.Millisecond)
				continue
			}
			if err != nil {
				return
			}
			for off := 0; off+eventSize <= n; off += eventSize {
				event := buf[off : off+eventSize]
				typ := binary.LittleEndian.Uint16(event[timevalSize : timevalSize+2])
				val := int32(binary.LittleEndian.Uint32(event[timevalSize+4 : timevalSize+8]))
				if typ == 1 && val == 1 { // EV_KEY press
					select {
					case presses <- time.Now():
					default:
					}
				}
			}
		}
	}()
	return pressReader{
		presses: presses,
		stop: func() {
			close(stopCh)
			_ = unix.IoctlSetInt(fd, eviocGrab, 0)
			_ = unix.Close(fd)
			<-done
			close(presses)
		},
	}
}

func collectPresses(presses <-chan time.Time, want int) []time.Time {
	out := make([]time.Time, 0, want)
	timeout := time.After(5 * time.Second)
	for len(out) < want {
		select {
		case ts, ok := <-presses:
			if !ok {
				return out
			}
			out = append(out, ts)
		case <-timeout:
			return out
		}
	}
	return out
}

type intervals struct {
	min, mean, max time.Duration
	count          int
}

func runTyping(t *testing.T, delay, variance, chars int) intervals {
	t.Helper()
	before := snapshotKeyboardEvents()
	if err := createVirtualKeyboard(vendorId, productId); err != nil {
		t.Fatalf("create virtual keyboard: %v", err)
	}
	defer destroyVirtualKeyboard()

	var node string
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		node = findVirtualKeyboardNode(t, before)
		if node != "" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if node == "" {
		t.Fatal("virtual keyboard event node not found")
	}

	reader := openPressReader(t, node)
	defer reader.stop()
	time.Sleep(100 * time.Millisecond)

	InputControlKeyboardTextWithDelay(strings.Repeat("a", chars), delay, variance)
	got := collectPresses(reader.presses, chars)
	if len(got) < 3 {
		t.Fatalf("expected at least 3 key presses, got %d", len(got))
	}

	var min, max, sum time.Duration
	for i := 1; i < len(got); i++ {
		d := got[i].Sub(got[i-1])
		if min == 0 || d < min {
			min = d
		}
		if d > max {
			max = d
		}
		sum += d
	}
	n := len(got) - 1
	return intervals{min: min, mean: sum / time.Duration(n), max: max, count: len(got)}
}

func TestUinputTypingTiming(t *testing.T) {
	if os.Getenv("OPENLINKHUB_UINPUT_TEST") != "1" {
		t.Skip("set OPENLINKHUB_UINPUT_TEST=1 and run as root to inject keys")
	}
	if os.Geteuid() != 0 {
		t.Skip("uinput timing test needs root")
	}
	if _, err := os.Stat("/dev/uinput"); err != nil {
		t.Skipf("/dev/uinput is not available: %v", err)
	}

	base := runTyping(t, 50, 0, 10)
	t.Logf("delay=50 variance=0 presses=%d intervals min=%v mean=%v max=%v",
		base.count, base.min, base.mean, base.max)
	if base.mean < 45*time.Millisecond || base.mean > 140*time.Millisecond {
		t.Fatalf("expected ~70 ms between presses (20 ms hold + 50 ms delay), got %v", base.mean)
	}

	jitter := runTyping(t, 50, 100, 12)
	t.Logf("delay=50 variance=100 presses=%d intervals min=%v mean=%v max=%v",
		jitter.count, jitter.min, jitter.mean, jitter.max)
	if jitter.mean <= base.mean+10*time.Millisecond {
		t.Fatalf("jitter did not raise the mean interval: base=%v jitter=%v", base.mean, jitter.mean)
	}
	if jitter.max <= jitter.min+20*time.Millisecond {
		t.Fatalf("jitter produced no visible spread: min=%v max=%v", jitter.min, jitter.max)
	}
}
