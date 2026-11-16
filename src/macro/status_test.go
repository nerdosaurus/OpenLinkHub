package macro

import "testing"

func TestRunningRegistry(t *testing.T) {
	stop := make(chan struct{})
	key := RegisterRunning(Trigger{Serial: "serial-1", Key: 32, Row: 1}, &Macro{
		Id: 7, Name: "Test macro", Repeat: -1,
	}, func() {
		close(stop)
	})

	running := GetRunning()
	if len(running) != 1 {
		t.Fatalf("expected 1 running macro, got %d", len(running))
	}
	if running[0].MacroId != 7 || running[0].MacroName != "Test macro" ||
		running[0].TriggerKey != 32 || running[0].TriggerRow != 1 ||
		running[0].Repeat != -1 {
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
