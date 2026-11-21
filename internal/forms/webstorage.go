package forms

import "regexp"

// webStorageShim is injected into every served HTML form document, as the
// first thing the browser executes.
//
// The CSP sandbox directive served with form documents (see sandboxHandler
// in the api package) gives them an opaque origin, in which any localStorage
// or sessionStorage access throws a SecurityError. Several Winlink form
// scripts read localStorage from window.onload (e.g. the Field Situation
// Report), so the exception aborts onload and later initializers (copyGPS,
// DateTime, ...) never run.
//
// The script unconditionally replaces both storages with in-memory Web
// Storage implementations, leaving form scripts unmodified. The native
// storages are inaccessible in this context (the property access itself
// throws), so there is nothing to probe for. Values do not persist across
// page loads.
const webStorageShim = `<script>
/* Pat: Web Storage shim for CSP-sandboxed forms (opaque origin). */
(function () {
  function makeStorage() {
    var data = Object.create(null);
    var keys = [];
    return {
      getItem: function (k) { return k in data ? data[k] : null; },
      setItem: function (k, v) {
        k = String(k);
        if (!(k in data)) keys.push(k);
        data[k] = String(v);
      },
      removeItem: function (k) {
        if (k in data) { delete data[k]; keys.splice(keys.indexOf(k), 1); }
      },
      clear: function () { data = Object.create(null); keys = []; },
      key: function (i) { return i >= 0 && i < keys.length ? keys[i] : null; },
      get length() { return keys.length; }
    };
  }
  ['localStorage', 'sessionStorage'].forEach(function (name) {
    Object.defineProperty(window, name, { value: makeStorage(), configurable: true });
  });
})();
</script>`

var (
	// The (\s[^>]*)? suffix matches only the <head>/<html> tags themselves
	// (optionally with attributes), not elements like <header>.
	headTagRe = regexp.MustCompile(`(?i)<head(\s[^>]*)?>`)
	htmlTagRe = regexp.MustCompile(`(?i)<html(\s[^>]*)?>`)
)

// injectHeadScript inserts script into doc as early as possible without
// breaking the doctype (inserting before it would trigger quirks mode):
// immediately after the opening <head> tag, or after <html> when no <head>
// is found, or at the document start as a last resort.
func injectHeadScript(doc, script string) string {
	for _, re := range []*regexp.Regexp{headTagRe, htmlTagRe} {
		if loc := re.FindStringIndex(doc); loc != nil {
			return doc[:loc[1]] + script + doc[loc[1]:]
		}
	}
	return script + doc
}
