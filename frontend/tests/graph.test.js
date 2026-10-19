import { test } from 'node:test';
import assert from 'node:assert/strict';
import { preprocessGraph, tooltipLabel } from '../src/lib/graph.js';

test('normalizes linkless graphs and excludes dangling edges', () => {
  assert.equal(tooltipLabel({ title: '<img onerror="bad">' }), '&lt;img onerror=&quot;bad&quot;&gt;');
  assert.deepEqual(preprocessGraph({ nodes: [], links: null }), { nodes: [], links: [] });
  const result = preprocessGraph({ nodes: [{ id: 'one' }], links: [{ source: 'one', target: 'absent' }] });
  assert.equal(result.links.length, 0);
  assert.equal(result.nodes[0].val, 1);
});

test('processes a large graph without mutating source IDs', context => {
  const nodes = Array.from({ length: 10000 }, (_, index) => ({ id: String(index), title: String(index) }));
  const links = Array.from({ length: 50000 }, (_, index) => ({ source: String(index % nodes.length), target: String((index + 1) % nodes.length) }));
  const start = performance.now();
  const result = preprocessGraph({ nodes, links });
  context.diagnostic(`10,000 nodes / 50,000 edges preprocessing: ${(performance.now() - start).toFixed(2)} ms (local observation, not a performance guarantee)`);
  assert.equal(result.nodes.length, 10000);
  assert.equal(result.links.length, 50000);
  assert.equal(links[0].source, '0');
});
