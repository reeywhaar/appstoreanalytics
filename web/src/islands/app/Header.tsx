import { postAuthLogout } from "@app/api/actions";
import type { Status } from "@app/api/types";
import { Button } from "@app/components/Button";
import { Wordmark } from "@app/components/Wordmark";
import { leaveFor } from "@app/leave";
import { Link } from "@app/islands/app/Link";
import { paths, type Route } from "@app/islands/app/route";

async function signOut() {
  try {
    await postAuthLogout();
  } finally {
    leaveFor("/login");
  }
}

/** The bar across the top: the name, home; how the sync is doing; and signing out. */
export function Header({
  route,
  status,
}: {
  route: Route;
  status: Status | undefined;
}) {
  const failing =
    status &&
    [status.apps, status.sales, status.analytics].some((p) => p.part.error);
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
          className="inline-flex min-h-8 items-center gap-1.5 rounded-md px-2 text-sm whitespace-nowrap text-muted hover:bg-fill hover:text-fg aria-[current=page]:text-fg pointer-coarse:min-h-10"
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
        <Button variant="link" className="whitespace-nowrap" onClick={signOut}>
          Sign out
        </Button>
      </div>
    </header>
  );
}
