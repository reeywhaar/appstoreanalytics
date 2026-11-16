import { Boundary } from "@app/components/Boundary";
import { Dashboard } from "@app/islands/app/Dashboard";
import { Header } from "@app/islands/app/Header";
import { Link } from "@app/islands/app/Link";
import { paths, useRoute, type Route } from "@app/islands/app/route";
import { Settings } from "@app/islands/app/Settings";
import { Status } from "@app/islands/app/Status";
import { useSync } from "@app/islands/app/sync";

export function App() {
  const route = useRoute();
  const status = useSync();
  return (
    <div className="flex min-h-dvh flex-col">
      <Header route={route} status={status} />
      <Boundary what="This page" key={route.page}>
        <Page route={route} />
      </Boundary>
    </div>
  );
}

function Page({ route }: { route: Route }) {
  switch (route.page) {
    case "dashboard":
      return <Dashboard app={route.app} days={route.days} />;
    case "settings":
      return <Settings />;
    case "status":
      return <Status />;
    case "missing":
      return (
        <main className="flex flex-1 flex-col items-center justify-center gap-3 p-4">
          <p className="text-sm text-muted">
            There is nothing at this address.
          </p>
          <Link href={paths.dashboard(null, 30)} className="text-sm underline">
            Go to the dashboard
          </Link>
        </main>
      );
  }
}
