package browseractivity

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"
)

var (
	errNotActiveTab = errors.New("not an active_tab message")
	errIncognito    = errors.New("incognito dropped")
	errNoHost       = errors.New("no host after strip")
	errForbiddenURL = errors.New("forbidden url scheme")
	errUnsupported  = errors.New("unsupported browser")
)

var forbiddenSchemes = map[string]struct{}{
	"chrome":           {},
	"chrome-extension": {},
	"edge":             {},
	"extension":        {},
	"about":            {},
	"file":             {},
	"data":             {},
	"blob":             {},
	"javascript":       {},
	"view-source":      {},
	"devtools":         {},
	"moz-extension":    {},
}

// StripURL keeps scheme+host (and non-default port). It drops userinfo, path,
// query, and fragment so tokens never leave the agent.
func StripURL(raw string) (host string, stripped string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", errNoHost
	}
	candidate := raw
	if !strings.Contains(candidate, "://") {
		candidate = "https://" + candidate
	}
	parsed, err := url.Parse(candidate)
	if err != nil {
		return "", "", errNoHost
	}
	scheme := strings.ToLower(parsed.Scheme)
	if _, banned := forbiddenSchemes[scheme]; banned {
		return "", "", errForbiddenURL
	}
	if scheme != "http" && scheme != "https" {
		return "", "", errForbiddenURL
	}
	h := strings.ToLower(strings.Trim(parsed.Hostname(), "."))
	if strings.HasPrefix(h, "www.") {
		h = strings.TrimPrefix(h, "www.")
	}
	if h == "" {
		return "", "", errNoHost
	}
	port := parsed.Port()
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		port = ""
	}
	origin := scheme + "://" + h
	if port != "" {
		origin += ":" + port
	}
	return h, origin, nil
}

func clipTitle(title string) string {
	title = strings.TrimSpace(title)
	if len(title) > 512 {
		return title[:512]
	}
	return title
}

func clipProfile(profile string) string {
	profile = strings.TrimSpace(profile)
	if len(profile) > 256 {
		return profile[:256]
	}
	return profile
}

// Sanitize converts an extension message into a persistable sample.
// Incognito, internal pages, and messages without a host are dropped.
func Sanitize(msg ExtensionMessage) (Sample, bool, error) {
	if strings.TrimSpace(msg.Type) != MessageActive {
		return Sample{}, true, errNotActiveTab
	}
	if msg.Incognito {
		return Sample{}, true, errIncognito
	}
	browser := strings.ToLower(strings.TrimSpace(msg.Browser))
	exe := NormalizeExecutable(msg.Executable)
	if exe == "" {
		exe = ExecutableForBrowser(browser)
	}
	if !IsBrowserExecutable(exe) {
		return Sample{}, true, errUnsupported
	}
	if browser == "" {
		browser = BrowserForExecutable(exe)
	}
	host, stripped, err := StripURL(msg.URL)
	if err != nil {
		return Sample{}, true, err
	}
	at := msg.At.UTC()
	if at.IsZero() {
		at = time.Now().UTC()
	}
	return Sample{
		Browser:    browser,
		Executable: exe,
		Domain:     host,
		URL:        stripped,
		Title:      clipTitle(msg.Title),
		Profile:    clipProfile(msg.Profile),
		WindowID:   msg.WindowID,
		TabID:      msg.TabID,
		At:         at,
	}, false, nil
}

func ParseAndSanitize(raw []byte) (Sample, bool, error) {
	var msg ExtensionMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return Sample{}, false, err
	}
	return Sanitize(msg)
}
