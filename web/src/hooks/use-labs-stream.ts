import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { adminKeys } from "@/hooks/use-admin";
import { labsKeys } from "@/hooks/use-labs";
import { parseSSEEvent } from "@/lib/sse";

export function useLabsStream(): void {
  const queryClient = useQueryClient();

  useEffect(() => {
    const stream = new EventSource("/sse/labs");

    stream.onmessage = (message) => {
      try {
        const event = parseSSEEvent(message.data);
        if (!event) return;

        if ("labId" in event) {
          queryClient.invalidateQueries({ queryKey: labsKeys.detail(event.labId) });
          queryClient.invalidateQueries({ queryKey: labsKeys.all });
        }

        if (event.type === "quota.snapshot") {
          queryClient.invalidateQueries({ queryKey: adminKeys.quota });
        }
      } catch (error) {
        console.warn("Failed to parse SSE event", error);
      }
    };

    stream.onerror = () => {
      console.info("SSE stream disconnected; browser will retry automatically");
    };

    return () => stream.close();
  }, [queryClient]);
}
