package selftest

import "time"

const incolumitasWaitTimeout = 25 * time.Second

// incolumitasReadyJS returns the page text once the new-tests JSON block appears.
const incolumitasReadyJS = `
() => {
  var text = document.body ? document.body.innerText : '';
  return text.indexOf('"puppeteerEvaluationScript"') >= 0 ? text : null;
}
`

// incolumitasExtractJS extracts all relevant JSON blocks and text from the page.
const incolumitasExtractJS = `
() => {
  var text = document.body ? document.body.innerText : '';
  if (!text) return null;

  // Helper: find the first balanced JSON object starting at idx.
  function extractJSON(src, start) {
    var depth = 0;
    var inStr = false;
    var esc = false;
    for (var i = start; i < src.length; i++) {
      var c = src[i];
      if (esc) { esc = false; continue; }
      if (inStr) {
        if (c === '\\') esc = true;
        else if (c === '"') inStr = false;
        continue;
      }
      if (c === '"') inStr = true;
      else if (c === '{') depth++;
      else if (c === '}') {
        depth--;
        if (depth === 0) return src.slice(start, i + 1);
      }
    }
    return null;
  }

  var result = { newTests: null, fpscanner: null, intoli: null, ipInfo: null, behavioralScore: null };

  // New tests block: first JSON containing puppeteerEvaluationScript.
  var idx = text.indexOf('"puppeteerEvaluationScript"');
  if (idx >= 0) {
    var start = text.lastIndexOf('{', idx);
    if (start >= 0) result.newTests = extractJSON(text, start);
  }

  // fpscanner block: first JSON containing "fpscanner" key.
  var fpsIdx = text.indexOf('"fpscanner"');
  if (fpsIdx >= 0) {
    var fpsStart = text.lastIndexOf('{', fpsIdx);
    if (fpsStart >= 0) result.fpscanner = extractJSON(text, fpsStart);
  }

  // intoli block: JSON containing "hasChrome" or "intoli".
  var intoliIdx = text.indexOf('"hasChrome"');
  if (intoliIdx < 0) intoliIdx = text.indexOf('"intoli"');
  if (intoliIdx >= 0) {
    var intoliStart = text.lastIndexOf('{', intoliIdx);
    if (intoliStart >= 0) result.intoli = extractJSON(text, intoliStart);
  }

  // IP info: JSON containing "is_datacenter".
  var ipIdx = text.indexOf('"is_datacenter"');
  if (ipIdx >= 0) {
    var ipStart = text.lastIndexOf('{', ipIdx);
    if (ipStart >= 0) result.ipInfo = extractJSON(text, ipStart);
  }

  // Behavioral score: "Behavioral Score: NN".
  var bsIdx = text.indexOf('Behavioral Score:');
  if (bsIdx >= 0) {
    var bsEnd = text.indexOf('\n', bsIdx);
    if (bsEnd < 0) bsEnd = Math.min(bsIdx + 80, text.length);
    result.behavioralScore = text.slice(bsIdx, bsEnd).trim();
  }

  return JSON.stringify(result);
}
`
