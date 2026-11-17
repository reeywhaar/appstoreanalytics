/** One app's values over a chart's days, as the chart draws it. */
export type Series = {
  id: string;
  name: string;
  /** A CSS colour: a palette slot's variable, or one made from the app's id. */
  color: string;
  values: (number | undefined)[];
};

/** How a chart draws its days: a column each, or a line through them. */
export type ChartMode = "bars" | "lines";

/** The palette's slots, --color-app-1 to --color-app-20 in main.css. */
const slots = 20;

/**
 * Each app's colour: the palette's slots in the order the apps were made, which is the order of
 * their ids, so a new app takes the next slot and no app already shown changes colour. Every app
 * known takes its slot whether or not it has anything to show, for the same reason.
 *
 * Past the palette, a bright colour from a hash of the id: the same app, the same colour, always.
 */
export function appColors(ids: string[]): Map<string, string> {
  const out = new Map<string, string>();
  [...new Set(ids)].toSorted(byId).forEach((id, i) => {
    out.set(id, i < slots ? `var(--color-app-${i + 1})` : hashed(id));
  });
  return out;
}

/** Apple's app ids are numbers that grow, too long for a float: compared by length, then text. */
function byId(a: string, b: string): number {
  return a.length - b.length || (a < b ? -1 : a > b ? 1 : 0);
}

/**
 * A colour from an id's hash: its hue from the hash, its lightness and chroma inside the bands
 * the palette keeps to on each surface, so it is as bright as the slots beside it.
 */
function hashed(id: string): string {
  // FNV-1a: small, and spreads neighbouring ids across the hues.
  let h = 0x811c9dc5;
  for (let i = 0; i < id.length; i++) {
    h ^= id.charCodeAt(i);
    h = Math.imul(h, 0x01000193);
  }
  const hue = (h >>> 0) % 360;
  return `light-dark(oklch(0.58 0.15 ${hue}), oklch(0.62 0.14 ${hue}))`;
}
