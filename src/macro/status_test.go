package macro

import "testing"

func TestRunningRegistry(t *testing.T) {
	stop := make(chan struct{})
	key := RegisterRunning("serial-1", 7, "Test macro", 32, func() {
		close(stop)
	})

	running := GetRunning()
	if len(running) != 1 {
		t.Fatalf("expected 1 running macro, got %d", len(running))
	}
	if running[0].MacroId != 7 || running[0].MacroName != "Test macro" {
		t.Fatalf("unexpected running entry: %+v", running[0])
	}

	if n := StopRunning(99, ""); n != 0 {
		t.Fatalf("stopping an unknown macro returned %d", n)
	}
	if n := StopRunning(7, ""); n != 1 {
		t.Fatalf("expected one stop, got %d", n)
	}
	select {
	case <-stop:
	default:
		t.Fatal("stop func was not called")
	}
	if len(GetRunning()) != 0 {
		t.Fatal("running registry was not cleared")
	}
	UnregisterRunning(key)
}

func TestLoopStateStop(t *testing.T) {
	loops := NewLoopState()
	stop, running := loops.Toggle("button")
	if running {
		t.Fatal("new loop reported as already running")
	}
	if !loops.IsRunning("button") {
		t.Fatal("loop was not recorded")
	}
	loops.Stop("button")
	select {
	case <-stop:
	default:
		t.Fatal("loop stop channel was not closed")
	}
	if loops.IsRunning("button") {
		t.Fatal("loop is still recorded after stop")
	}
}
