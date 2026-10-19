import { test } from 'node:test';
import assert from 'node:assert/strict';
import { subscribeToLiveUpdates } from '../src/lib/live.js';

test('reconnect invalidates all views and isolates failing listeners', () => {
  const originalWindow = globalThis.window;
  const originalSource = globalThis.EventSource;
  const originalError = console.error;
  let connection;
  class FakeSource {
    callbacks = new Map();
    constructor() { connection = this; }
    addEventListener(name, callback) { this.callbacks.set(name, callback); }
    close() { this.closed = true; }
  }
  globalThis.window = {};
  globalThis.EventSource = FakeSource;
  console.error = () => {};
  let first, second;
  try {
    first = subscribeToLiveUpdates(() => { throw new Error('one listener failed'); });
    const events = [];
    second = subscribeToLiveUpdates(event => events.push(event));
    connection.onopen();
    assert.equal(events[0].activity, true);
    assert.equal(events[0].tree, true);
    connection.callbacks.get('vault')({ data: JSON.stringify({ search: true }) });
    assert.equal(events[1].search, true);
    first(); second();
    assert.equal(connection.closed, true);
  } finally {
    first?.(); second?.();
    globalThis.window = originalWindow;
    globalThis.EventSource = originalSource;
    console.error = originalError;
  }
});
