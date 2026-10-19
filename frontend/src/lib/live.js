const listeners = new Set();

let eventSource = null;

function ensureEventSource() {
  if (eventSource || typeof window === 'undefined') {
    return;
  }

  eventSource = new EventSource('/api/events');
  eventSource.onopen = () => {
    for (const listener of listeners) {
      try { listener({ tree: true, graph: true, search: true, notes: true, activity: true }); }
      catch (error) { console.error('Live update listener failed:', error); }
    }
  };
  eventSource.addEventListener('vault', (event) => {
    try {
      const payload = JSON.parse(event.data);
      for (const listener of listeners) {
        try { listener(payload); }
        catch (error) { console.error('Live update listener failed:', error); }
      }
    } catch (error) {
      console.error('Failed to parse live update payload:', error);
    }
  });

  eventSource.onerror = () => {
    // Let EventSource handle reconnects automatically.
  };
}

function closeEventSourceIfIdle() {
  if (listeners.size > 0 || !eventSource) {
    return;
  }

  eventSource.close();
  eventSource = null;
}

export function subscribeToLiveUpdates(listener) {
  listeners.add(listener);
  ensureEventSource();

  return () => {
    listeners.delete(listener);
    closeEventSourceIfIdle();
  };
}
