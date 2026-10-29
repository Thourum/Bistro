import { useEffect } from 'react';
import { wsClient } from '../services/ws/connection';

/** App owns the transport lifecycle; route consumers use wsClient directly. */
export function useWebSocketConnection(): void {
  useEffect(() => {
    wsClient.connect();
    return () => wsClient.disconnect();
  }, []);
}
