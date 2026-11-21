// Line icons on a 16-unit grid, in the spirit of SF Symbols.
const svg = (body: string, size = 16) =>
  `<svg class="icon" width="${size}" height="${size}" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${body}</svg>`;

const filled = (d: string, size = 16) =>
  `<svg class="icon" width="${size}" height="${size}" viewBox="0 0 16 16" aria-hidden="true"><path fill="currentColor" d="${d}"/></svg>`;

const HEART = "M8 13.9S1.6 10.3 1.6 5.9A3.4 3.4 0 0 1 8 4a3.4 3.4 0 0 1 6.4 1.9c0 4.4-6.4 8-6.4 8z";
const STAR = "M8 1.7l1.95 3.95 4.35.63-3.15 3.07.74 4.33L8 11.63l-3.89 2.05.74-4.33L1.7 6.28l4.35-.63z";

export const icons = {
  photos: svg(`<rect x="1.75" y="3.25" width="12.5" height="9.5" rx="2"/><path d="M1.75 10.5l3.2-3 2.6 2.4 2-1.7 4.7 3.6"/><circle cx="10.6" cy="6.2" r="1"/>`),
  clock: svg(`<circle cx="8" cy="8" r="6.25"/><path d="M8 4.6V8l2.3 1.5"/>`),
  heart: svg(`<path d="M8 13.4S2.1 10 2.1 5.9A2.95 2.95 0 0 1 8 4.4a2.95 2.95 0 0 1 5.9 1.5C13.9 10 8 13.4 8 13.4z"/>`),
  heartFill: filled(HEART),
  heartSmall: filled(HEART, 15),
  tag: svg(`<path d="M2.3 2.3h5.2l6.2 6.2-5.2 5.2-6.2-6.2z"/><circle cx="5.2" cy="5.2" r=".9"/>`),
  folder: svg(`<path d="M1.9 4.6c0-.8.6-1.4 1.4-1.4h3l1.4 1.5h5c.8 0 1.4.6 1.4 1.4v5.8c0 .8-.6 1.4-1.4 1.4H3.3c-.8 0-1.4-.6-1.4-1.4z"/>`),
  folderPlus: svg(`<path d="M1.9 4.6c0-.8.6-1.4 1.4-1.4h3l1.4 1.5h5c.8 0 1.4.6 1.4 1.4v5.8c0 .8-.6 1.4-1.4 1.4H3.3c-.8 0-1.4-.6-1.4-1.4z"/><path d="M8 7.3v3.6M6.2 9.1h3.6"/>`),
  folderAlert: svg(`<path d="M1.9 4.6c0-.8.6-1.4 1.4-1.4h3l1.4 1.5h5c.8 0 1.4.6 1.4 1.4v5.8c0 .8-.6 1.4-1.4 1.4H3.3c-.8 0-1.4-.6-1.4-1.4z"/><path d="M8 7v2.2M8 10.8v.1"/>`),
  chevron: svg(`<path d="M6 4l4 4-4 4"/>`, 12),
  chevronDown: svg(`<path d="M4 6l4 4 4-4"/>`, 10),
  chevronUp: svg(`<path d="M4 10l4-4 4 4"/>`, 10),
  chevronLeft: svg(`<path d="M10 3.5L5.5 8l4.5 4.5"/>`, 20),
  chevronRight: svg(`<path d="M6 3.5L10.5 8 6 12.5"/>`, 20),
  plus: svg(`<path d="M8 3.5v9M3.5 8h9"/>`, 14),
  minus: svg(`<path d="M3.5 8h9"/>`, 15),
  plusLg: svg(`<path d="M8 3.5v9M3.5 8h9"/>`, 15),
  search: svg(`<circle cx="7" cy="7" r="4.4"/><path d="M10.3 10.3l3.2 3.2"/>`),
  justified: svg(`<rect x="1.75" y="2.5" width="7" height="5" rx="1.2"/><rect x="10.25" y="2.5" width="4" height="5" rx="1.2"/><rect x="1.75" y="9" width="4" height="4.5" rx="1.2"/><rect x="7.25" y="9" width="7" height="4.5" rx="1.2"/>`),
  grid: svg(`<rect x="2" y="2" width="5" height="5" rx="1.2"/><rect x="9" y="2" width="5" height="5" rx="1.2"/><rect x="2" y="9" width="5" height="5" rx="1.2"/><rect x="9" y="9" width="5" height="5" rx="1.2"/>`),
  sidebarRight: svg(`<rect x="1.75" y="2.75" width="12.5" height="10.5" rx="2"/><path d="M10 2.75v10.5"/>`),
  sort: svg(`<path d="M5 3v10M2.6 5.4L5 3l2.4 2.4M11 13V3M8.6 10.6L11 13l2.4-2.4"/>`),
  small: svg(`<rect x="3" y="3" width="10" height="10" rx="2"/>`, 12),
  large: svg(`<rect x="1.5" y="1.5" width="13" height="13" rx="2.5"/>`),
  star: filled(STAR, 15),
  close: svg(`<path d="M4 4l8 8M12 4l-8 8"/>`),
  closeSmall: `<svg class="icon" width="8" height="8" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" aria-hidden="true"><path d="M4 4l8 8M12 4l-8 8"/></svg>`,
  info: svg(`<circle cx="8" cy="8" r="6.25"/><path d="M8 7.2v3.9M8 4.9v.1"/>`),
  expand: svg(`<path d="M9.5 2.5h4v4M13.5 2.5L9 7M6.5 13.5h-4v-4M2.5 13.5L7 9"/>`, 14),
  check: svg(`<path d="M3.5 8.4l2.9 2.9 6.1-6.6"/>`, 13),
  landscape: svg(`<rect x="1.75" y="4" width="12.5" height="8" rx="1.5"/>`),
  portrait: svg(`<rect x="4" y="1.75" width="8" height="12.5" rx="1.5"/>`),
  square: svg(`<rect x="3" y="3" width="10" height="10" rx="1.5"/>`),
  panorama: svg(`<rect x="0.75" y="5.5" width="14.5" height="5" rx="1.3"/>`),
  drop: svg(`<path d="M8 2.5v8M4.8 7.4L8 10.6l3.2-3.2M2.5 11v1.3c0 .7.6 1.2 1.2 1.2h8.6c.7 0 1.2-.5 1.2-1.2V11"/>`, 18),
};
