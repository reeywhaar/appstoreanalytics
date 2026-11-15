import { useEffect, useRef } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";

import { getStatus } from "@app/api/actions";
import { qk } from "@app/api/keys";
import type { Status } from "@app/api/types";

/**
 * How the sync is doing, asked often while one runs and seldom otherwise, and everything read
 * from the reports made stale once one finishes, so what is on screen is what was just read.
 */
export function useSync(): Status | undefined {
  const client = useQueryClient();
  const status = useQuery({
    queryKey: qk.status,
    queryFn: getStatus,
    refetchInterval: (q) => (q.state.data?.running ? 3000 : 60_000),
  });
  const running = status.data?.running ?? false;
  const was = useRef(running);
  useEffect(() => {
    if (was.current && !running) {
      void client.invalidateQueries({ queryKey: qk.reports });
      void client.invalidateQueries({ queryKey: qk.apps });
    }
    was.current = running;
  }, [running, client]);
  return status.data;
}
