package macro

import (
	"OpenLinkHub/src/rgb"
	"testing"
	"time"
)

func TestIndicatorRGBDefaults(t *testing.T) {
	red := IndicatorRGB(rgb.Color{})
	if red.Red != 255 || red.Green != 0 || red.Blue != 0 {
		t.Fatalf("black indicator colour should become red, got %+v", red)
	}

	amber := IndicatorRGB2(rgb.Color{})
	if amber.Red != 255 || amber.Green == 0 {
		t.Fatalf("black second colour should become amber, got %+v", amber)
	}

	custom := rgb.Color{Red: 0, Green: 120, Blue: 255}
	if got := IndicatorRGB(custom); got.Green != 120 || got.Blue != 255 {
		t.Fatalf("custom colour was changed: %+v", got)
	}
}

func TestIndicatorFactorRanges(t *testing.T) {
	effects := []string{"pulse", "blink", "breathe", "double blink", "progress", "static", "unknown"}
	for _, effect := range effects {
		min, max := 2.0, -1.0
		for i := 0; i < 400; i++ {
			elapsed := time.Duration(i) * 10 * time.Millisecond
			factor := IndicatorFactor(effect, elapsed, 1)
			if factor < 0 || factor > 1 {
				t.Fatalf("%s: factor %.3f out of range at %v", effect, factor, elapsed)
			}
			if factor < min {
				min = factor
			}
			if factor > max {
				max = factor
			}
		}
		if effect == "static" {
			if min != 1 || max != 1 {
				t.Fatalf("static should stay at 1, got %.2f..%.2f", min, max)
			}
			continue
		}
		if max-min < 0.5 {
			t.Fatalf("%s: effect is too subtle (%.2f..%.2f)", effect, min, max)
		}
	}
}

func TestIndicatorFactorSpeedFallback(t *testing.T) {
	if got := IndicatorFactor("pulse", time.Second, 0); got != IndicatorFactor("pulse", time.Second, 1) {
		t.Fatalf("speed 0 should behave like speed 1, got %v", got)
	}
}

func TestIndicatorFrameAlternate(t *testing.T) {
	base := rgb.Color{Red: 255}
	original := rgb.Color{Green: 255}
	first := IndicatorFrame(base, original, rgb.Color{}, "alternate", 0, 0, 1)
	if first.Green != 255 {
		t.Fatalf("alternate should start from the original colour, got %+v", first)
	}
	second := IndicatorFrame(base, original, rgb.Color{}, "alternate", time.Duration(indicatorCycleSeconds/2*float64(time.Second)), 0, 1)
	if second.Red != 255 {
		t.Fatalf("alternate should switch to the indicator colour, got %+v", second)
	}
}

func TestIndicatorFrameTwoColour(t *testing.T) {
	base := rgb.Color{Red: 255}
	second := rgb.Color{Blue: 255}
	frame := IndicatorFrame(base, rgb.Color{}, second, "two-colour", time.Duration(indicatorCycleSeconds/8*float64(time.Second)), 0, 1)
	if frame.Red == 0 || frame.Blue == 0 {
		t.Fatalf("two-colour should blend both colours, got %+v", frame)
	}
}

func TestIndicatorFrameProgress(t *testing.T) {
	base := rgb.Color{Red: 255}
	duration := 10 * time.Second
	start := IndicatorFrame(base, rgb.Color{}, rgb.Color{}, "progress", 0, duration, 1)
	end := IndicatorFrame(base, rgb.Color{}, rgb.Color{}, "progress", duration, duration, 1)
	if start.Red >= end.Red || end.Red < 250 {
		t.Fatalf("progress should ramp up: start %.0f end %.0f", start.Red, end.Red)
	}

	// Without a known duration progress falls back to pulse.
	fallback := IndicatorFrame(base, rgb.Color{}, rgb.Color{}, "progress", time.Second, 0, 1)
	pulse := IndicatorFrame(base, rgb.Color{}, rgb.Color{}, "pulse", time.Second, 0, 1)
	if fallback.Red != pulse.Red {
		t.Fatalf("progress without duration should match pulse: %.0f vs %.0f", fallback.Red, pulse.Red)
	}
}

func TestEstimateDuration(t *testing.T) {
	profile := &Macro{
		Repeat:      2,
		RepeatDelay: 500,
		Actions: map[int]Actions{
			0: {ActionType: 3},
			1: {ActionType: 5, ActionDelay: 1000},
		},
	}
	got := EstimateDuration(profile)
	want := 2500 * time.Millisecond
	if got != want {
		t.Fatalf("expected %v, got %v", want, got)
	}

	loop := &Macro{Repeat: -1, Actions: profile.Actions}
	if got := EstimateDuration(loop); got != 0 {
		t.Fatalf("loops have no progress duration, got %v", got)
	}
}
