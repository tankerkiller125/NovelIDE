//go:build linux && desktop

// Package spellcheck enables WebKitGTK's native spellchecker for the Wails
// webview. WebKitGTK ships with spell checking DISABLED at the WebContext
// level, and the HTML spellcheck attribute alone does nothing until it is
// enabled here (backed by enchant/hunspell dictionaries on the system).
//
// The real implementation only compiles under the `desktop` build tag that
// `wails build`/`wails dev` add — mirroring how Wails gates its own webkit
// cgo code — so plain `go build ./...` needs no C headers.
package spellcheck

/*
#cgo linux pkg-config: gtk+-3.0
#cgo !webkit2_41 pkg-config: webkit2gtk-4.0
#cgo webkit2_41 pkg-config: webkit2gtk-4.1

#include <webkit2/webkit2.h>
#include <stdlib.h>

typedef struct {
	gboolean enabled;
	char *lang;
} NvSpellOpts;

static gboolean nv_apply_spellcheck(gpointer data) {
	NvSpellOpts *o = (NvSpellOpts *)data;
	WebKitWebContext *ctx = webkit_web_context_get_default();
	webkit_web_context_set_spell_checking_enabled(ctx, o->enabled);
	if (o->lang != NULL && o->lang[0] != '\0') {
		const gchar *langs[2] = { o->lang, NULL };
		webkit_web_context_set_spell_checking_languages(ctx, langs);
	}
	g_free(o->lang);
	g_free(o);
	return G_SOURCE_REMOVE;
}

// nv_set_spellcheck schedules the change on the GTK main loop — WebKit API
// must not be called from arbitrary goroutine threads.
static void nv_set_spellcheck(gboolean enabled, const char *lang) {
	NvSpellOpts *o = g_malloc0(sizeof(NvSpellOpts));
	o->enabled = enabled;
	o->lang = g_strdup(lang == NULL ? "" : lang);
	g_idle_add(nv_apply_spellcheck, o);
}
*/
import "C"

import "unsafe"

// Set enables or disables native spell checking, using the given
// dictionary language (e.g. "en_US"). Safe to call from any goroutine.
func Set(enabled bool, lang string) {
	clang := C.CString(lang)
	defer C.free(unsafe.Pointer(clang))
	var e C.gboolean
	if enabled {
		e = 1
	}
	C.nv_set_spellcheck(e, clang)
}
