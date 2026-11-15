package macro

import (
	"OpenLinkHub/src/rgb"
	"testing"
	"time"
)

func TestIndicatorRGBDefaultsToRed(t *testing.T) {
	got := IndicatorRGB(rgb.Color{})
	if got.Red != 255 || got.Green != 0 || got.Blue != 0 {
		t.Fatalf("black indicator colour should become red, got %+v", got)
	}

	custom := rgb.Color{Red: 0, Green: 120, Blue: 255}
	if got := IndicatorRGB(custom); got.Red != 0 || got.Green != 120 || got.Blue != 255 {
		t.Fatalf("custom colour was changed: %+v", got)
	}
}

func TestIndicatorFactorRanges(t *testing.T) {
	effects := []string{"pulse", "blink", "breathe", "double blink", "static", "unknown"}
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

func TestIndicatorColorScalesBase(t *testing.T) {
	base := rgb.Color{Red: 255, Green: 0, Blue: 0}
	got := IndicatorColor(base, "static", 0, 1)
	if got.Red != 255 || got.Hex != "#ff0000" {
		t.Fatalf("unexpected static colour: %+v", got)
	}
}
