interface SSECallbacks {
  onEvent: (eventName: string, payload: string) => void;
  onError: (error: Error) => void;
}

function dispatchEvent(eventName: string, dataParts: string[], callbacks: SSECallbacks): void {
  if (!eventName || dataParts.length === 0) {
    return;
  }
  callbacks.onEvent(eventName, dataParts.join('\n'));
}

export async function streamSSE(
  response: Response,
  callbacks: SSECallbacks,
): Promise<void> {
  if (!response.ok) {
    const body = await response.text();
    throw new Error(`HTTP ${response.status}: ${body}`);
  }

  if (!response.body) {
    throw new Error('Missing response body for SSE stream');
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';

  let currentEvent = '';
  let currentData: string[] = [];

  try {
    while (true) {
      const { value, done } = await reader.read();
      if (done) {
        break;
      }

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split(/\r?\n/);
      buffer = lines.pop() ?? '';

      for (const line of lines) {
        if (!line) {
          dispatchEvent(currentEvent, currentData, callbacks);
          currentEvent = '';
          currentData = [];
          continue;
        }

        if (line.startsWith('event:')) {
          currentEvent = line.slice('event:'.length).trim();
          continue;
        }

        if (line.startsWith('data:')) {
          currentData.push(line.slice('data:'.length).trim());
        }
      }
    }

    dispatchEvent(currentEvent, currentData, callbacks);
  } catch (error) {
    callbacks.onError(error instanceof Error ? error : new Error(String(error)));
  } finally {
    reader.releaseLock();
  }
}
