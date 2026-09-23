package ui

// #cgo pkg-config: gtk4
// #include <gtk/gtk.h>
import "C"

import (
	"runtime"
	"unsafe"

	"github.com/diamondburned/gotk4/pkg/core/gerror"
	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// mediaStreamError is stream's error, nil when it has none. It stands in
// for gotk4's MediaStream.Error, which frees the GError it reads although
// gtk_media_stream_get_error hands it out transfer-none: the stream keeps
// pointing at freed memory, and the next read of it (every handler of the
// same "error" notify reads it) crashed the app with a SIGSEGV — a clip
// whose audio sink timed out was enough.
func mediaStreamError(stream *gtk.MediaStream) error {
	// Native is a uintptr; read it back as a pointer the way vet accepts.
	// KeepAlive below holds the object for the call, as gotk4 itself does.
	addr := coreglib.InternObject(stream).Native()
	native := (*C.GtkMediaStream)(*(*unsafe.Pointer)(unsafe.Pointer(&addr)))
	err := C.gtk_media_stream_get_error(native)
	runtime.KeepAlive(stream)
	if err == nil {
		return nil
	}
	return gerror.Copy(unsafe.Pointer(err))
}
