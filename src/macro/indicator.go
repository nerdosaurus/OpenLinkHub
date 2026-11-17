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

// indicatorRed is the fallback colour for an unset (black) indicator.
var indicatorRed = rgb.Color{Red: 255, Brightness: 1, Hex: "#ff0000"}

// indicatorAmber is the fallback second colour for a two-colour indicator.
var indicatorAmber = rgb.Color{Red: 255, Green: 176, Hex: "#ffb000", Brightness: 1}

func isBlack(color rgb.Color) bool {
	return color.Red <= 0 && color.Green <= 0 && color.Blue <= 0
}

// IndicatorRGB normalises an indicator colour. An unset (black) colour becomes
// red, because a black status pulse is indistinguishable from the LEDs being
// off and is almost never what someone meant to choose.
func IndicatorRGB(base rgb.Color) rgb.Color {
	if isBlack(base) {
		return indicatorRed
	}
	return base
}

// IndicatorRGB2 normalises the secondary colour used by two-colour effects.
func IndicatorRGB2(base rgb.Color) rgb.Color {
	if isBlack(base) {
		return indicatorAmber
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
//	progress     ramp from dim to full over duration (falls back to pulse)
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
	case "progress":
		return 0.02 + 0.98*math.Pow(0.5+0.5*math.Sin(phase*2*math.Pi), 2.4)
	default: // pulse
		s := 0.5 + 0.5*math.Sin(phase*2*math.Pi)
		return 0.02 + 0.98*math.Pow(s, 2.4)
	}
}

// IndicatorCycle reports how far through the effect cycle we are (0..1),
// used by effects that alternate rather than dim.
func IndicatorCycle(elapsed time.Duration, speed float64) float64 {
	if speed <= 0 {
		speed = 1
	}
	return math.Mod(elapsed.Seconds()/indicatorCycleSeconds*speed, 1)
}

// IndicatorFrame returns the colour to display for an effect.
//
// base is the indicator colour, original the colour the zone/key had before the
// indicator started (used by "alternate"), secondary the second colour (used by
// "two-colour"), duration the estimated one-shot length (used by "progress";
// zero means unknown, so progress behaves like pulse).
func IndicatorFrame(
	base rgb.Color,
	original rgb.Color,
	secondary rgb.Color,
	effect string,
	elapsed time.Duration,
	duration time.Duration,
	loop bool,
	speed float64,
) rgb.Color {
	if speed <= 0 {
		speed = 1
	}
	switch strings.ToLower(strings.TrimSpace(effect)) {
	case "alternate":
		if IndicatorCycle(elapsed, speed) < 0.5 {
			return scaled(original, 1)
		}
		return scaled(base, 1)
	case "two-colour", "two color", "blend":
		blend := 0.5 + 0.5*math.Sin(IndicatorCycle(elapsed, speed)*2*math.Pi)
		mixed := rgb.Color{
			Red:        base.Red + (secondary.Red-base.Red)*blend,
			Green:      base.Green + (secondary.Green-base.Green)*blend,
			Blue:       base.Blue + (secondary.Blue-base.Blue)*blend,
			Brightness: 1,
		}
		mixed.Hex = fmtHex(mixed, 1)
		return mixed
	case "progress":
		if duration > 0 {
			progress := elapsed.Seconds() / duration.Seconds()
			if loop {
				progress = math.Mod(progress, 1)
			}
			progress = math.Min(1, math.Max(0, progress))
			// Perceptual curve: a linear ramp on a red LED looks saturated well
			// before the end, which reads as "stuck at 100%".
			factor := 0.03 + 0.97*math.Pow(progress, 2.4)
			return scaled(base, factor)
		}
		fallthrough
	default:
		return scaled(base, IndicatorFactor(effect, elapsed, speed))
	}
}

// EstimateDuration approximates a macro's play time for the progress effect.
// For a loop it returns one cycle (one pass plus the repeat delay); for a
// one-shot it returns the whole run. Tap/repeat timings are rough and held
// actions are ignored (their length is unknown).
func EstimateDuration(profile *Macro) time.Duration {
	if profile == nil {
		return 0
	}
	repeats := profile.Repeat
	if repeats <= 0 {
		repeats = 1
	}
	var per time.Duration
	for i := 0; i < len(profile.Actions); i++ {
		action, ok := profile.Actions[i]
		if !ok {
			continue
		}
		switch action.ActionType {
		case 1, 3, 9, 20: // keyboard, mouse button, mouse position
			if action.ActionRepeat > 1 {
				per += time.Duration(action.ActionRepeat) * 10 * time.Millisecond
				per += time.Duration(action.ActionRepeat-1) * time.Duration(action.ActionRepeatDelay) * time.Millisecond
			}
		case 5: // delay
			per += time.Duration(action.ActionDelay) * time.Millisecond
		case 6: // text
			per += time.Duration(len([]rune(action.ActionText))) * 40 * time.Millisecond
			if action.ActionRepeat > 1 {
				per += time.Duration(action.ActionRepeat-1) * time.Duration(action.ActionRepeatDelay) * time.Millisecond
			}
		}
	}
	cycle := per + time.Duration(profile.RepeatDelay)*time.Millisecond
	if profile.Repeat < 0 {
		return cycle
	}
	total := time.Duration(repeats)*per + time.Duration(repeats-1)*time.Duration(profile.RepeatDelay)*time.Millisecond
	return total
}

// Dim scales an already-computed colour, e.g. for a faint echo zone.
func Dim(color rgb.Color, factor float64) rgb.Color {
	return scaled(color, factor)
}

func scaled(base rgb.Color, factor float64) rgb.Color {
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
