// Profile loader — reads window.__stealthProfile set by Go via EvalOnNewDocument.
// All other stealth modules use window.__sp as shorthand.
// If Go didn't inject a profile, the stealth complement won't apply profile-dependent overrides.
if (window.__stealthProfile) {
  window.__sp = window.__stealthProfile;
}
