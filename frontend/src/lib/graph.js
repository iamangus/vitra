export function preprocessGraph(data) {
  const nodes = data.nodes || [];
  const links = data.links || [];
  const validIds = new Set(nodes.map(node => node.id));
  const degrees = new Map();
  const validLinks = links.filter(link => validIds.has(link.source) && validIds.has(link.target));
  for (const link of validLinks) {
    degrees.set(link.source, (degrees.get(link.source) || 0) + 1);
    degrees.set(link.target, (degrees.get(link.target) || 0) + 1);
  }
  let maxLinks = 1;
  for (const degree of degrees.values()) maxLinks = Math.max(maxLinks, degree);
  return {
    nodes: nodes.map(node => ({ id: node.id, title: node.title, type: node.type || 'Note', val: 1 + Math.pow((degrees.get(node.id) || 0) / maxLinks, 1.7) * 6 })),
    links: validLinks.map(link => ({ sourceId: link.source, targetId: link.target })),
  };
}

export function tooltipLabel(node) {
  return String(node.title || '').replace(/[&<>"']/g, character => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[character]);
}
