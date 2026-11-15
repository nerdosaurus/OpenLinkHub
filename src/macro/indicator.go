package macro

// Package: macro
// License: GPL-3.0 or later

import (
	"OpenLinkHub/src/rgb"
	"math"
	"strings"
	"time"
)

// indicatorCycleSeconds is the length of one full indicator cycle at speed 1.
const indicatorCycleSeconds = 1.6

// IndicatorRGB normalises an indicator colour. An unset (black) colour becomes
// red, because a black status pulse is indistinguishable from the LEDs being
// off and is almost never what someone meant to choose.
func IndicatorRGB(base rgb.Color) rgb.Color {
	if base.Red <= 0 && base.Green <= 0 && base.Blue <= 0 {
		return rgb.Color{Red: 255, Brightness: 1, Hex: "#ff0000"}
	}
	return base
}

// IndicatorFactor returns the brightness multiplier (0..1) for an indicator
// effect at elapsed time. Effects:
//
//	pulse        sharp attack/decay, mostly bright with a short dark gap
//	breathe      gentle sine, never fully dark
//	blink        hard on/off
//	double blink two quick flashes, then a pause (heartbeat)
//	static       constant
func IndicatorFactor(effect string, elapsed time.Duration, speed float64) float64 {
	if speed <= 0 {
		speed = 1
	}
	phase := (elapsed.Seconds() / indicatorCycleSeconds) * speed
	switch strings.ToLower(strings.TrimSpace(effect)) {
	case "static":
		return 1
	case "blink":
		if math.Mod(phase, 1) < 0.5 {
			return 1
		}
		return 0
	case "double blink", "double", "heartbeat":
		switch p := math.Mod(phase, 1); {
		case p < 0.10:
			return 1
		case p < 0.18:
			return 0
		case p < 0.28:
			return 1
		default:
			return 0
		}
	case "breathe":
		return 0.35 + 0.65*(0.5+0.5*math.Sin(phase*2*math.Pi))
	default: // pulse
		s := 0.5 + 0.5*math.Sin(phase*2*math.Pi)
		return 0.02 + 0.98*math.Pow(s, 2.4)
	}
}

// IndicatorColor returns base scaled by the indicator effect.
func IndicatorColor(base rgb.Color, effect string, elapsed time.Duration, speed float64) rgb.Color {
	factor := IndicatorFactor(effect, elapsed, speed)
	return rgb.Color{
		Red:        base.Red * factor,
		Green:      base.Green * factor,
		Blue:       base.Blue * factor,
		Brightness: 1,
		Hex:        fmtHex(base, factor),
	}
}

func fmtHex(base rgb.Color, factor float64) string {
	const digits = "0123456789abcdef"
	components := [3]int{
		int(base.Red * factor),
		int(base.Green * factor),
		int(base.Blue * factor),
	}
	out := make([]byte, 0, 7)
	out = append(out, '#')
	for _, value := range components {
		if value < 0 {
			value = 0
		}
		if value > 255 {
			value = 255
		}
		out = append(out, digits[value>>4], digits[value&0x0f])
	}
	return string(out)
}
