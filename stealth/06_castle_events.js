// Castle.io behavioral event generation.
// Castle SDK Part 3 captures DOM interaction events (mousemove, click, keydown, scroll).
// CDP Input.dispatch* bypasses DOM event listeners, so Castle sees no human activity.
// This module dispatches synthetic DOM events that Castle's listeners can capture.

window.__castleWarmup = function(durationMs) {
  durationMs = durationMs || 3000;
  const start = Date.now();
  let eventCount = 0;

  // Random point within viewport
  const randX = () => Math.floor(Math.random() * (window.innerWidth - 100)) + 50;
  const randY = () => Math.floor(Math.random() * (window.innerHeight - 100)) + 50;

  return new Promise(resolve => {
    const interval = setInterval(() => {
      const elapsed = Date.now() - start;
      if (elapsed >= durationMs) {
        clearInterval(interval);
        resolve(eventCount);
        return;
      }

      const x = randX(), y = randY();
      const opts = {bubbles: true, cancelable: true, clientX: x, clientY: y, button: 0};

      // Mousemove — most important for Castle behavioral tracking
      document.dispatchEvent(new MouseEvent('mousemove', opts));
      eventCount++;

      // Occasional click (every ~500ms)
      if (Math.random() < 0.15) {
        document.dispatchEvent(new PointerEvent('pointerdown', opts));
        document.dispatchEvent(new MouseEvent('mousedown', opts));
        document.dispatchEvent(new PointerEvent('pointerup', opts));
        document.dispatchEvent(new MouseEvent('mouseup', opts));
        eventCount += 4;
      }

      // Occasional scroll
      if (Math.random() < 0.1) {
        window.dispatchEvent(new WheelEvent('wheel', {
          deltaY: (Math.random() - 0.5) * 200,
          bubbles: true
        }));
        eventCount++;
      }

      // Occasional keydown (focus events)
      if (Math.random() < 0.05) {
        document.dispatchEvent(new KeyboardEvent('keydown', {
          key: 'Tab', code: 'Tab', bubbles: true
        }));
        document.dispatchEvent(new KeyboardEvent('keyup', {
          key: 'Tab', code: 'Tab', bubbles: true
        }));
        eventCount += 2;
      }
    }, 50); // ~20 events/sec = 60+ events in 3 seconds
  });
};
