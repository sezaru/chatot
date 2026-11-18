package audio

import (
	"fmt"
	"os/exec"
	"strings"
)

// Source is one microphone the sound server offers.
type Source struct {
	// Name is what Recorder.Source takes, e.g.
	// "alsa_input.pci-0000_00_1f.3.analog-stereo".
	Name string
	// Description is the human name, e.g. "Built-in Audio Analog Stereo".
	Description string
	// Default marks the source the system records from by default.
	Default bool
}

// Sources lists the PulseAudio (or PipeWire's pulse server) sources that
// are microphones, the system default first marked. Monitors of outputs
// are left out: they capture what the computer plays, not a voice.
// Spawns ffmpeg, which takes a fraction of a second: call it off the main
// thread.
func Sources() ([]Source, error) {
	out, err := exec.Command("ffmpeg", "-hide_banner", "-sources", "pulse").Output()
	srcs := parseSources(string(out))
	if len(srcs) == 0 && err != nil {
		return nil, fmt.Errorf("chatot/audio: list sources: %w", err)
	}
	return srcs, nil
}

// parseSources reads `ffmpeg -sources pulse` output: a heading, then one
// line per source, "* name [description] (none)" for the default and
// "  name [description] (none)" for the rest.
func parseSources(out string) []Source {
	var srcs []Source
	for _, line := range strings.Split(out, "\n") {
		def := strings.HasPrefix(line, "* ")
		if !def && !strings.HasPrefix(line, "  ") {
			continue
		}
		line = strings.TrimSpace(line[2:])
		open := strings.Index(line, " [")
		close := strings.LastIndex(line, "]")
		if open <= 0 || close < open {
			continue
		}
		s := Source{Name: line[:open], Description: line[open+2 : close], Default: def}
		if strings.HasSuffix(s.Name, ".monitor") {
			continue
		}
		srcs = append(srcs, s)
	}
	return srcs
}
