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
  let groupAnchorByKey = new Map();
  let loading = $state(true);
  let error = $state(null);

  // Viewport + interaction
  let transform = $state({ x: 0, y: 0, k: 1 });
  let isDragging = $state(false);
  let dragNode = $state(null);
  let hoverNode = $state(null);
  let selectedNodeId = $state(null);
  let mousePos = $state({ x: 0, y: 0 });
  let dragStartPos = $state({ x: 0, y: 0 });
  let movedDuringDrag = $state(false);
  let suppressClick = $state(false);

  // Adaptive simulation config
  let simConfig = $state({
    repulsion: 5000,
    springLength: 250,
    springStrength: 0.15,
    damping: 0.92,
    centerForce: 0.01,
    maxSpeed: 4,
    preTicks: 300,
    fitPadding: 120,
    labelZoomThreshold: 0.95,
    detailZoomThreshold: 1.45,
    backgroundLabelLimit: 12,
    idleLabelDegreeThreshold: 4,
    edgeOpacity: 0.28,
    collisionStrength: 0.18,
    hubSpacingScale: 3,
    groupForce: 0.004,
    groupOrbitRadius: 280,
    crossGroupSpringScale: 1.45,
    crossGroupSpringStrengthScale: 0.72,
  });

  let animationId = $state(null);
  let resizeObserver = $state(null);
  let simRunning = $state(false);

  $effect(() => {
    dpr = window.devicePixelRatio || 1;
    loadGraph();

    return () => {
      if (animationId) cancelAnimationFrame(animationId);
      if (resizeObserver) resizeObserver.disconnect();
    };
  });

  $effect(() => {
    if (!canvas || !container) return;

    ctx = canvas.getContext('2d');
    if (!ctx) return;

    resizeCanvas();

    resizeObserver = new ResizeObserver(() => {
      resizeCanvas();
      if (nodes.length > 0) {
        fitGraphToViewport(0.86);
      }
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

  $effect(() => {
    if (nodes.length > 0 && ctx && !simRunning && canvas) {
      simRunning = true;
      startSimulation();
    }
  });

  function clamp(value, min, max) {
    return Math.max(min, Math.min(max, value));
  }

  function hashString(value) {
    let hash = 0;
    for (let i = 0; i < value.length; i++) {
      hash = (hash * 31 + value.charCodeAt(i)) >>> 0;
    }
    return hash;
  }

  function deriveGroupKey(nodeId) {
    const parts = nodeId.split('/').filter(Boolean);
    if (parts.length <= 1) return nodeId;

    if (parts[0] === 'k8s-cluster' && parts[1] === 'Layers' && parts.length >= 3) {
      return parts.slice(0, 3).join('/');
    }

    if (parts[0] === 'k8s-cluster') {
      return parts.slice(0, Math.min(2, parts.length)).join('/');
    }

    return parts.slice(0, Math.min(2, parts.length)).join('/');
  }

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

  function buildSimConfig(nodeCount, linkCount) {
    const safeNodeCount = Math.max(1, nodeCount);
    const avgDegree = nodeCount > 0 ? (2 * linkCount) / nodeCount : 0;
    const maxPossibleLinks = nodeCount > 1 ? (nodeCount * (nodeCount - 1)) / 2 : 1;
    const density = linkCount / Math.max(1, maxPossibleLinks);

    return {
      repulsion: 4200 + safeNodeCount * 180 + density * 18000,
      springLength: 180 + Math.sqrt(safeNodeCount) * 18 + avgDegree * 12,
      springStrength: clamp(0.09 - density * 0.08, 0.015, 0.08),
      damping: clamp(0.91 - density * 0.05, 0.84, 0.92),
      centerForce: clamp(0.006 - safeNodeCount * 0.00003 - density * 0.002, 0.0012, 0.0045),
      maxSpeed: clamp(3 + Math.sqrt(safeNodeCount) * 0.18, 3, 7),
      preTicks: Math.round(clamp(220 + safeNodeCount * 7 + linkCount * 0.7, 220, 1200)),
      fitPadding: Math.round(135 + Math.sqrt(safeNodeCount) * 20 + avgDegree * 8),
      labelZoomThreshold: safeNodeCount > 60 ? 1.1 : 0.9,
      detailZoomThreshold: safeNodeCount > 60 ? 1.55 : 1.25,
      backgroundLabelLimit: Math.round(clamp(Math.sqrt(safeNodeCount) * 1.5, 8, 20)),
      idleLabelDegreeThreshold: Math.max(4, Math.ceil(avgDegree)),
      edgeOpacity: clamp(0.28 - density * 0.1, 0.1, 0.28),
      collisionStrength: clamp(0.16 + density * 0.35 + safeNodeCount * 0.0006, 0.16, 0.28),
      hubSpacingScale: clamp(2.5 + density * 5 + avgDegree * 0.08, 2.5, 5.5),
      groupForce: clamp(0.003 + density * 0.024 + avgDegree * 0.00018, 0.003, 0.011),
      groupOrbitRadius: Math.round(290 + Math.sqrt(safeNodeCount) * 38 + avgDegree * 11),
      crossGroupSpringScale: clamp(1.35 + density * 1.5, 1.35, 1.9),
      crossGroupSpringStrengthScale: clamp(0.7 - density * 0.3, 0.45, 0.7),
    };
  }

  function buildGroupAnchors(newNodes, nextConfig) {
    const groups = new Map();
    for (const node of newNodes) {
      if (!groups.has(node.groupKey)) {
        groups.set(node.groupKey, []);
      }
      groups.get(node.groupKey).push(node);
    }

    const orderedGroups = [...groups.entries()].sort((a, b) => b[1].length - a[1].length || a[0].localeCompare(b[0]));
    const anchors = new Map();

    orderedGroups.forEach(([groupKey], index) => {
      const angle = orderedGroups.length === 1 ? 0 : (index / orderedGroups.length) * Math.PI * 2;
      const radiusJitter = orderedGroups.length <= 2 ? 0 : (index % 3) * nextConfig.springLength * 0.18;
      const orbitRadius = orderedGroups.length === 1 ? 0 : nextConfig.groupOrbitRadius + radiusJitter;
      anchors.set(groupKey, {
        x: Math.cos(angle) * orbitRadius,
        y: Math.sin(angle) * orbitRadius,
      });
    });

    return anchors;
  }

  function seedNodePositions(newNodes, nextConfig, anchors) {
    const groups = new Map();
    for (const node of newNodes) {
      if (!groups.has(node.groupKey)) {
        groups.set(node.groupKey, []);
      }
      groups.get(node.groupKey).push(node);
    }

    const orderedGroups = [...groups.entries()].sort((a, b) => b[1].length - a[1].length || a[0].localeCompare(b[0]));
    for (const [groupKey, groupNodes] of orderedGroups) {
      const anchor = anchors.get(groupKey) || { x: 0, y: 0 };
      const orderedNodes = [...groupNodes].sort((a, b) => b.links - a.links || a.id.localeCompare(b.id));
      const baseSpacing = 42 + Math.sqrt(groupNodes.length) * 13 + nextConfig.springLength * 0.06;
      let ring = 0;
      let indexInRing = 0;
      let ringCapacity = Math.max(1, Math.ceil(Math.sqrt(groupNodes.length) * 1.8));

      for (const node of orderedNodes) {
        if (indexInRing >= ringCapacity) {
          ring += 1;
          indexInRing = 0;
          ringCapacity = Math.max(6, Math.round(Math.sqrt(groupNodes.length) * (ring + 1) * 1.5));
        }

        const angle = ringCapacity === 1
          ? 0
          : (indexInRing / ringCapacity) * Math.PI * 2 + ring * 0.35 + (hashString(groupKey) % 7) * 0.09;
        const ringRadius = ring * baseSpacing;
        const jitterSeed = hashString(node.id);
        const jitter = Math.min(18, 6 + ring * 2);
        const jitterX = ((((jitterSeed & 1023) / 1023) * 2) - 1) * jitter;
        const jitterY = (((((jitterSeed >> 10) & 1023) / 1023) * 2) - 1) * jitter;

        node.x = anchor.x + Math.cos(angle) * ringRadius + jitterX;
        node.y = anchor.y + Math.sin(angle) * ringRadius + jitterY;
        node.vx = 0;
        node.vy = 0;

        indexInRing += 1;
      }
    }
  }

  function initGraph(data) {
    const nextNodeIndexById = new Map();
    const newNodes = data.nodes.map((n) => ({
      id: n.id,
      title: n.title,
      groupKey: deriveGroupKey(n.id),
      x: 0,
      y: 0,
      vx: 0,
      vy: 0,
      radius: 4,
      links: 0,
    }));

    for (const node of newNodes) {
      nextNodeIndexById.set(node.id, nextNodeIndexById.size);
    }

    const newLinks = data.links.map((link) => ({
      sourceId: link.source,
      targetId: link.target,
    })).filter((link) => nextNodeIndexById.has(link.sourceId) && nextNodeIndexById.has(link.targetId));

    for (const link of newLinks) {
      const source = newNodes[nextNodeIndexById.get(link.sourceId)];
      const target = newNodes[nextNodeIndexById.get(link.targetId)];
      if (source) source.links += 1;
      if (target) target.links += 1;
    }

    const maxLinks = Math.max(1, ...newNodes.map((node) => node.links));
    for (const node of newNodes) {
      const linkRatio = node.links / maxLinks;
      // Use a steeper curve so hubs read much larger in dense graphs.
      node.radius = 2.5 + Math.pow(linkRatio, 1.7) * 14;
    }

    const nextConfig = buildSimConfig(newNodes.length, newLinks.length);
    const nextGroupAnchors = buildGroupAnchors(newNodes, nextConfig);
    seedNodePositions(newNodes, nextConfig, nextGroupAnchors);

    nodes = newNodes;
    links = newLinks;
    nodeIndexById = nextNodeIndexById;
    groupAnchorByKey = nextGroupAnchors;
    simConfig = nextConfig;
    selectedNodeId = null;
    hoverNode = null;
  }

  function startSimulation() {
    for (let i = 0; i < simConfig.preTicks; i++) {
      simulate(1);
    }

    fitGraphToViewport(0.86);

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

  function getBounds(padding = simConfig.fitPadding) {
    let minX = Infinity;
    let maxX = -Infinity;
    let minY = Infinity;
    let maxY = -Infinity;

    for (const node of nodes) {
      minX = Math.min(minX, node.x - node.radius);
      maxX = Math.max(maxX, node.x + node.radius);
      minY = Math.min(minY, node.y - node.radius);
      maxY = Math.max(maxY, node.y + node.radius);
    }

    return {
      minX: minX - padding,
      maxX: maxX + padding,
      minY: minY - padding,
      maxY: maxY + padding,
    };
  }

  function fitGraphToViewport(fill = 0.9) {
    if (nodes.length === 0 || !canvas) return;

    const bounds = getBounds();
    const rect = canvas.getBoundingClientRect();
    const graphW = Math.max(1, bounds.maxX - bounds.minX);
    const graphH = Math.max(1, bounds.maxY - bounds.minY);
    const scale = Math.min(rect.width / graphW, rect.height / graphH, 2);
    const k = Math.max(0.08, scale * fill);
    const centerX = (bounds.minX + bounds.maxX) / 2;
    const centerY = (bounds.minY + bounds.maxY) / 2;

    transform = {
      k,
      x: rect.width / 2 - centerX * k,
      y: rect.height / 2 - centerY * k,
    };
  }

  function simulate(dt) {
    const cx = 0;
    const cy = 0;

    for (let i = 0; i < nodes.length; i++) {
      for (let j = i + 1; j < nodes.length; j++) {
        const a = nodes[i];
        const b = nodes[j];
        let dx = a.x - b.x;
        let dy = a.y - b.y;
        let dist = Math.sqrt(dx * dx + dy * dy) || 1;
        const sameGroup = a.groupKey === b.groupKey;
        const repulsionMultiplier = sameGroup ? 0.82 : 1.35;
        const force = (simConfig.repulsion * repulsionMultiplier * dt) / (dist * dist);
        const fx = (dx / dist) * force;
        const fy = (dy / dist) * force;
        a.vx += fx;
        a.vy += fy;
        b.vx -= fx;
        b.vy -= fy;

        const minDistance = a.radius + b.radius + 18
          + Math.sqrt(a.links) * simConfig.hubSpacingScale
          + Math.sqrt(b.links) * simConfig.hubSpacingScale
          + (sameGroup ? 0 : 24);
        if (dist < minDistance) {
          // Add a direct separation force so hubs keep enough room for their edges to fan out.
          const overlap = minDistance - dist;
          const collisionForce = overlap * simConfig.collisionStrength * dt;
          const cfx = (dx / dist) * collisionForce;
          const cfy = (dy / dist) * collisionForce;
          a.vx += cfx;
          a.vy += cfy;
          b.vx -= cfx;
          b.vy -= cfy;
        }
      }
    }

    for (const link of links) {
      const a = nodes[nodeIndexById.get(link.sourceId)];
      const b = nodes[nodeIndexById.get(link.targetId)];
      if (!a || !b) continue;
      const sameGroup = a.groupKey === b.groupKey;
      let dx = b.x - a.x;
      let dy = b.y - a.y;
      let dist = Math.sqrt(dx * dx + dy * dy) || 1;
      const targetLength = simConfig.springLength * (sameGroup ? 1 : simConfig.crossGroupSpringScale);
      const springStrength = simConfig.springStrength * (sameGroup ? 1 : simConfig.crossGroupSpringStrengthScale);
      const force = (dist - targetLength) * springStrength * dt;
      const fx = (dx / dist) * force;
      const fy = (dy / dist) * force;
      a.vx += fx;
      a.vy += fy;
      b.vx -= fx;
      b.vy -= fy;
    }

    for (const node of nodes) {
      const anchor = groupAnchorByKey.get(node.groupKey);
      if (anchor) {
        node.vx += (anchor.x - node.x) * simConfig.groupForce * dt;
        node.vy += (anchor.y - node.y) * simConfig.groupForce * dt;
      }

      node.vx += (cx - node.x) * simConfig.centerForce * dt;
      node.vy += (cy - node.y) * simConfig.centerForce * dt;

      node.vx *= simConfig.damping;
      node.vy *= simConfig.damping;

      const speed = Math.sqrt(node.vx * node.vx + node.vy * node.vy);
      if (speed > simConfig.maxSpeed) {
        node.vx = (node.vx / speed) * simConfig.maxSpeed;
        node.vy = (node.vy / speed) * simConfig.maxSpeed;
      }

      node.x += node.vx * dt;
      node.y += node.vy * dt;
    }
  }

  function getConnectedIds(focusId) {
    const connectedIds = new Set();
    if (!focusId) return connectedIds;

    connectedIds.add(focusId);
    for (const link of links) {
      if (link.sourceId === focusId) {
        connectedIds.add(link.targetId);
      } else if (link.targetId === focusId) {
        connectedIds.add(link.sourceId);
      }
    }
    return connectedIds;
  }

  function getVisibleWorldRect(width, height) {
    return {
      minX: (-transform.x) / transform.k,
      maxX: (width - transform.x) / transform.k,
      minY: (-transform.y) / transform.k,
      maxY: (height - transform.y) / transform.k,
    };
  }

  function isNodeVisible(node, rect) {
    return node.x + node.radius >= rect.minX
      && node.x - node.radius <= rect.maxX
      && node.y + node.radius >= rect.minY
      && node.y - node.radius <= rect.maxY;
  }

  function rectsOverlap(a, b) {
    return a.x < b.x + b.w
      && a.x + a.w > b.x
      && a.y < b.y + b.h
      && a.y + a.h > b.y;
  }

  function render() {
    if (!ctx || !canvas) return;

    const rect = canvas.getBoundingClientRect();
    const width = rect.width;
    const height = rect.height;
    const isDark = document.documentElement.classList.contains('dark');

    ctx.fillStyle = isDark ? '#0a0a0c' : '#fafafa';
    ctx.fillRect(0, 0, width, height);

    ctx.save();
    ctx.translate(transform.x, transform.y);
    ctx.scale(transform.k, transform.k);

    const selectedConnectedIds = getConnectedIds(selectedNodeId);
    const hoverConnectedIds = getConnectedIds(hoverNode?.id || '');
    const hasSelection = selectedConnectedIds.size > 0;
    const visibleWorld = getVisibleWorldRect(width, height);
    const baseLinkAlpha = simConfig.edgeOpacity;

    const defaultLinkColor = isDark
      ? `rgba(139, 139, 152, ${baseLinkAlpha})`
      : `rgba(161, 161, 171, ${baseLinkAlpha})`;
    const crossGroupLinkColor = isDark
      ? `rgba(139, 139, 152, ${baseLinkAlpha * 0.55})`
      : `rgba(161, 161, 171, ${baseLinkAlpha * 0.55})`;
    const dimLinkColor = isDark ? 'rgba(139, 139, 152, 0.05)' : 'rgba(161, 161, 171, 0.08)';
    const nodeColor = isDark ? '#a855f7' : '#7c3aed';
    const hoverColor = isDark ? '#c084fc' : '#6d28d9';
    const dimNodeColor = isDark ? 'rgba(168, 85, 247, 0.2)' : 'rgba(124, 58, 237, 0.2)';
    const textColor = isDark ? 'rgba(240, 240, 245, 0.92)' : 'rgba(26, 26, 30, 0.92)';
    const mutedTextColor = isDark ? 'rgba(240, 240, 245, 0.78)' : 'rgba(26, 26, 30, 0.78)';

    for (const link of links) {
      const source = nodes[nodeIndexById.get(link.sourceId)];
      const target = nodes[nodeIndexById.get(link.targetId)];
      if (!source || !target) continue;
      const sameGroup = source.groupKey === target.groupKey;

      const touchesSelection = selectedNodeId && (link.sourceId === selectedNodeId || link.targetId === selectedNodeId);
      const touchesHover = hoverNode && (link.sourceId === hoverNode.id || link.targetId === hoverNode.id);

      ctx.beginPath();
      ctx.moveTo(source.x, source.y);
      ctx.lineTo(target.x, target.y);
      ctx.strokeStyle = touchesHover
        ? (isDark ? 'rgba(192, 132, 252, 0.78)' : 'rgba(109, 40, 217, 0.75)')
        : (touchesSelection
          ? (isDark ? 'rgba(168, 85, 247, 0.68)' : 'rgba(124, 58, 237, 0.68)')
          : (hasSelection ? dimLinkColor : (sameGroup ? defaultLinkColor : crossGroupLinkColor)));
      ctx.lineWidth = touchesHover ? 1.8 : (touchesSelection ? 1.5 : 1);
      ctx.stroke();
    }

    for (const node of nodes) {
      const isSelected = node.id === selectedNodeId;
      const isHover = node.id === hoverNode?.id;
      const linkedToSelection = selectedConnectedIds.has(node.id);
      const linkedToHover = hoverConnectedIds.has(node.id);
      const dimmed = hasSelection && !linkedToSelection;

      ctx.beginPath();
      ctx.arc(node.x, node.y, node.radius, 0, Math.PI * 2);
      ctx.fillStyle = isHover
        ? hoverColor
        : (isSelected || linkedToSelection || linkedToHover ? nodeColor : (dimmed ? dimNodeColor : (isDark ? 'rgba(168, 85, 247, 0.82)' : 'rgba(124, 58, 237, 0.82)')));
      ctx.fill();

      if (isSelected || isHover || linkedToSelection) {
        ctx.beginPath();
        ctx.arc(node.x, node.y, node.radius + 3, 0, Math.PI * 2);
        ctx.strokeStyle = isHover
          ? (isDark ? 'rgba(192, 132, 252, 0.65)' : 'rgba(109, 40, 217, 0.65)')
          : (isDark ? 'rgba(168, 85, 247, 0.55)' : 'rgba(124, 58, 237, 0.55)');
        ctx.lineWidth = isSelected ? 1.6 : 1;
        ctx.stroke();
      }
    }

    ctx.font = `${12 / transform.k}px -apple-system, BlinkMacSystemFont, sans-serif`;
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';

    const primaryCandidates = [];
    const backgroundCandidates = [];

    for (const node of nodes) {
      if (!isNodeVisible(node, visibleWorld)) continue;

      const isSelected = node.id === selectedNodeId;
      const isHover = node.id === hoverNode?.id;
      const linkedToSelection = selectedConnectedIds.has(node.id) && !isSelected;
      const linkedToHover = hoverConnectedIds.has(node.id) && !isHover;

      if (isHover) {
        primaryCandidates.push({ node, priority: 1000, force: true, textColor, background: true });
        continue;
      }

      if (isSelected) {
        primaryCandidates.push({ node, priority: 900, force: true, textColor, background: true });
        continue;
      }

      if (transform.k >= simConfig.labelZoomThreshold && linkedToSelection) {
        primaryCandidates.push({ node, priority: 700 + node.links, force: false, textColor, background: true });
        continue;
      }

      if (transform.k >= simConfig.labelZoomThreshold && linkedToHover) {
        primaryCandidates.push({ node, priority: 650 + node.links, force: false, textColor, background: true });
        continue;
      }

      if (transform.k >= simConfig.detailZoomThreshold && node.links >= simConfig.idleLabelDegreeThreshold) {
        backgroundCandidates.push({ node, priority: 400 + node.links, force: false, textColor: mutedTextColor, background: true });
      }
    }

    backgroundCandidates.sort((a, b) => b.priority - a.priority);
    const labelCandidates = [...primaryCandidates, ...backgroundCandidates.slice(0, simConfig.backgroundLabelLimit)]
      .sort((a, b) => b.priority - a.priority);

    const acceptedRects = [];
    const padding = 4 / transform.k;
    const textHeight = 14 / transform.k;

    for (const candidate of labelCandidates) {
      const textY = candidate.node.y - candidate.node.radius - 12 / transform.k;
      const textWidth = ctx.measureText(candidate.node.title).width;
      const labelRect = {
        x: candidate.node.x - textWidth / 2 - padding,
        y: textY - textHeight / 2 - padding,
        w: textWidth + padding * 2,
        h: textHeight + padding * 2,
      };

      if (!candidate.force && acceptedRects.some((rect) => rectsOverlap(rect, labelRect))) {
        continue;
      }

      acceptedRects.push(labelRect);
      ctx.fillStyle = isDark ? 'rgba(10, 10, 12, 0.86)' : 'rgba(250, 250, 250, 0.9)';
      ctx.beginPath();
      ctx.roundRect(labelRect.x, labelRect.y, labelRect.w, labelRect.h, 3 / transform.k);
      ctx.fill();

      ctx.fillStyle = candidate.textColor;
      ctx.fillText(candidate.node.title, candidate.node.x, textY);
    }

    ctx.restore();
  }

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
      dragNode = null;
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
        if (Math.abs(e.clientX - dragStartPos.x) > 2 || Math.abs(e.clientY - dragStartPos.y) > 2) {
          movedDuringDrag = true;
          suppressClick = true;
        }
        transform = { ...transform, x: transform.x + dx, y: transform.y + dy };
      }
    } else {
      const pos = getWorldPos(e.clientX, e.clientY);
      hoverNode = findNodeAt(pos.x, pos.y);
      if (canvas) canvas.style.cursor = hoverNode ? 'pointer' : 'grab';
    }
  }

  function onMouseUp() {
    if (!isDragging) return;
    clearInteractionState();
  }

  function onClick(e) {
    if (suppressClick) {
      suppressClick = false;
      return;
    }

    const pos = getWorldPos(e.clientX, e.clientY);
    const node = findNodeAt(pos.x, pos.y);

    if (!node) {
      selectedNodeId = null;
      return;
    }

    if (selectedNodeId === node.id) {
      dispatch('navigate', node.id);
      return;
    }

    selectedNodeId = node.id;
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
    fitGraphToViewport(0.9);
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

  .graph-info {
    font-size: 0.75rem;
    color: var(--color-faint);
    user-select: none;
  }

  .graph-info.emphasis {
    color: var(--color-muted);
  }
</style>
