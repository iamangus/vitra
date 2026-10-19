export function focusScope(node) {
  const previous = document.activeElement;
  const controls = () =>
    [
      ...node.querySelectorAll(
        'button, input, select, textarea, a[href], summary, [tabindex="0"]',
      ),
    ].filter((el) => !el.disabled && el.checkVisibility());
  (controls()[0] || node).focus();
  function handle(event) {
    if (event.key !== "Tab") return;
    const items = controls();
    if (!items.length) {
      event.preventDefault();
      node.focus();
      return;
    }
    const first = items[0],
      last = items.at(-1);
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  }
  node.addEventListener("keydown", handle);
  return {
    destroy() {
      node.removeEventListener("keydown", handle);
      if (previous instanceof HTMLElement && previous.isConnected)
        previous.focus();
    },
  };
}
