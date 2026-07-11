//go:build !linux || !desktop

package spellcheck

// Set is a no-op outside the Linux desktop build; other platforms'
// webviews (WebView2, WKWebView) spellcheck based on OS settings.
func Set(enabled bool, lang string) {}
