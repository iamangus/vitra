const listeners = new Set();

let eventSource = null;

function ensureEventSource() {
  if (eventSource || typeof window === 'undefined') {
    return;
  }

  eventSource = new EventSource('/api/events');
  eventSource.addEventListener('vault', (event) => {
    try {
      const payload = JSON.parse(event.data);
      for (const listener of listeners) {
        listener(payload);
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
