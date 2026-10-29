import type { WSInboundMessage, WsEventType } from '../../../types/wsSchemas';

export interface WsEventContext {
  readonly resolveResponse: (id: string | undefined, data: unknown) => boolean;
}

export type WsEventHandler<Type extends WsEventType = WsEventType> = (
  message: Readonly<WSInboundMessage<Type>>,
  context: WsEventContext,
) => void;

/** Resolves response-only events without changing domain state. */
export function resolveWsResponse(
  message: Readonly<WSInboundMessage>,
  context: WsEventContext,
): void {
  context.resolveResponse(message.id, message.data);
}
