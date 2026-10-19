<script>
  import { createEventDispatcher, onMount } from "svelte";
  import ForceGraph3D from "3d-force-graph";
  import * as THREE from "three";
  import { graph } from "../lib/api.js";
  import { subscribeToLiveUpdates } from "../lib/live.js";
  import { theme } from "../stores/theme.js";
  import { preprocessGraph, tooltipLabel } from "../lib/graph.js";

  const dispatch = createEventDispatcher();
  const colors = {
    Note: "#8b5cf6",
    Metric: "#60a5fa",
    Runbook: "#fb7185",
    API: "#4ade80",
    Table: "#fbbf24",
    Definition: "#2dd4bf",
    Reference: "#f472b6",
  };
  let container;
  let instance;
  let data = { nodes: [], links: [] };
  let sprites = new Map();
  let adjacency = new Map();
  let selected = null;
  let hovered = null;
  let type = "all";
  let showLabels = false;
  let generation = 0;
  let disposed = false;
  let refreshTimer;
  let fitTimer;
  let controller;

  function dark() {
    return document.documentElement.classList.contains("dark");
  }
  function disposeLabels() {
    for (const sprite of sprites.values()) {
      sprite.material.map?.dispose();
      sprite.material.dispose();
    }
    sprites.clear();
  }
  function related(id) {
    return (
      id === selected ||
      id === hovered ||
      adjacency.get(selected)?.has(id) ||
      adjacency.get(hovered)?.has(id)
    );
  }
  function nodeColor(node) {
    if (node.id === selected || node.id === hovered)
      return dark() ? "#e9d5ff" : "#6d28d9";
    if (selected && !related(node.id)) return dark() ? "#544365" : "#c9b8df";
    return colors[node.type] || colors.Note;
  }

  function spriteFor(node) {
    const canvas = document.createElement("canvas"),
      context = canvas.getContext("2d");
    if (!context) return null;
    const text = String(node.title || "").slice(0, 120);
    context.font = '500 64px "Inter Variable", system-ui, sans-serif';
    canvas.width = Math.min(
      4096,
      Math.ceil(context.measureText(text).width + 40),
    );
    canvas.height = 96;
    context.font = '500 64px "Inter Variable", system-ui, sans-serif';
    context.textBaseline = "middle";
    context.fillStyle = dark() ? "#e9d5ff" : "#4c1d95";
    context.fillText(text, 20, 48, canvas.width - 40);
    const texture = new THREE.CanvasTexture(canvas);
    texture.minFilter = THREE.LinearFilter;
    texture.generateMipmaps = false;
    const sprite = new THREE.Sprite(
      new THREE.SpriteMaterial({
        map: texture,
        transparent: true,
        depthTest: false,
        depthWrite: false,
      }),
    );
    sprite.scale.set((8 * canvas.width) / canvas.height, 8, 1);
    sprite.position.set(0, 8, 0);
    sprite.renderOrder = 999;
    sprite.visible = showLabels || related(node.id);
    sprites.set(node.id, sprite);
    return sprite;
  }

  function refresh() {
    if (!instance) return;
    instance.nodeColor((node) => nodeColor(node));
    instance.linkColor((link) =>
      related(link.sourceId) && related(link.targetId)
        ? dark()
          ? "#c4b5fd"
          : "#8b5cf6"
        : dark()
          ? "#41414d"
          : "#d1cbdc",
    );
    for (const [id, sprite] of sprites)
      sprite.visible = showLabels || related(id);
  }

  function select(node) {
    selected = node?.id || null;
    dispatch("select", node || null);
    refresh();
  }
  export function selectNode(id) {
    select(data.nodes.find((node) => node.id === id));
  }
  export function clearSelection() {
    select(null);
  }
  export function fit() {
    instance?.zoomToFit(
      window.matchMedia("(prefers-reduced-motion: reduce)").matches ? 0 : 350,
      45,
    );
  }
  export function labels() {
    showLabels = !showLabels;
    try {
      localStorage.setItem("graphShowAllLabels", String(showLabels));
    } catch {}
    refresh();
    return showLabels;
  }
  export function filterByType(value) {
    type = value;
    const ids = new Set(
      data.nodes
        .filter((node) => type === "all" || node.type === type)
        .map((node) => node.id),
    );
    instance?.nodeVisibility((node) => ids.has(node.id));
    instance?.linkVisibility(
      (link) => ids.has(link.sourceId) && ids.has(link.targetId),
    );
  }
  export function retry() {
    load(false);
  }

  async function load(live) {
    const id = ++generation;
    controller?.abort();
    controller = new AbortController();
    try {
      const response = await graph.get(controller.signal);
      if (disposed || id !== generation) return;
      if (!Array.isArray(response?.nodes))
        throw new Error("Invalid graph data");
      const processed = preprocessGraph(response);
      const previous = new Map(data.nodes.map((node) => [node.id, node]));
      if (live)
        for (const node of processed.nodes) {
          const before = previous.get(node.id);
          if (before)
            for (const key of [
              "x",
              "y",
              "z",
              "vx",
              "vy",
              "vz",
              "fx",
              "fy",
              "fz",
            ])
              if (before[key] !== undefined) node[key] = before[key];
        }
      data = processed;
      adjacency = new Map(data.nodes.map((node) => [node.id, new Set()]));
      for (const link of data.links) {
        adjacency.get(link.sourceId)?.add(link.targetId);
        adjacency.get(link.targetId)?.add(link.sourceId);
      }
      if (!adjacency.has(selected)) selected = null;
      if (instance) {
        disposeLabels();
        instance.graphData(data);
        instance.nodeThreeObject((node) => spriteFor(node));
        filterByType(type);
        refresh();
        if (!live) {
          clearTimeout(fitTimer);
          fitTimer = setTimeout(fit, 200);
        }
      }
      dispatch("data", { nodes: data.nodes, links: data.links });
    } catch (error) {
      if (!disposed && id === generation && error.name !== "AbortError")
        dispatch(
          "loaderror",
          "The graph could not load. Check your connection and try again.",
        );
    }
  }

  onMount(() => {
    try {
      showLabels = localStorage.getItem("graphShowAllLabels") === "true";
    } catch {}
    dispatch("labels", showLabels);
    try {
      instance = new ForceGraph3D(container)
        .nodeId("id")
        .linkSource("sourceId")
        .linkTarget("targetId")
        .nodeVal("val")
        .nodeRelSize(6)
        .nodeResolution(20)
        .nodeLabel((node) => tooltipLabel(node))
        .nodeThreeObject((node) => spriteFor(node))
        .nodeThreeObjectExtend(true)
        .showNavInfo(false)
        .enableNodeDrag(true)
        .warmupTicks(30)
        .d3VelocityDecay(0.3)
        .d3AlphaDecay(0.015)
        .onNodeClick(select)
        .onNodeHover((node) => {
          hovered = node?.id || null;
          refresh();
        })
        .onBackgroundClick(() => select(null));
      instance.d3Force("charge").strength(-80);
    } catch {
      dispatch(
        "canvaserror",
        "The 3D canvas is unavailable. Browse the note list to explore your connections.",
      );
    }
    const observer = new ResizeObserver(() => {
      if (instance)
        instance.width(container.clientWidth).height(container.clientHeight);
    });
    observer.observe(container);
    const unsubscribeTheme = theme.subscribe(() => {
      if (instance) {
        instance.backgroundColor(dark() ? "#0a0a0c" : "#fafafa");
        disposeLabels();
        instance.nodeThreeObject((node) => spriteFor(node));
        refresh();
      }
    });
    const unsubscribeLive = subscribeToLiveUpdates((event) => {
      if (event.graph) {
        clearTimeout(refreshTimer);
        refreshTimer = setTimeout(() => load(true), 250);
      }
    });
    load(false);
    return () => {
      disposed = true;
      generation++;
      controller?.abort();
      clearTimeout(refreshTimer);
      clearTimeout(fitTimer);
      observer.disconnect();
      unsubscribeTheme();
      unsubscribeLive();
      disposeLabels();
      instance?._destructor?.();
      instance = null;
    };
  });
</script>

<div
  class="graph-canvas"
  bind:this={container}
  role="img"
  aria-label="Three-dimensional note connections. Use the graph note search for keyboard navigation."
></div>

<style>
  .graph-canvas {
    position: absolute;
    inset: 0;
    overflow: hidden;
  }
  .graph-canvas :global(canvas) {
    display: block;
  }
</style>
