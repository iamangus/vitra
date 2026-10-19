<script>
  export let name = "note";
  export let size = 18;
  const paths = {
    note: "M14 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z M14 3v6h6 M8 13h8 M8 17h5",
    folder:
      "M3 7a2 2 0 0 1 2-2h5l2 3h7a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z",
    search: "M21 21l-5-5 M18 10a7 7 0 1 1-14 0 7 7 0 0 1 14 0",
    menu: "M4 6h16 M4 12h16 M4 18h16",
    close: "M6 6l12 12 M18 6L6 18",
    plus: "M12 5v14 M5 12h14",
    more: "M5 12h.01 M12 12h.01 M19 12h.01",
    chevron: "M9 5l7 7-7 7",
    graph:
      "M8 11l8-5 M8 13l8 5 M18 8v8 M8 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0 M21 5a3 3 0 1 1-6 0 3 3 0 0 1 6 0 M21 19a3 3 0 1 1-6 0 3 3 0 0 1 6 0",
    activity: "M3 12h4l3-8 4 16 3-8h4",
    sun: "M12 2v2 M12 20v2 M2 12h2 M20 12h2 M5 5l1.5 1.5 M17.5 17.5L19 19 M5 19l1.5-1.5 M17.5 6.5L19 5 M17 12a5 5 0 1 1-10 0 5 5 0 0 1 10 0",
    arrow: "M5 12h14 M13 6l6 6-6 6",
    back: "M19 12H5 M11 6l-6 6 6 6",
    check: "M5 12l4 4L19 6",
    warning:
      "M12 9v4 M12 17h.01 M10 4L2 18a2 2 0 0 0 2 3h16a2 2 0 0 0 2-3L14 4a2 2 0 0 0-4 0",
    link: "M10 13a5 5 0 0 0 7 0l3-3a5 5 0 0 0-7-7l-2 2 M14 11a5 5 0 0 0-7 0l-3 3a5 5 0 0 0 7 7l2-2",
    outline: "M9 5h12 M9 12h12 M9 19h12 M3 5h.01 M3 12h.01 M3 19h.01",
    edit: "M15 4l5 5 M4 20l5-1L21 7a2 2 0 0 0-5-5L4 14z",
    move: "M12 3v18 M3 12h18 M8 7l4-4 4 4 M8 17l4 4 4-4 M7 8l-4 4 4 4 M17 8l4 4-4 4",
    download: "M12 3v12 M7 10l5 5 5-5 M4 17v4h16v-4",
    trash: "M3 6h18 M9 6V3h6v3 M5 6l1 15h12l1-15 M10 10v7 M14 10v7",
    clock: "M12 7v5l3 2 M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0",
    fit: "M3 9V3h6 M15 3h6v6 M21 15v6h-6 M9 21H3v-6",
    copy: "M9 9h12v12H9z M15 5V3H3v12h2",
  };
</script>

<svg
  width={size}
  height={size}
  viewBox="0 0 24 24"
  fill="none"
  stroke="currentColor"
  stroke-width="1.65"
  stroke-linecap="round"
  stroke-linejoin="round"
  aria-hidden="true"><path d={paths[name] || paths.note} /></svg
>
