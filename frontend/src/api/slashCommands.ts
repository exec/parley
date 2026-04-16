import { apiClient } from './client';
import { BotCommand, InteractionInvokeResponse } from './types';

/** List every slash command registered for the given server. */
export async function listServerCommands(serverID: string): Promise<BotCommand[]> {
  const result = await apiClient.get<BotCommand[] | null>(`/servers/${serverID}/commands`);
  return result ?? [];
}

/** Invoke a slash command. Backend responds with an interaction token; the actual
 *  bot message arrives later through the normal MESSAGE_CREATE WS event. */
export async function invokeCommand(
  channelID: string,
  commandID: number,
  options: Record<string, unknown>,
): Promise<InteractionInvokeResponse> {
  return apiClient.post<InteractionInvokeResponse>(`/channels/${channelID}/interactions`, {
    command_id: commandID,
    options,
  });
}
