import { useEffect, useState } from "react";

import { copy } from "@app/clipboard";
import { CheckIcon, CopyIcon } from "@app/components/icons/Icon";

/** Values to be copied somewhere else, each under its label with a button that copies it. */
export function CopyBlock({
  rows,
}: {
  rows: { label: string; value: string }[];
}) {
  return (
    <div className="flex flex-col gap-1.5 rounded-md border border-line bg-page py-2 pr-1.5 pl-3 text-sm">
      {rows.map((r) => (
        <CopyRow key={r.label} {...r} />
      ))}
    </div>
  );
}

function CopyRow({ label, value }: { label: string; value: string }) {
  const [copied, setCopied] = useState(false);
  useEffect(() => {
    if (!copied) return;
    const t = window.setTimeout(() => setCopied(false), 1500);
    return () => window.clearTimeout(t);
  }, [copied]);

  return (
    <div className="flex items-start gap-2">
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="text-muted">{label}</span>
        <code className="font-mono break-all">{value}</code>
      </div>
      <button
        type="button"
        aria-label={`Copy ${label.toLowerCase()}`}
        title={copied ? "Copied" : "Copy"}
        onClick={async () => setCopied(await copy(value))}
        className="mt-0.5 shrink-0 rounded-md p-1.5 text-faint hover:bg-fill hover:text-fg"
      >
        {copied ? (
          <CheckIcon className="size-4" />
        ) : (
          <CopyIcon className="size-4" />
        )}
      </button>
    </div>
  );
}
