package audio

import (
	"reflect"
	"testing"
)

func TestParseSources(t *testing.T) {
	// Captured from `ffmpeg -hide_banner -sources pulse` on PipeWire 1.6.
	out := `Auto-detected sources for pulse:
* effect_output.rnnoise [Noise Canceling source] (none)
  effect_output.karaoke-mic [Karaoke Microphone (no beamformer)] (none)
  alsa_output.platform-sound.HiFi__Headphones__sink.monitor [Monitor of Built-in Audio Headphones] (none)
  alsa_input.platform-sound.HiFi__Headset__source [Built-in Audio Headset Microphone] (none)
  effect_output.j493-mic [MacBook Pro J493 Microphone] (none)
  audio_effect.j493-convolver.monitor [Monitor of MacBook Pro J493 Speakers] (none)
`
	want := []Source{
		{Name: "effect_output.rnnoise", Description: "Noise Canceling source", Default: true},
		{Name: "effect_output.karaoke-mic", Description: "Karaoke Microphone (no beamformer)"},
		{Name: "alsa_input.platform-sound.HiFi__Headset__source", Description: "Built-in Audio Headset Microphone"},
		{Name: "effect_output.j493-mic", Description: "MacBook Pro J493 Microphone"},
	}
	if got := parseSources(out); !reflect.DeepEqual(got, want) {
		t.Errorf("parseSources =\n%+v\nwant\n%+v", got, want)
	}
	if got := parseSources("Cannot list sources: Not implemented.\n"); len(got) != 0 {
		t.Errorf("parseSources(error) = %+v, want none", got)
	}
}
