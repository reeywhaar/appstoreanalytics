import type { Status } from "@app/api/types";
import { Wordmark } from "@app/components/Wordmark";
import { Link } from "@app/islands/app/Link";
import { paths, type Route } from "@app/islands/app/route";

const link =
  "inline-flex min-h-8 items-center gap-1.5 rounded-md px-2 text-sm whitespace-nowrap text-muted hover:bg-fill hover:text-fg aria-[current=page]:text-fg pointer-coarse:min-h-10";

/**
 * The bar across the top: the name, which is home; Status, which carries how the sync is doing;
 * and Settings, where signing out is too.
 */
export function Header({
  route,
  status,
}: {
  route: Route;
  status: Status | undefined;
}) {
  const parts = status
    ? [
        status.apps,
        status.analytics,
        ...(status.sales.on ? [status.sales, status.rates] : []),
      ]
    : [];
  const failing = parts.some((p) => p.part.error);
  return (
    <header className="border-b border-line bg-bg">
      <div className="mx-auto flex w-full max-w-6xl items-center gap-2 px-4 py-2">
        <Link
          href={paths.dashboard(
            null,
            route.page === "dashboard" ? route.days : 30,
          )}
          className="font-semibold whitespace-nowrap"
        >
          <Wordmark />
        </Link>
        <span className="flex-1" />
        <Link
          href={paths.status}
          aria-current={route.page === "status" ? "page" : undefined}
          className={link}
        >
          {status?.running ? (
            <span className="size-2 animate-pulse rounded-full bg-series" />
          ) : failing ? (
            <span className="text-down" aria-hidden="true">
              !
            </span>
          ) : null}
          {status?.running ? (
            "Syncing…"
          ) : failing ? (
            <>
              <span className="hidden sm:inline">Sync needs a look</span>
              <span className="sm:hidden">Status</span>
            </>
          ) : (
            "Status"
          )}
        </Link>
        <Link
          href={paths.settings}
          aria-current={route.page === "settings" ? "page" : undefined}
          className={link}
        >
          Settings
        </Link>
      </div>
    </header>
  );
}
