<script>
  import { createEventDispatcher } from 'svelte';
  import { graph } from '../lib/api.js';

  const dispatch = createEventDispatcher();

  // Canvas refs
  let canvas = $state(null);
  let container = $state(null);
  let ctx = $state(null);
  let dpr = $state(1);

  // Graph data
  let nodes = $state([]);
  let links = $state([]);
  let nodeIndexById = new Map();
  let loading = $state(true);
  let error = $state(null);

  // Viewport
  let transform = $state({ x: 0, y: 0, k: 1 });
  let isDragging = $state(false);
  let dragNode = $state(null);
  let hoverNode = $state(null);
  let mousePos = $state({ x: 0, y: 0 });
  let dragStartPos = $state({ x: 0, y: 0 });
  let movedDuringDrag = $state(false);
  let suppressClick = $state(false);

  // Simulation params
  const REPULSION = 5000;
  const SPRING_LENGTH = 250;
  const SPRING_STRENGTH = 0.15;
  const DAMPING = 0.92;
  const CENTER_FORCE = 0.01;
  const MAX_SPEED = 4;

  let animationId = $state(null);
  let resizeObserver = $state(null);
  let simRunning = $state(false);

  // Initialize
  $effect(() => {
    dpr = window.devicePixelRatio || 1;
    loadGraph();

    return () => {
      if (animationId) cancelAnimationFrame(animationId);
      if (resizeObserver) resizeObserver.disconnect();
    };
  });

  // Setup canvas when element is bound
  $effect(() => {
    if (!canvas || !container) return;

    ctx = canvas.getContext('2d');
    if (!ctx) return;

    resizeCanvas();

    resizeObserver = new ResizeObserver(() => {
      resizeCanvas();
    });
    resizeObserver.observe(container);

    return () => {
      if (resizeObserver) resizeObserver.disconnect();
    };
  });

  $effect(() => {
    if (!canvas) return;

    const handleWindowMouseMove = (e) => {
      onMouseMove(e);
    };

    const handleWindowMouseUp = (e) => {
      onMouseUp(e);
    };

    window.addEventListener('mousemove', handleWindowMouseMove);
    window.addEventListener('mouseup', handleWindowMouseUp);

    return () => {
      window.removeEventListener('mousemove', handleWindowMouseMove);
      window.removeEventListener('mouseup', handleWindowMouseUp);
    };
  });

  // Start simulation when we have data and context
  $effect(() => {
    if (nodes.length > 0 && ctx && !simRunning && canvas) {
      simRunning = true;
      startSimulation();
    }
  });

  function resizeCanvas() {
    if (!canvas || !container || !ctx) return;
    const rect = container.getBoundingClientRect();
    const w = Math.max(1, Math.floor(rect.width));
    const h = Math.max(1, Math.floor(rect.height));

    canvas.width = w * dpr;
    canvas.height = h * dpr;
    canvas.style.width = w + 'px';
    canvas.style.height = h + 'px';

    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  }

  async function loadGraph() {
    try {
      const data = await graph.get();
      if (!data || !Array.isArray(data.nodes)) {
        throw new Error('Invalid graph data');
      }
      initGraph(data);
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  function initGraph(data) {
    const nextNodeIndexById = new Map();
    const newNodes = data.nodes.map((n, i) => ({
      id: n.id,
      title: n.title,
      // Place nodes on a circle to guarantee separation
      x: Math.cos((i / data.nodes.length) * Math.PI * 2) * 100,
      y: Math.sin((i / data.nodes.length) * Math.PI * 2) * 100,
      vx: 0,
      vy: 0,
      radius: 4,
      links: 0,
    }));

    for (const n of newNodes) {
      nextNodeIndexById.set(n.id, nextNodeIndexById.size);
    }

    const newLinks = data.links.map(l => ({
      sourceId: l.source,
      targetId: l.target,
    })).filter(l => nextNodeIndexById.has(l.sourceId) && nextNodeIndexById.has(l.targetId));

    // Count links per node and set radius by centrality
    for (const link of newLinks) {
      const s = newNodes[nextNodeIndexById.get(link.sourceId)];
      const t = newNodes[nextNodeIndexById.get(link.targetId)];
      if (s) s.links++;
      if (t) t.links++;
    }
    const maxLinks = Math.max(1, ...newNodes.map(n => n.links));
    for (const node of newNodes) {
      node.radius = 3 + (node.links / maxLinks) * 6;
    }

    nodes = newNodes;
    links = newLinks;
    nodeIndexById = nextNodeIndexById;
  }

  function startSimulation() {
    // Run 300 ticks of simulation instantly (no rendering) to settle the layout
    for (let i = 0; i < 300; i++) {
      simulate(1);
    }

    // Compute fit-to-view transform based on settled positions
    computeInitialTransform();

    let lastTime = performance.now();

    function tick(now) {
      const dt = Math.min((now - lastTime) / 16.67, 3);
      lastTime = now;

      if (!isDragging && nodes.length > 0) {
        simulate(dt);
      }
      render();
      animationId = requestAnimationFrame(tick);
    }

    animationId = requestAnimationFrame(tick);
  }

  function computeInitialTransform() {
    if (nodes.length === 0 || !canvas) return;
    let minX = Infinity, maxX = -Infinity, minY = Infinity, maxY = -Infinity;
    for (const node of nodes) {
      minX = Math.min(minX, node.x);
      maxX = Math.max(maxX, node.x);
      minY = Math.min(minY, node.y);
      maxY = Math.max(maxY, node.y);
    }
    const rect = canvas.getBoundingClientRect();
    const graphW = maxX - minX + 100;
    const graphH = maxY - minY + 100;
    const scale = Math.min(rect.width / graphW, rect.height / graphH, 2);
    const k = Math.max(0.1, scale * 0.85);
    transform = {
      k,
      x: rect.width / 2 - (minX + maxX) / 2 * k,
      y: rect.height / 2 - (minY + maxY) / 2 * k,
    };
  }

  function simulate(dt) {
    const cx = 0;
    const cy = 0;

    // Repulsion
    for (let i = 0; i < nodes.length; i++) {
      for (let j = i + 1; j < nodes.length; j++) {
        const a = nodes[i];
        const b = nodes[j];
        let dx = a.x - b.x;
        let dy = a.y - b.y;
        let dist = Math.sqrt(dx * dx + dy * dy) || 1;
        const force = (REPULSION * dt) / (dist * dist);
        const fx = (dx / dist) * force;
        const fy = (dy / dist) * force;
        a.vx += fx;
        a.vy += fy;
        b.vx -= fx;
        b.vy -= fy;
      }
    }

    // Spring attraction
    for (const link of links) {
      const a = nodes[nodeIndexById.get(link.sourceId)];
      const b = nodes[nodeIndexById.get(link.targetId)];
      if (!a || !b) continue;
      let dx = b.x - a.x;
      let dy = b.y - a.y;
      let dist = Math.sqrt(dx * dx + dy * dy) || 1;
      const force = (dist - SPRING_LENGTH) * SPRING_STRENGTH * dt;
      const fx = (dx / dist) * force;
      const fy = (dy / dist) * force;
      a.vx += fx;
      a.vy += fy;
      b.vx -= fx;
      b.vy -= fy;
    }

    // Center gravity + damping + speed limit + position update
    for (const node of nodes) {
      node.vx += (cx - node.x) * CENTER_FORCE * dt;
      node.vy += (cy - node.y) * CENTER_FORCE * dt;

      node.vx *= DAMPING;
      node.vy *= DAMPING;

      const speed = Math.sqrt(node.vx * node.vx + node.vy * node.vy);
      if (speed > MAX_SPEED) {
        node.vx = (node.vx / speed) * MAX_SPEED;
        node.vy = (node.vy / speed) * MAX_SPEED;
      }

      node.x += node.vx * dt;
      node.y += node.vy * dt;
    }
  }

  function render() {
    if (!ctx || !canvas) return;

    const rect = canvas.getBoundingClientRect();
    const w = rect.width;
    const h = rect.height;

    // Clear with background color to ensure visibility
    ctx.fillStyle = document.documentElement.classList.contains('dark') ? '#0a0a0c' : '#fafafa';
    ctx.fillRect(0, 0, w, h);

    ctx.save();
    ctx.translate(transform.x, transform.y);
    ctx.scale(transform.k, transform.k);

    // Colors
    const isDark = document.documentElement.classList.contains('dark');
    const linkColor = isDark ? 'rgba(139, 139, 152, 0.3)' : 'rgba(161, 161, 171, 0.4)';
    const nodeColor = isDark ? '#a855f7' : '#7c3aed';
    const hoverColor = isDark ? '#c084fc' : '#6d28d9';
    const textColor = isDark ? 'rgba(240, 240, 245, 0.9)' : 'rgba(26, 26, 30, 0.9)';

    // Highlight connected links if hovering
    const connectedIds = new Set();
    if (hoverNode) {
      connectedIds.add(hoverNode.id);
      for (const link of links) {
        if (link.sourceId === hoverNode.id || link.targetId === hoverNode.id) {
          connectedIds.add(link.sourceId);
          connectedIds.add(link.targetId);
        }
      }
    }

    // Draw nodes
    for (const node of nodes) {
      const highlighted = connectedIds.has(node.id);
      const isHover = node.id === hoverNode?.id;

      ctx.beginPath();
      ctx.arc(node.x, node.y, node.radius, 0, Math.PI * 2);
      ctx.fillStyle = isHover
        ? hoverColor
        : (highlighted ? nodeColor : (isDark ? 'rgba(168, 85, 247, 0.8)' : 'rgba(124, 58, 237, 0.8)'));
      ctx.fill();

      if (isHover || (highlighted && node.links > 2)) {
        ctx.beginPath();
        ctx.arc(node.x, node.y, node.radius + 3, 0, Math.PI * 2);
        ctx.strokeStyle = isDark ? 'rgba(168, 85, 247, 0.5)' : 'rgba(124, 58, 237, 0.5)';
        ctx.lineWidth = 1;
        ctx.stroke();
      }
    }

    // Draw links after nodes so the anchor point is visibly the node center.
    ctx.lineCap = 'round';
    for (const link of links) {
      const source = nodes[nodeIndexById.get(link.sourceId)];
      const target = nodes[nodeIndexById.get(link.targetId)];
      if (!source || !target) continue;
      const highlighted = hoverNode && (link.sourceId === hoverNode.id || link.targetId === hoverNode.id);
      ctx.beginPath();
      ctx.moveTo(source.x, source.y);
      ctx.lineTo(target.x, target.y);
      ctx.strokeStyle = highlighted
        ? (isDark ? 'rgba(168, 85, 247, 0.7)' : 'rgba(124, 58, 237, 0.7)')
        : linkColor;
      ctx.lineWidth = highlighted ? 1.5 : 1;
      ctx.stroke();
    }

    // Draw labels for hovered node and high-centrality nodes
    ctx.font = `${12 / transform.k}px -apple-system, BlinkMacSystemFont, sans-serif`;
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';

    for (const node of nodes) {
      const shouldLabel = node.id === hoverNode?.id || node.links >= Math.max(3, nodes.length * 0.02);
      if (!shouldLabel) continue;

      const highlighted = connectedIds.has(node.id);
      const padding = 4 / transform.k;
      const textY = node.y - node.radius / transform.k - 10 / transform.k;
      const metrics = ctx.measureText(node.title);
      const textW = metrics.width;

      // Label background
      ctx.fillStyle = isDark ? 'rgba(10, 10, 12, 0.85)' : 'rgba(250, 250, 250, 0.9)';
      ctx.beginPath();
      ctx.roundRect(
        node.x - textW / 2 - padding,
        textY - 7 / transform.k - padding,
        textW + padding * 2,
        14 / transform.k + padding * 2,
        3 / transform.k
      );
      ctx.fill();

      ctx.fillStyle = highlighted ? (isDark ? '#f0f0f5' : '#1a1a1e') : textColor;
      ctx.fillText(node.title, node.x, textY);
    }

    ctx.restore();
  }

  // Event handlers
  function getWorldPos(clientX, clientY) {
    const rect = canvas.getBoundingClientRect();
    return {
      x: (clientX - rect.left - transform.x) / transform.k,
      y: (clientY - rect.top - transform.y) / transform.k,
    };
  }

  function findNodeAt(worldX, worldY) {
    for (let i = nodes.length - 1; i >= 0; i--) {
      const node = nodes[i];
      const dx = worldX - node.x;
      const dy = worldY - node.y;
      const hitRadius = Math.max(node.radius + 6, 10);
      if (dx * dx + dy * dy < hitRadius * hitRadius) {
        return node;
      }
    }
    return null;
  }

  function clearInteractionState() {
    dragNode = null;
    isDragging = false;
    hoverNode = null;
    movedDuringDrag = false;
    if (canvas) {
      canvas.style.cursor = 'grab';
    }
  }

  function onMouseDown(e) {
    e.preventDefault();
    const pos = getWorldPos(e.clientX, e.clientY);
    const node = findNodeAt(pos.x, pos.y);

    if (node) {
      dragNode = node;
      isDragging = true;
      dragNode.vx = 0;
      dragNode.vy = 0;
    } else {
      isDragging = true;
    }

    mousePos = { x: e.clientX, y: e.clientY };
    dragStartPos = { x: e.clientX, y: e.clientY };
    movedDuringDrag = false;
  }

  function onMouseMove(e) {
    const dx = e.clientX - mousePos.x;
    const dy = e.clientY - mousePos.y;

    if (isDragging) {
      e.preventDefault();
      mousePos = { x: e.clientX, y: e.clientY };
      if (dragNode) {
        if (Math.abs(e.clientX - dragStartPos.x) > 2 || Math.abs(e.clientY - dragStartPos.y) > 2) {
          movedDuringDrag = true;
          suppressClick = true;
        }
        dragNode.x += dx / transform.k;
        dragNode.y += dy / transform.k;
        dragNode.vx = 0;
        dragNode.vy = 0;
      } else {
        transform = { ...transform, x: transform.x + dx, y: transform.y + dy };
      }
    } else {
      const pos = getWorldPos(e.clientX, e.clientY);
      hoverNode = findNodeAt(pos.x, pos.y);
      if (canvas) canvas.style.cursor = hoverNode ? 'pointer' : 'grab';
    }
  }

  function onMouseUp(e) {
    if (!isDragging) return;

    if (dragNode && !movedDuringDrag) {
      const dx = e.clientX - dragStartPos.x;
      const dy = e.clientY - dragStartPos.y;
      if (Math.abs(dx) < 3 && Math.abs(dy) < 3) {
        dispatch('navigate', dragNode.id);
      }
    }
    clearInteractionState();
  }

  function onClick(e) {
    if (suppressClick) {
      suppressClick = false;
      return;
    }
    const pos = getWorldPos(e.clientX, e.clientY);
    const node = findNodeAt(pos.x, pos.y);
    if (node) {
      dispatch('navigate', node.id);
    }
  }

  function onWheel(e) {
    e.preventDefault();
    const rect = canvas.getBoundingClientRect();
    const mx = e.clientX - rect.left;
    const my = e.clientY - rect.top;

    const zoomFactor = e.deltaY > 0 ? 0.9 : 1.1;
    const newK = Math.max(0.1, Math.min(5, transform.k * zoomFactor));

    transform = {
      ...transform,
      x: mx - (mx - transform.x) * (newK / transform.k),
      y: my - (my - transform.y) * (newK / transform.k),
      k: newK,
    };
  }

  function resetView() {
    if (nodes.length === 0) return;
    let minX = Infinity, maxX = -Infinity, minY = Infinity, maxY = -Infinity;
    for (const node of nodes) {
      minX = Math.min(minX, node.x);
      maxX = Math.max(maxX, node.x);
      minY = Math.min(minY, node.y);
      maxY = Math.max(maxY, node.y);
    }
    const rect = canvas.getBoundingClientRect();
    const graphW = maxX - minX + 100;
    const graphH = maxY - minY + 100;
    const scale = Math.min(rect.width / graphW, rect.height / graphH, 2);
    const k = Math.max(0.1, scale * 0.9);
    transform = {
      k,
      x: rect.width / 2 - ((minX + maxX) / 2) * k,
      y: rect.height / 2 - ((minY + maxY) / 2) * k,
    };
  }
</script>

<div class="graph-view" bind:this={container}>
  {#if loading}
    <div class="loading">Loading graph...</div>
  {:else if error}
    <div class="error">{error}</div>
  {:else if nodes.length === 0}
    <div class="empty">No notes found. Create some notes with [[WikiLinks]] to see the graph.</div>
  {:else}
    <canvas
      bind:this={canvas}
      onmousedown={onMouseDown}
      onmousemove={onMouseMove}
      onmouseup={onMouseUp}
      onclick={onClick}
      onmouseleave={() => {
        if (!isDragging) hoverNode = null;
      }}
      onwheel={onWheel}
    ></canvas>
    <div class="graph-controls">
      <button onclick={resetView} title="Fit to view">
        <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 3h6v6M9 21H3v-6M21 3l-7 7M3 21l7-7"/></svg>
      </button>
      <span class="graph-info">{nodes.length} notes · {links.length} links</span>
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

  canvas {
    width: 100%;
    height: 100%;
    display: block;
    cursor: grab;
    touch-action: none;
    user-select: none;
  }

  canvas:active {
    cursor: grabbing;
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

  .graph-info {
    font-size: 0.75rem;
    color: var(--color-faint);
    user-select: none;
  }
</style>
