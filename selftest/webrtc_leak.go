package selftest

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
)

const webrtcWaitTimeout = 30 * time.Second

// webrtcReadyJS returns non-null when the WebRTC table has loaded IPs.
const webrtcReadyJS = `
(function() {
  var cells = document.querySelectorAll('table td, .ip-address, [data-ip]');
  return cells.length > 0 ? String(cells.length) : null;
})()
`

// webrtcExtractJS extracts IP addresses from the browserleaks WebRTC page.
const webrtcExtractJS = `
(function() {
  var out = { publicIps: [], localIps: [], allIps: [] };

  // browserleaks.com/webrtc renders IPs in a table
  var tableRows = document.querySelectorAll('table tr');
  for (var i = 0; i < tableRows.length; i++) {
    var row = tableRows[i];
    var cells = row.querySelectorAll('td');
    if (cells.length < 2) continue;
    var value = cells[1].textContent.trim();
    if (!value || value === '-' || value === 'N/A') continue;

    // Extract IP-like values
    var ips = value.match(/\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b/g) || [];
    for (var j = 0; j < ips.length; j++) {
      var ip = ips[j];
      out.allIps.push(ip);
      // RFC1918 ranges: 10.x, 172.16-31.x, 192.168.x
      if (ip.indexOf('10.') === 0 ||
          ip.indexOf('192.168.') === 0 ||
          /^172\.(1[6-9]|2\d|3[01])\./.test(ip)) {
        out.localIps.push(ip);
      } else {
        out.publicIps.push(ip);
      }
    }
  }

  // Also check dedicated IP display elements
  var ipEls = document.querySelectorAll('.ip, [class*="ip-address"], [data-ip]');
  for (var k = 0; k < ipEls.length; k++) {
    var m = ipEls[k].textContent.trim().match(/\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b/);
    if (m) out.allIps.push(m[0]);
  }

  // Deduplicate using object keys (no spread operator needed)
  function dedup(arr) {
    var seen = {}; var out2 = [];
    for (var x = 0; x < arr.length; x++) { if (!seen[arr[x]]) { seen[arr[x]] = true; out2.push(arr[x]); } }
    return out2;
  }
  out.allIps = dedup(out.allIps);
  out.localIps = dedup(out.localIps);
  out.publicIps = dedup(out.publicIps);

  return JSON.stringify(out);
})()
`

// extractWebRTCLeak checks for IP leaks at https://browserleaks.com/webrtc
//
// Strategy: wait for the IP table to load, extract all IPs, assert no RFC1918.
// ok=true means no local (RFC1918) IPs were leaked via WebRTC.
func extractWebRTCLeak(ctx context.Context, page *rod.Page) (TargetResult, error) {
	result := TargetResult{
		Target: "webrtc_leak",
		URL:    "https://browserleaks.com/webrtc",
	}

	deadline := time.Now().Add(webrtcWaitTimeout)
	for time.Now().Before(deadline) {
		val, err := page.Eval(webrtcReadyJS)
		if err == nil && val != nil {
			s := strings.TrimSpace(val.Value.String())
			if s != "" && s != "null" {
				break
			}
		}
		select {
		case <-ctx.Done():
			return result, fmt.Errorf("webrtc_leak: context cancelled while waiting")
		case <-time.After(500 * time.Millisecond):
		}
	}

	// Extra wait for WebRTC ICE candidate gathering.
	select {
	case <-ctx.Done():
		return result, fmt.Errorf("webrtc_leak: context cancelled")
	case <-time.After(2 * time.Second):
	}

	val, err := page.Eval(webrtcExtractJS)
	if err != nil {
		return result, fmt.Errorf("webrtc_leak: eval extract: %w", err)
	}
	if val == nil || val.Value.String() == "" || val.Value.String() == "null" {
		return result, fmt.Errorf("webrtc_leak: selector not found")
	}

	var ips struct {
		PublicIPs []string `json:"publicIps"`
		LocalIPs  []string `json:"localIps"`
		AllIPs    []string `json:"allIps"`
	}
	if err := parseJSON(val.Value.String(), &ips); err != nil {
		return result, fmt.Errorf("webrtc_leak: parse result: %w", err)
	}

	// Re-classify using isRFC1918 for Go-side validation (the JS extraction
	// does a first pass; Go validates to catch edge cases).
	var confirmedLocal []string
	for _, ip := range ips.AllIPs {
		if isRFC1918(ip) {
			confirmedLocal = append(confirmedLocal, ip)
		}
	}
	if len(confirmedLocal) > 0 {
		ips.LocalIPs = confirmedLocal
	}

	// Pass if no local IPs leaked.
	leaked := len(ips.LocalIPs) > 0
	var trustScore float64
	if !leaked {
		trustScore = maxTrustScore
	}

	result.OK = !leaked
	result.TrustScore = trustScore
	result.Sections = map[string]any{
		"publicIps": ips.PublicIPs,
		"localIps":  ips.LocalIPs,
		"leaked":    leaked,
	}
	if leaked {
		result.Error = fmt.Sprintf("WebRTC leak detected: %s", strings.Join(ips.LocalIPs, ", "))
	}
	return result, nil
}

// isRFC1918 reports whether ip is in a private RFC1918 range:
// 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16.
func isRFC1918(ip string) bool {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 { //nolint:mnd // 4 octets in IPv4
		return false
	}
	switch {
	case parts[0] == "10":
		return true
	case parts[0] == "192" && parts[1] == "168":
		return true
	case parts[0] == "172":
		second := 0
		for _, c := range parts[1] {
			if c < '0' || c > '9' {
				return false
			}
			second = second*10 + int(c-'0') //nolint:mnd // decimal digit extraction
		}
		return second >= 16 && second <= 31 //nolint:mnd // RFC1918 172.16-31 range
	default:
		return false
	}
}
