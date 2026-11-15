/**
 * The arithmetic of an axis: clean numbers to put ticks at, and which days to label.
 */

/** A step of 1, 2 or 5 times a power of ten, so that about `count` of them cover `span`. */
export function niceStep(span: number, count: number): number {
  if (span <= 0) return 1;
  const raw = span / count;
  const power = 10 ** Math.floor(Math.log10(raw));
  const fraction = raw / power;
  const nice = fraction <= 1 ? 1 : fraction <= 2 ? 2 : fraction <= 5 ? 5 : 10;
  return nice * power;
}

/**
 * Ticks from below the lowest value to above the highest, always through zero, on clean
 * numbers. Whole numbers only when the values are counts.
 */
export function ticks(
  low: number,
  high: number,
  count = 4,
  whole = true,
): number[] {
  const min = Math.min(0, low);
  const max = Math.max(0, high, whole ? 1 : 0.01);
  let step = niceStep(max - min, count);
  if (whole) step = Math.max(1, Math.round(step));
  const out: number[] = [];
  for (let v = Math.floor(min / step) * step; v < max + step; v += step) {
    out.push(Number(v.toFixed(10)));
    if (v >= max) break;
  }
  return out;
}

/** Which of n days carry a label: about `count` of them, evenly, always the last. */
export function labelled(n: number, count = 6): Set<number> {
  const every = Math.max(1, Math.ceil(n / count));
  const out = new Set<number>();
  for (let i = n - 1; i >= 0; i -= every) out.add(i);
  return out;
}
