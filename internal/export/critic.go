package export

import "regexp"

// CriticMarkup is the plaintext editorial-annotation standard NovelIDE uses
// for notes/comments. These are the author's private markings and must never
// appear in an exported book — stripCriticMarkup resolves them away before
// rendering.
//
//	{>>...<<}        comment / note      -> removed
//	{==text==}       highlight           -> text (marks dropped)
//	{++text++}       insertion (accept)  -> text
//	{--text--}       deletion (accept)   -> removed
//	{~~old~>new~~}   substitution        -> new
var (
	cmComment      = regexp.MustCompile(`(?s)\{>>.*?<<\}`)
	cmSubstitution = regexp.MustCompile(`(?s)\{~~.*?~>(.*?)~~\}`)
	cmDeletion     = regexp.MustCompile(`(?s)\{--.*?--\}`)
	cmInsertion    = regexp.MustCompile(`(?s)\{\+\+(.*?)\+\+\}`)
	cmHighlight    = regexp.MustCompile(`(?s)\{==(.*?)==\}`)
)

// stripCriticMarkup removes editorial annotations, accepting all changes.
// Comments are removed first so an attached comment is gone before its
// highlight is unwrapped.
func stripCriticMarkup(s string) string {
	s = cmComment.ReplaceAllString(s, "")
	s = cmSubstitution.ReplaceAllString(s, "$1")
	s = cmDeletion.ReplaceAllString(s, "")
	s = cmInsertion.ReplaceAllString(s, "$1")
	s = cmHighlight.ReplaceAllString(s, "$1")
	return s
}

// sceneMarker matches an in-chapter scene divider comment (see
// internal/workspace/scene.go). On export the title is dropped and the
// divider becomes a thematic break, which the theme renders as its
// scene-break glyph. `***` (not `---`) is used so it can't be mistaken for a
// Setext heading underline on the preceding line.
var sceneMarker = regexp.MustCompile(`(?m)^[ \t]*<!--[ \t]*scene(?:[ \t]*:[ \t]*.*?)?[ \t]*-->[ \t]*$`)

func convertSceneBreaks(s string) string {
	return sceneMarker.ReplaceAllString(s, "***")
}
