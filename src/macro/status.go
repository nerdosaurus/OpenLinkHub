package macro

// Package: macro
// License: GPL-3.0 or later

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// Running is one live macro execution. It is exposed through
// /api/macro/status so clients can show a running indicator and a stop button.
type Running struct {
	MacroId      int    `json:"macroId"`
	MacroName    string `json:"macroName"`
	Repeat       int    `json:"repeat"`
	DeviceSerial string `json:"deviceSerial"`
	TriggerKey   int    `json:"triggerKey"`
	TriggerRow   int    `json:"triggerRow"`
	Cycle        int    `json:"cycle"`     // loop passes completed
	StartedAt    int64  `json:"startedAt"` // Unix milliseconds
	Started      string `json:"started"`   // RFC3339, for humans
	stop         func()
}

var (
	runningMutex sync.Mutex
	running      = map[string]*Running{}
)

func runningKey(serial string, macroId, triggerKey int) string {
	return fmt.Sprintf("%s:%d:%d", serial, macroId, triggerKey)
}

// RegisterRunning records a started macro and returns its registry key.
func RegisterRunning(trigger Trigger, profile *Macro, stop func()) string {
	key := runningKey(trigger.Serial, profile.Id, trigger.Key)
	now := time.Now()
	runningMutex.Lock()
	running[key] = &Running{
		MacroId:      profile.Id,
		MacroName:    profile.Name,
		Repeat:       profile.Repeat,
		DeviceSerial: trigger.Serial,
		TriggerKey:   trigger.Key,
		TriggerRow:   trigger.Row,
		StartedAt:    now.UnixMilli(),
		Started:      now.Format(time.RFC3339),
		stop:         stop,
	}
	runningMutex.Unlock()
	return key
}

// BumpRunningCycle records one completed loop pass for the running macro.
func BumpRunningCycle(key string) {
	runningMutex.Lock()
	if item, ok := running[key]; ok {
		item.Cycle++
	}
	runningMutex.Unlock()
}

// UnregisterRunning removes a finished macro.
func UnregisterRunning(key string) {
	runningMutex.Lock()
	delete(running, key)
	runningMutex.Unlock()
}

// GetRunning returns a snapshot sorted by start time.
func GetRunning() []Running {
	runningMutex.Lock()
	out := make([]Running, 0, len(running))
	for _, item := range running {
		copied := *item
		copied.stop = nil
		out = append(out, copied)
	}
	runningMutex.Unlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedAt == out[j].StartedAt {
			return out[i].MacroId < out[j].MacroId
		}
		return out[i].StartedAt < out[j].StartedAt
	})
	return out
}

// StopRunning stops all executions of macroId, optionally limited to one
// device. It returns the number of executions asked to stop.
func StopRunning(macroId int, serial string) int {
	runningMutex.Lock()
	stops := make([]func(), 0)
	for key, item := range running {
		if item.MacroId != macroId {
			continue
		}
		if serial != "" && item.DeviceSerial != serial {
			continue
		}
		if item.stop != nil {
			stops = append(stops, item.stop)
		}
		delete(running, key)
	}
	runningMutex.Unlock()

	for _, stop := range stops {
		stop()
	}
	return len(stops)
}
