package macro

// Package: macro
// License: GPL-3.0 or later

import (
	"OpenLinkHub/src/inputmanager"
	"sync"
	"time"
)

// DeviceHooks is implemented by devices to allow Play to interact with
// device-specific key tracking and cancellation. It centralizes the macro
// playback logic that was previously copy-pasted (and drifted) across every
// device file.
type DeviceHooks interface {
	// TrackKey adds a held key to the device's macro tracker for later release.
	TrackKey(key int, value uint16, actionType uint8)
	// ReleaseKeys releases all currently tracked (held) keys.
	ReleaseKeys()
	// NewStopRepeat returns a channel that is closed to cancel any in-flight
	// per-action repeat, replacing the previous one.
	NewStopRepeat() <-chan struct{}
}

// Trigger identifies the device element that started a macro. Row is only
// meaningful on per-key devices (0 is a valid row; use Key alone for mice).
type Trigger struct {
	Serial string
	Key    int
	Row    int
}

// IndicatorHooks is optionally implemented by devices that can show a status
// animation on the key/zone that triggered the macro. Keeping it separate from
// DeviceHooks avoids forcing every device to implement it.
type IndicatorHooks interface {
	StartMacroIndicator(trigger Trigger, indicator *Indicator)
	StopMacroIndicator()
}

// LoopState tracks running macro loops, keyed by an arbitrary comparable value
// (e.g. a button bitmask or macro id). Each device owns its own LoopState so
// loops never collide across devices.
type LoopState struct {
	mu    sync.Mutex
	loops map[any]chan struct{}
}

// NewLoopState returns an initialized LoopState.
func NewLoopState() *LoopState {
	return &LoopState{loops: make(map[any]chan struct{})}
}

// Toggle returns the stop channel and true if a loop is already running for
// key; otherwise it starts a new loop and returns its stop channel and false.
// The caller must close the returned stop channel to halt a running loop.
func (l *LoopState) Toggle(key any) (stop chan struct{}, running bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if stop, ok := l.loops[key]; ok {
		delete(l.loops, key)
		return stop, true
	}
	stop = make(chan struct{})
	l.loops[key] = stop
	return stop, false
}

// Stop halts a running loop for key, if present.
func (l *LoopState) Stop(key any) {
	l.mu.Lock()
	if stop, ok := l.loops[key]; ok {
		delete(l.loops, key)
		close(stop)
	}
	l.mu.Unlock()
}

// IsRunning reports whether a loop is currently running for key.
func (l *LoopState) IsRunning(key any) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, ok := l.loops[key]
	return ok
}

func waitStop(stop <-chan struct{}, duration time.Duration) bool {
	if duration <= 0 {
		return true
	}
	select {
	case <-stop:
		return false
	case <-time.After(duration):
		return true
	}
}

// Play executes a macro profile's actions, honoring hold/repeat/loop semantics.
// A negative profile.Repeat means "loop" (toggle on/off via loopKey); zero or
// positive means "play N times" (0 plays once).
//
// trigger is used for the running-status registry and the optional indicator
// animation; it does not affect playback.
func Play(hooks DeviceHooks, profile *Macro, loopKey any, loops *LoopState, trigger Trigger) {
	if profile == nil {
		return
	}

	runMacro := func(firstRun bool, stop <-chan struct{}) bool {
		for i := 0; i < len(profile.Actions); i++ {
			select {
			case <-stop:
				return false
			default:
			}

			v, valid := profile.Actions[i]
			if !valid {
				continue
			}
			if v.ActionHold && !firstRun {
				continue
			}
			if v.ActionHold {
				hooks.TrackKey(i, v.ActionCommand, v.ActionType)
			}

			switch v.ActionType {
			case 1, 3: // keyboard
				if v.ActionRepeat > 0 && !v.ActionHold {
					localStop := hooks.NewStopRepeat()
					go func(action Actions) {
						for z := 0; z < int(action.ActionRepeat); z++ {
							select {
							case <-localStop:
								return
							default:
								inputmanager.InputControlKeyboard(action.ActionCommand, false)
							}
							if action.ActionRepeatDelay > 0 && action.ActionRepeat > 1 {
								time.Sleep(time.Duration(action.ActionRepeatDelay) * time.Millisecond)
							}
						}
					}(v)
				} else {
					inputmanager.InputControlKeyboard(v.ActionCommand, v.ActionHold)
				}
			case 9: // mouse button
				if v.ActionRepeat > 0 && !v.ActionHold {
					for z := 0; z < int(v.ActionRepeat); z++ {
						select {
						case <-stop:
							return false
						default:
						}
						inputmanager.InputControlMouse(v.ActionCommand)
						if v.ActionRepeatDelay > 0 && v.ActionRepeat > 1 {
							if !waitStop(stop, time.Duration(v.ActionRepeatDelay)*time.Millisecond) {
								return false
							}
						}
					}
				} else if v.ActionHold && v.ActionRepeat == 0 {
					inputmanager.InputControlMouseHold(v.ActionCommand, v.ActionHold)
				} else {
					inputmanager.InputControlMouse(v.ActionCommand)
				}
			case 5: // delay
				if v.ActionDelay > 0 {
					if !waitStop(stop, time.Duration(inputmanager.DelayMilliseconds(v.ActionDelay))*time.Millisecond) {
						return false
					}
				}
			case 6: // text
				if v.ActionRepeat > 0 {
					for z := 0; z < int(v.ActionRepeat); z++ {
						select {
						case <-stop:
							return false
						default:
						}
						inputmanager.InputControlKeyboardText(v.ActionText)
						if v.ActionRepeatDelay > 0 && v.ActionRepeat > 1 {
							if !waitStop(stop, time.Duration(v.ActionRepeatDelay)*time.Millisecond) {
								return false
							}
						}
					}
				} else {
					inputmanager.InputControlKeyboardText(v.ActionText)
				}
			case 20: // mouse position
				if v.ActionRepeat > 0 && !v.ActionHold {
					localStop := hooks.NewStopRepeat()
					go func(action Actions) {
						for z := 0; z < int(action.ActionRepeat); z++ {
							select {
							case <-localStop:
								return
							default:
								if action.MousePositionAbsolute {
									inputmanager.InputControlMoveAbsolute(int32(action.MousePositionX), int32(action.MousePositionY))
								} else {
									inputmanager.InputControlMove(int32(action.MousePositionX), int32(action.MousePositionY))
								}
							}
							if action.ActionRepeatDelay > 0 && action.ActionRepeat > 1 {
								time.Sleep(time.Duration(action.ActionRepeatDelay) * time.Millisecond)
							}
						}
					}(v)
				} else {
					if v.MousePositionAbsolute {
						inputmanager.InputControlMoveAbsolute(int32(v.MousePositionX), int32(v.MousePositionY))
					} else {
						inputmanager.InputControlMove(int32(v.MousePositionX), int32(v.MousePositionY))
					}
				}
			}
		}
		return true
	}

	cleanup := func() {
		hooks.NewStopRepeat()
		hooks.ReleaseKeys()
	}

	startIndicator := func() func() {
		if profile.Indicator == nil || !profile.Indicator.Enabled {
			return func() {}
		}
		if indicatorHooks, ok := hooks.(IndicatorHooks); ok {
			indicator := *profile.Indicator
			indicator.Loop = profile.Repeat < 0
			if indicator.DurationMs == 0 {
				if duration := EstimateDuration(profile); duration > 0 {
					indicator.DurationMs = duration.Milliseconds()
				}
			}
			indicatorHooks.StartMacroIndicator(trigger, &indicator)
			return indicatorHooks.StopMacroIndicator
		}
		return func() {}
	}

	if profile.Repeat < 0 {
		stop, isRunning := loops.Toggle(loopKey)
		key := runningKey(trigger.Serial, profile.Id, trigger.Key)
		if isRunning {
			close(stop)
			UnregisterRunning(key)
			cleanup()
			return
		}

		RegisterRunning(trigger, profile.Id, profile.Name, func() {
			loops.Stop(loopKey)
		})

		go func() {
			defer UnregisterRunning(key)
			stopIndicator := startIndicator()
			defer stopIndicator()

			firstRun := true
			for {
				select {
				case <-stop:
					cleanup()
					return
				default:
				}
				if !runMacro(firstRun, stop) {
					cleanup()
					return
				}
				firstRun = false
				if profile.RepeatDelay > 0 {
					if !waitStop(stop, time.Duration(profile.RepeatDelay)*time.Millisecond) {
						cleanup()
						return
					}
				}
			}
		}()
		return
	}

	stop := make(chan struct{})
	key := RegisterRunning(trigger, profile.Id, profile.Name, func() {
		close(stop)
	})
	defer UnregisterRunning(key)
	stopIndicator := startIndicator()
	defer stopIndicator()

	repeat := profile.Repeat
	if repeat <= 0 {
		repeat = 1
	}
	for r := 0; r < repeat; r++ {
		if !runMacro(r == 0, stop) {
			cleanup()
			return
		}
		if profile.RepeatDelay > 0 && r < repeat-1 {
			if !waitStop(stop, time.Duration(profile.RepeatDelay)*time.Millisecond) {
				cleanup()
				return
			}
		}
	}
}
