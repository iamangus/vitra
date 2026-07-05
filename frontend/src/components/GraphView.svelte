<script>
  import { createEventDispatcher, onMount } from 'svelte';
  import ForceGraph3D from '3d-force-graph';
  import * as THREE from 'three';
  import { graph } from '../lib/api.js';
  import { subscribeToLiveUpdates } from '../lib/live.js';
  import { theme } from '../stores/theme.js';

  const dispatch = createEventDispatcher();

  let container = $state(null);
  let loading = $state(true);
  let error = $state(null);
  let selectedNodeId = $state(null);
  let hoverNodeId = $state(null);
  let showAllLabels = $state(false);
  let nodeCount = $state(0);
  let linkCount = $state(0);
  let hasData = $state(false);

  let graphInstance = null;
  let graphDataObj = { nodes: [], links: [] };
  let _selectedId = null;
  let cachedSelectedConnected = new Set();
  let _hoverId = null;
  let cachedHoverConnected = new Set();
  let graphReloadTimeout = null;
  let themeUnsub = null;
  let labelSprites = null;
  let isDarkMode = $state(false);

  isDarkMode = isDark();

  onMount(() => {
    showAllLabels = localStorage.getItem('graphShowAllLabels') === 'true';
    loadGraph();

    const liveUnsub = subscribeToLiveUpdates((event) => {
      if (!event.graph) return;
      clearTimeout(graphReloadTimeout);
      graphReloadTimeout = setTimeout(() => loadGraph(true), 250);
    });

    themeUnsub = theme.subscribe(() => {
      if (graphInstance) updateTheme();
    });

    return () => {
      clearTimeout(graphReloadTimeout);
      liveUnsub();
      if (themeUnsub) themeUnsub();
    };
  });

  $effect(() => {
    if (!container) return;
    if (graphInstance) return;

    labelSprites = new Map();

    graphInstance = ForceGraph3D()(container)
      .nodeId('id')
      .linkSource('sourceId')
      .linkTarget('targetId')
      .backgroundColor(isDark() ? '#0a0a0c' : '#fafafa')
      .nodeLabel(node => node.title)
      .nodeVal(node => node.val || 1)
      .nodeColor(node => getNodeColor(node))
      .linkColor(link => getLinkColor(link))
      .linkWidth(1)
      .nodeRelSize(6)
      .nodeResolution(24)
      .nodeThreeObject(makeLabelSprite)
      .nodeThreeObjectExtend(true)
      .onNodeClick(handleNodeClick)
      .onNodeHover(handleNodeHover)
      .onBackgroundClick(() => { selectedNodeId = null; refreshRender(); })
      .enableNodeDrag(true)
      .showNavInfo(false)
      .warmupTicks(30)
      .d3VelocityDecay(0.3)
      .d3AlphaDecay(0.015);

    graphInstance.d3Force('charge').strength(-80);

    if (graphDataObj.nodes.length > 0) {
      graphInstance.graphData(graphDataObj);
    }
  });

  function isDark() {
    return document.documentElement.classList.contains('dark');
  }

  function getSelectedConnected() {
    if (selectedNodeId === _selectedId) return cachedSelectedConnected;
    _selectedId = selectedNodeId;
    cachedSelectedConnected = new Set();
    if (!selectedNodeId) return cachedSelectedConnected;
    cachedSelectedConnected.add(selectedNodeId);
    for (const link of graphDataObj.links) {
      if (link.sourceId === selectedNodeId) cachedSelectedConnected.add(link.targetId);
      else if (link.targetId === selectedNodeId) cachedSelectedConnected.add(link.sourceId);
    }
    return cachedSelectedConnected;
  }

  function getHoverConnected() {
    if (hoverNodeId === _hoverId) return cachedHoverConnected;
    _hoverId = hoverNodeId;
    cachedHoverConnected = new Set();
    if (!hoverNodeId) return cachedHoverConnected;
    cachedHoverConnected.add(hoverNodeId);
    for (const link of graphDataObj.links) {
      if (link.sourceId === hoverNodeId) cachedHoverConnected.add(link.targetId);
      else if (link.targetId === hoverNodeId) cachedHoverConnected.add(link.sourceId);
    }
    return cachedHoverConnected;
  }

  function getNodeColor(node) {
    const dark = isDark();
    const selConn = getSelectedConnected();
    const hovConn = getHoverConnected();
    const base = typeColors[node.type || 'Note'] || typeColors['Note'];
    if (node.id === selectedNodeId) return dark ? '#c084fc' : '#6d28d9';
    if (node.id === hoverNodeId) return dark ? '#c084fc' : '#6d28d9';
    if (selConn.has(node.id)) return dark ? lighten(base, 0.85) : darken(base, 0.7);
    if (hovConn.has(node.id)) return dark ? lighten(base, 0.85) : darken(base, 0.7);
    if (selectedNodeId) return dark ? 'rgba(168, 85, 247, 0.28)' : 'rgba(124, 58, 237, 0.28)';
    return base;
  }

  const typeColors = {
    'Note':       'rgba(124, 58, 237, 0.82)',
    'Metric':     'rgba(59, 130, 246, 0.82)',
    'Runbook':    'rgba(239, 68, 68, 0.82)',
    'API':        'rgba(34, 197, 94, 0.82)',
    'Table':      'rgba(245, 158, 11, 0.82)',
    'Skill':      'rgba(168, 85, 247, 0.82)',
    'Definition': 'rgba(20, 184, 166, 0.82)',
    'Reference':  'rgba(236, 72, 153, 0.82)',
  };

  function lighten(rgba, amt) {
    const m = rgba.match(/[\d.]+/g);
    if (!m || m.length < 4) return rgba;
    const [r, g, b, a] = [parseFloat(m[0]), parseFloat(m[1]), parseFloat(m[2]), parseFloat(m[3])];
    return `rgba(${Math.min(255, r * amt)}, ${Math.min(255, g * amt)}, ${Math.min(255, b * amt)}, ${a})`;
  }

  function darken(rgba, amt) {
    const m = rgba.match(/[\d.]+/g);
    if (!m || m.length < 4) return rgba;
    const [r, g, b, a] = [parseFloat(m[0]), parseFloat(m[1]), parseFloat(m[2]), parseFloat(m[3])];
    return `rgba(${Math.floor(r * amt)}, ${Math.floor(g * amt)}, ${Math.floor(b * amt)}, ${a})`;
  }

  function getLinkColor(link) {
    const dark = isDark();
    const selConn = getSelectedConnected();
    const hovConn = getHoverConnected();
    const touchesHover = hovConn.has(link.sourceId) && hovConn.has(link.targetId);
    const touchesSelection = selConn.has(link.sourceId) && selConn.has(link.targetId);
    if (touchesHover) return dark ? 'rgba(192, 132, 252, 0.78)' : 'rgba(109, 40, 217, 0.75)';
    if (touchesSelection) return dark ? 'rgba(168, 85, 247, 0.6)' : 'rgba(124, 58, 237, 0.6)';
    if (selectedNodeId) return dark ? 'rgba(139, 139, 152, 0.06)' : 'rgba(161, 161, 171, 0.08)';
    return dark ? 'rgba(139, 139, 152, 0.25)' : 'rgba(161, 161, 171, 0.28)';
  }

  function makeLabelSprite(node) {
    const canvas = document.createElement('canvas');
    const ctx = canvas.getContext('2d');
    const text = node.title || '';
    const fontSize = 128;
    ctx.font = `bold ${fontSize}px -apple-system, system-ui, sans-serif`;
    const metrics = ctx.measureText(text);
    const padding = fontSize * 0.3;
    canvas.width = metrics.width + padding * 2;
    canvas.height = fontSize * 1.5;

    const dark = isDarkMode;
    ctx.font = `bold ${fontSize}px -apple-system, system-ui, sans-serif`;
    ctx.textBaseline = 'middle';
    ctx.fillStyle = dark ? 'rgba(212, 180, 252, 0.92)' : 'rgba(76, 29, 149, 0.88)';
    ctx.fillText(text, padding, canvas.height / 2);

    const texture = new THREE.CanvasTexture(canvas);
    texture.minFilter = THREE.LinearFilter;
    texture.generateMipmaps = false;
    const material = new THREE.SpriteMaterial({ map: texture, transparent: true, depthTest: false, depthWrite: false });
    const sprite = new THREE.Sprite(material);
    const aspect = canvas.width / canvas.height;
    const labelHeight = 10;
    sprite.scale.set(labelHeight * aspect, labelHeight, 1);
    const radius = Math.cbrt((node.val || 1) * 6);
    sprite.position.set(0, radius + 4, 0);
    sprite.renderOrder = 999;
    sprite.visible = false;
    labelSprites.set(node.id, sprite);
    return sprite;
  }

  function shouldShowLabel(id) {
    if (showAllLabels) return true;
    if (id === selectedNodeId || id === hoverNodeId) return true;
    const selConn = getSelectedConnected();
    const hovConn = getHoverConnected();
    if (selConn.has(id) || hovConn.has(id)) return true;
    return false;
  }

  function refreshRender() {
    if (!graphInstance) return;
    graphInstance.nodeColor(node => getNodeColor(node));
    graphInstance.linkColor(link => getLinkColor(link));
    if (labelSprites) {
      for (const [id, sprite] of labelSprites) {
        sprite.visible = shouldShowLabel(id);
      }
    }
  }

  function handleNodeClick(node, event) {
    if (!node) {
      selectedNodeId = null;
      refreshRender();
      return;
    }
    if (selectedNodeId === node.id) {
      dispatch('navigate', node.id);
      return;
    }
    selectedNodeId = node.id;
    refreshRender();
  }

  function handleNodeHover(node) {
    hoverNodeId = node ? node.id : null;
    refreshRender();
  }

  function updateTheme() {
    if (!graphInstance) return;
    isDarkMode = isDark();
    graphInstance.backgroundColor(isDarkMode ? '#0a0a0c' : '#fafafa');
    labelSprites = new Map();
    if (graphDataObj.nodes.length > 0) {
      graphInstance.graphData(graphDataObj);
      setTimeout(() => refreshRender(), 100);
    } else {
      graphInstance.refresh();
    }
  }

  function resetView() {
    if (!graphInstance) return;
    graphInstance.zoomToFit(400, 35);
  }

  function toggleAllLabels() {
    showAllLabels = !showAllLabels;
    localStorage.setItem('graphShowAllLabels', showAllLabels ? 'true' : 'false');
    if (labelSprites) {
      for (const [id, sprite] of labelSprites) {
        sprite.visible = shouldShowLabel(id);
      }
    }
  }

  function preprocessData(data) {
    const maxLinks = Math.max(1, ...data.nodes.map(n => {
      let count = 0;
      for (const link of data.links) {
        if (link.source === n.id || link.target === n.id) count++;
      }
      return count;
    }));

    const processedNodes = data.nodes.map(n => {
      let links = 0;
      for (const link of data.links) {
        if (link.source === n.id || link.target === n.id) links++;
      }
      const linkRatio = links / maxLinks;
      return {
        id: n.id,
        title: n.title,
        type: n.type || 'Note',
        val: 1 + Math.pow(linkRatio, 1.7) * 6,
      };
    });

    const validIds = new Set(processedNodes.map(n => n.id));
    const processedLinks = data.links
      .filter(l => validIds.has(l.source) && validIds.has(l.target))
      .map(l => ({
        sourceId: l.source,
        targetId: l.target,
      }));

    return { nodes: processedNodes, links: processedLinks };
  }

  function updateGraph(isLive = false) {
    if (!graphInstance) return;

    if (isLive) {
      const prevData = graphInstance.graphData();
      if (prevData && prevData.nodes) {
        const posMap = new Map();
        for (const n of prevData.nodes) {
          if (n.x !== undefined) posMap.set(n.id, { x: n.x, y: n.y, z: n.z });
        }
        for (const node of graphDataObj.nodes) {
          const saved = posMap.get(node.id);
          if (saved) {
            node.x = saved.x;
            node.y = saved.y;
            node.z = saved.z;
          }
        }
      }
    }

    graphInstance.graphData(graphDataObj);

    if (!isLive && graphDataObj.nodes.length > 0) {
      setTimeout(() => graphInstance.zoomToFit(400, 40), 200);
    }
  }

  async function loadGraph(isLive = false) {
    error = null;
    try {
      const data = await graph.get();
      if (!data || !Array.isArray(data.nodes)) {
        throw new Error('Invalid graph data');
      }
      const processed = preprocessData(data);
      graphDataObj = processed;
      nodeCount = processed.nodes.length;
      linkCount = processed.links.length;
      hasData = processed.nodes.length > 0;
      _selectedId = null;
      _hoverId = null;
      if (graphInstance) {
        updateGraph(isLive);
        refreshRender();
      }
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }
</script>

<div class="graph-view" bind:this={container}>
  {#if loading}
    <div class="loading">Loading graph...</div>
  {:else if error}
    <div class="error">{error}</div>
  {:else}
    {#if !hasData}
      <div class="empty graph-overlay">No notes found. Create some notes with [[WikiLinks]] to see the graph.</div>
    {/if}
    <div class="graph-controls">
      <button onclick={resetView} title="Fit to view" disabled={nodeCount === 0}>
        <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 3h6v6M9 21H3v-6M21 3l-7 7M3 21l7-7"/></svg>
      </button>
      <button
        class="icon-toggle"
        class:active={showAllLabels}
        onclick={toggleAllLabels}
        title="Toggle always-visible node labels"
        aria-label="Toggle always-visible node labels"
        aria-pressed={showAllLabels}
      >
        <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <circle cx="5" cy="6" r="1.5" fill="currentColor"/>
          <circle cx="5" cy="12" r="1.5" fill="currentColor"/>
          <circle cx="5" cy="18" r="1.5" fill="currentColor"/>
          <path d="M9 6h10"/>
          <path d="M9 12h10"/>
          <path d="M9 18h10"/>
        </svg>
      </button>
      <span class="graph-info">{nodeCount} notes · {linkCount} links</span>
      {#if selectedNodeId}
        <span class="graph-info emphasis">Click selected note again to open it</span>
      {/if}
    </div>
  {/if}
</div>

<style>
  .graph-view {
    width: 100%;
    height: 100%;
    position: relative;
    overflow: hidden;
    background: var(--bg);
  }

  .graph-view :global(canvas) {
    display: block;
  }

  .loading,
  .error,
  .empty {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100%;
    color: var(--color-muted);
    font-size: 0.875rem;
    padding: 2rem;
    text-align: center;
  }

  .error {
    color: var(--accent-missing);
  }

  .graph-controls {
    position: absolute;
    bottom: 1rem;
    left: 1rem;
    display: flex;
    align-items: center;
    gap: 0.75rem;
    z-index: 10;
    flex-wrap: wrap;
  }

  .graph-controls button {
    padding: 0.4rem;
    border-radius: var(--radius-sm);
    background: var(--bg-elevated);
    border: 1px solid var(--border-color);
    color: var(--color-muted);
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    transition: all 0.15s;
    box-shadow: var(--shadow-sm);
  }

  .graph-controls button:hover {
    background: var(--hover-bg);
    color: var(--color);
  }

  .graph-controls button:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .graph-controls button:disabled:hover {
    background: var(--bg-elevated);
    color: var(--color-muted);
  }

  .graph-controls button.active {
    background: var(--primary-soft);
    color: var(--primary);
    border-color: color-mix(in srgb, var(--primary) 35%, var(--border-color));
  }

  .graph-controls button.icon-toggle {
    width: 2rem;
    height: 2rem;
    padding: 0;
    color: var(--color-faint);
  }

  .graph-controls button.icon-toggle svg {
    display: block;
    stroke: currentColor;
    color: inherit;
  }

  .graph-controls button.icon-toggle:hover {
    color: var(--color);
  }

  .graph-controls button.icon-toggle.active {
    background: var(--primary-soft);
    color: var(--primary);
    border-color: color-mix(in srgb, var(--primary) 35%, var(--border-color));
  }

  .graph-info {
    font-size: 0.75rem;
    color: var(--color-faint);
    user-select: none;
  }

  .graph-info.emphasis {
    color: var(--color-muted);
  }

  .graph-overlay {
    position: absolute;
    inset: 0;
    z-index: 5;
    background: color-mix(in srgb, var(--bg) 82%, transparent);
    pointer-events: none;
  }
</style>
