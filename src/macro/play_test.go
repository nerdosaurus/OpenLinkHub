package macro

import (
	"sync/atomic"
	"testing"
	"time"
)

type fakeHooks struct {
	starts   atomic.Int32
	stops    atomic.Int32
	restarts atomic.Int32
}

func (f *fakeHooks) TrackKey(int, uint16, uint8) {}
func (f *fakeHooks) ReleaseKeys()                {}
func (f *fakeHooks) NewStopRepeat() <-chan struct{} {
	return make(chan struct{})
}
func (f *fakeHooks) StartMacroIndicator(Trigger, *Indicator) {
	f.starts.Add(1)
}
func (f *fakeHooks) StopMacroIndicator() {
	f.stops.Add(1)
}
func (f *fakeHooks) RestartMacroIndicatorCycle() {
	f.restarts.Add(1)
}

func TestPlayLoopRestartsIndicatorEachPass(t *testing.T) {
	hooks := &fakeHooks{}
	profile := &Macro{
		Id:          99,
		Name:        "loop test",
		Repeat:      -1,
		RepeatDelay: 25,
		Actions:     map[int]Actions{},
		Indicator:   &Indicator{Enabled: true, Effect: "progress"},
	}
	loops := NewLoopState()
	Play(hooks, profile, "loop-key", loops, Trigger{Serial: "serial", Key: 1})

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if hooks.restarts.Load() >= 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	running := GetRunning()
	if len(running) != 1 {
		t.Fatalf("expected one running macro, got %d", len(running))
	}
	if running[0].Cycle < 2 {
		t.Fatalf("expected at least two loop passes, got %d", running[0].Cycle)
	}
	if hooks.starts.Load() != 1 {
		t.Fatalf("indicator should start once, got %d", hooks.starts.Load())
	}
	if hooks.restarts.Load() < 2 {
		t.Fatalf("indicator should restart each pass, got %d", hooks.restarts.Load())
	}
	if n := StopRunning(profile.Id, ""); n != 1 {
		t.Fatalf("expected one stop, got %d", n)
	}
	time.Sleep(30 * time.Millisecond)
	if hooks.stops.Load() != 1 {
		t.Fatalf("indicator should stop once, got %d", hooks.stops.Load())
	}
}
