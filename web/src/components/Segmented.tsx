import type { ReactNode } from "react";

/**
 * A choice of a few, side by side, the one chosen raised: the look of the dashboard's range and
 * chart switches, for a choice in a form. Each is a button pressed or not, in a group named for
 * what it chooses.
 */
export function Segmented<T extends string>({
  label,
  value,
  options,
  onChange,
  className = "",
}: {
  label: string;
  value: T;
  options: { value: T; label: ReactNode }[];
  onChange: (value: T) => void;
  className?: string;
}) {
  return (
    <div
      className={`flex rounded-md bg-control p-0.5 ${className}`}
      role="group"
      aria-label={label}
    >
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          aria-pressed={o.value === value}
          onClick={() => onChange(o.value)}
          className="inline-flex min-h-7 flex-1 items-center justify-center rounded px-2.5 text-sm text-muted hover:text-fg aria-pressed:bg-bg aria-pressed:text-fg aria-pressed:shadow-sm"
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}
