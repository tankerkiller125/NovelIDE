package detect

import (
	"strings"
	"sync"

	"github.com/aaaton/golem/v4"
	"github.com/aaaton/golem/v4/dicts/en"
)

// lemmaOverrides pins the irregular forms our lexicons depend on, so a
// dictionary gap can never silently change detection behavior.
var lemmaOverrides = map[string]string{
	"slew": "slay", "slain": "slay",
	"wed": "wed", "wedded": "wed",
	"wore": "wear", "worn": "wear",
	"bore": "bear", "borne": "bear",
	"wielded": "wield", "wields": "wield",
	"became": "become",
}

var (
	lemOnce sync.Once
	lem     *golem.Lemmatizer
)

// lemmaOf normalizes any inflected word to its base form: dictionary
// lemmatization (golem, fully offline) with a small override table for the
// irregulars we care about. Unknown words come back unchanged.
func lemmaOf(word string) string {
	w := strings.ToLower(word)
	if o, ok := lemmaOverrides[w]; ok {
		return o
	}
	lemOnce.Do(func() {
		if l, err := golem.New(en.New()); err == nil {
			lem = l
		}
	})
	if lem != nil && lem.InDict(w) {
		return lem.LemmaLower(w)
	}
	return w
}
