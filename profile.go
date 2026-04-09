package browser

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed stealth/profiles/*.json
var profileFS embed.FS

// StealthProfile holds browser fingerprint data loaded from a JSON profile file.
type StealthProfile struct {
	ID       string            `json:"id"`
	OS       string            `json:"os"`
	Browser  string            `json:"browser"`
	Version  string            `json:"version"`
	Platform string            `json:"platform"`
	UA       string            `json:"userAgent"`
	UAData   UAData            `json:"userAgentData"`
	Screen   ScreenProfile     `json:"screen"`
	GPU      GPUProfile        `json:"gpu"`
	Hardware HardwareProfile   `json:"hardware"`
	Langs    []string          `json:"languages"`
	Timezone string            `json:"timezone"`
	Conn     ConnectionProfile `json:"connection"`
	Fonts    []string          `json:"fonts,omitempty"`
	Plugins  []PluginDef       `json:"plugins,omitempty"`
	Voices   []VoiceDef        `json:"voices,omitempty"`
}

// UAData holds User-Agent Client Hints data.
type UAData struct {
	Brands          []Brand `json:"brands"`
	FullVersionList []Brand `json:"fullVersionList"`
	Mobile          bool    `json:"mobile"`
	Platform        string  `json:"platform"`
	FullVersion     string  `json:"fullVersion"`
	PlatformVersion string  `json:"platformVersion"`
	Architecture    string  `json:"architecture"`
	Model           string  `json:"model"`
	Bitness         string  `json:"bitness"`
	Wow64           bool    `json:"wow64"`
}

// Brand represents a single entry in the UA brands list.
type Brand struct {
	Brand   string `json:"brand"`
	Version string `json:"version"`
}

// ScreenProfile holds screen/display metrics.
type ScreenProfile struct {
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	AvailWidth  int     `json:"availWidth"`
	AvailHeight int     `json:"availHeight"`
	ColorDepth  int     `json:"colorDepth"`
	PixelDepth  int     `json:"pixelDepth"`
	DPR         float64 `json:"devicePixelRatio"`
}

// GPUProfile holds WebGL vendor/renderer strings.
type GPUProfile struct {
	Vendor   string `json:"vendor"`
	Renderer string `json:"renderer"`
}

// HardwareProfile holds hardware concurrency and touch point data.
type HardwareProfile struct {
	Concurrency    int `json:"hardwareConcurrency"`
	DeviceMemory   int `json:"deviceMemory"`
	MaxTouchPoints int `json:"maxTouchPoints"`
}

// ConnectionProfile holds Network Information API values.
type ConnectionProfile struct {
	Type          string  `json:"type"`
	Downlink      float64 `json:"downlink"`
	RTT           int     `json:"rtt"`
	EffectiveType string  `json:"effectiveType"`
}

// PluginDef defines a single navigator.plugins entry.
type PluginDef struct {
	Name        string `json:"name"`
	Filename    string `json:"filename"`
	Description string `json:"description"`
}

// VoiceDef defines a single SpeechSynthesis voice entry.
type VoiceDef struct {
	Name     string `json:"name"`
	Lang     string `json:"lang"`
	VoiceURI string `json:"voiceURI"`
	Default  bool   `json:"default,omitempty"`
}

const defaultProfileName = "mac_chrome145"

// LoadProfile loads a stealth profile by name. Pass "" to load the default profile.
func LoadProfile(name string) (*StealthProfile, error) {
	if name == "" {
		name = defaultProfileName
	}
	data, err := profileFS.ReadFile("stealth/profiles/" + name + ".json")
	if err != nil {
		return nil, fmt.Errorf("profile %q: %w", name, err)
	}
	var p StealthProfile
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("profile %q: parse: %w", name, err)
	}
	return &p, nil
}

// ListProfiles returns all available profile names.
func ListProfiles() []string {
	entries, _ := profileFS.ReadDir("stealth/profiles")
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			names = append(names, strings.TrimSuffix(e.Name(), ".json"))
		}
	}
	return names
}

// InjectJS returns a JS snippet that assigns the profile to window.__stealthProfile.
func (p *StealthProfile) InjectJS() string {
	data, _ := json.Marshal(p)
	return fmt.Sprintf("window.__stealthProfile = %s; window.__sp = window.__stealthProfile;", data)
}
