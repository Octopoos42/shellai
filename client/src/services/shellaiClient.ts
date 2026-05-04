import { streamSSE } from 'src/services/sse';
import type {
  ResponseAPIKey,
  ResponseErrorApi,
  EventChatDone,
  EventChatPlan,
  EventChatToken,
  DefinitionChatTool,
  EventChatToolRejected,
  EventChatToolRequest,
  EventChatToolResult,
  ResponseMe,
  SnippetSkill,
  ResponseSession,
  ResponseSessionWithMessages,
  EventShellChunk,
  EventShellExit,
} from 'src/types/shellai';

interface ApiClientConfig {
  baseUrl: () => string;
  apiKey: () => string;
  adminAuth: () => string;
}

interface ShellEventHandlers {
  onStdout: (event: EventShellChunk) => void;
  onStderr: (event: EventShellChunk) => void;
  onExit: (event: EventShellExit) => void;
}

interface ChatEventHandlers {
  onToken: (event: EventChatToken) => void;
  onDone: (event: EventChatDone) => void;
  onPlan: (event: EventChatPlan) => void;
  onToolRequest: (event: EventChatToolRequest) => void;
  onToolResult: (event: EventChatToolResult) => void;
  onToolRejected: (event: EventChatToolRejected) => void;
  onErrorEvent: (event: ResponseErrorApi) => void;
}

function parseErrorMessage(errorBody: unknown, fallback: string): string {
  if (!errorBody || typeof errorBody !== 'object') {
    return fallback;
  }
  const asApiErr = errorBody as Partial<ResponseErrorApi>;
  if (typeof asApiErr.message === 'string' && asApiErr.message.length > 0) {
    return asApiErr.message;
  }
  return fallback;
}

export class ShellAIClient {
  constructor(private readonly config: ApiClientConfig) {}

  private getAuthHeaders(): HeadersInit {
    const key = this.config.apiKey().trim();
    return key ? { 'X-API-Key': key } : {};
  }

  private getAdminHeaders(): HeadersInit {
    const auth = this.config.adminAuth().trim();
    return auth ? { Authorization: `Basic ${auth}` } : {};
  }

  private async makeJsonRequest<T>(path: string, init?: RequestInit): Promise<T> {
    const response = await fetch(`${this.config.baseUrl()}${path}`, {
      ...init,
      headers: {
        'Content-Type': 'application/json',
        ...this.getAuthHeaders(),
        ...(init?.headers ?? {}),
      },
    });

    const bodyText = await response.text();
    const parsed = bodyText ? (JSON.parse(bodyText) as unknown) : undefined;

    if (!response.ok) {
      throw new Error(parseErrorMessage(parsed, `HTTP ${response.status}`));
    }

    return parsed as T;
  }

  getMe(): Promise<ResponseMe> {
    return this.makeJsonRequest<ResponseMe>('/api/me', { method: 'GET' });
  }

  listSessions(): Promise<ResponseSession[]> {
    return this.makeJsonRequest<ResponseSession[]>('/api/sessions', { method: 'GET' });
  }

  createSession(title: string): Promise<ResponseSession> {
    return this.makeJsonRequest<ResponseSession>('/api/sessions', {
      method: 'POST',
      body: JSON.stringify({ title }),
    });
  }

  getSession(id: string): Promise<ResponseSessionWithMessages> {
    return this.makeJsonRequest<ResponseSessionWithMessages>(`/api/sessions/${id}`, { method: 'GET' });
  }

  deleteSession(id: string): Promise<void> {
    return this.makeJsonRequest<void>(`/api/sessions/${id}`, { method: 'DELETE' });
  }

  renameSession(id: string, title: string): Promise<ResponseSession> {
    return this.makeJsonRequest<ResponseSession>(`/api/sessions/${id}`, {
      method: 'PATCH',
      body: JSON.stringify({ title }),
    });
  }

  async streamShell(command: string, handlers: ShellEventHandlers, cwd?: string): Promise<void> {
    const response = await fetch(`${this.config.baseUrl()}/api/shell/exec`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...this.getAuthHeaders(),
      },
      body: JSON.stringify({ command, cwd: cwd || undefined }),
    });

    await streamSSE(response, {
      onEvent: (eventName, payload) => {
        const data = payload ? (JSON.parse(payload) as unknown) : undefined;
        if (eventName === 'stdout') {
          handlers.onStdout(data as EventShellChunk);
          return;
        }
        if (eventName === 'stderr') {
          handlers.onStderr(data as EventShellChunk);
          return;
        }
        if (eventName === 'exit') {
          handlers.onExit(data as EventShellExit);
        }
      },
      onError: (error) => {
        throw error;
      },
    });
  }

  async streamChat(
    sessionId: string,
    message: string,
    model: string,
    tools: DefinitionChatTool[],
    handlers: ChatEventHandlers,
    signal?: AbortSignal,
  ): Promise<void> {
    const init: RequestInit = {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...this.getAuthHeaders(),
      },
      body: JSON.stringify({
        message,
        model: model || undefined,
        tools: tools.length > 0 ? tools : undefined,
      }),
    };

    if (signal) {
      init.signal = signal;
    }

    const response = await fetch(`${this.config.baseUrl()}/api/sessions/${sessionId}/chat`, {
      ...init,
    });

    await streamSSE(response, {
      onEvent: (eventName, payload) => {
        const data = payload ? (JSON.parse(payload) as unknown) : undefined;
        if (eventName === 'token') {
          handlers.onToken(data as EventChatToken);
          return;
        }
        if (eventName === 'done') {
          handlers.onDone(data as EventChatDone);
          return;
        }
        if (eventName === 'plan') {
          handlers.onPlan(data as EventChatPlan);
          return;
        }
        if (eventName === 'tool_request') {
          handlers.onToolRequest(data as EventChatToolRequest);
          return;
        }
        if (eventName === 'tool_result') {
          handlers.onToolResult(data as EventChatToolResult);
          return;
        }
        if (eventName === 'tool_rejected') {
          handlers.onToolRejected(data as EventChatToolRejected);
          return;
        }
        if (eventName === 'error') {
          handlers.onErrorEvent(data as ResponseErrorApi);
        }
      },
      onError: (error) => {
        throw error;
      },
    });
  }

  confirmToolCall(sessionId: string, confirmId: string, approved: boolean): Promise<void> {
    return this.makeJsonRequest<void>(`/api/sessions/${sessionId}/tool-confirm`, {
      method: 'POST',
      body: JSON.stringify({
        confirm_id: confirmId,
        approved,
      }),
    });
  }

  listSkills(): Promise<SnippetSkill[]> {
    return this.makeJsonRequest<SnippetSkill[]>('/api/skills', { method: 'GET' });
  }

  listPublicSkills(): Promise<SnippetSkill[]> {
    return this.makeJsonRequest<SnippetSkill[]>('/api/skills/public', { method: 'GET' });
  }

  createSkill(payload: Pick<SnippetSkill, 'name' | 'description' | 'content' | 'is_public'>): Promise<SnippetSkill> {
    return this.makeJsonRequest<SnippetSkill>('/api/skills', {
      method: 'POST',
      body: JSON.stringify(payload),
    });
  }

  updateSkill(id: string, payload: Partial<Pick<SnippetSkill, 'name' | 'description' | 'content' | 'is_public'>>): Promise<SnippetSkill> {
    return this.makeJsonRequest<SnippetSkill>(`/api/skills/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(payload),
    });
  }

  deleteSkill(id: string): Promise<void> {
    return this.makeJsonRequest<void>(`/api/skills/${id}`, {
      method: 'DELETE',
    });
  }

  listAPIKeys(): Promise<ResponseAPIKey[]> {
    return this.makeJsonRequest<ResponseAPIKey[]>('/api/admin/apikeys', {
      method: 'GET',
      headers: {
        ...this.getAdminHeaders(),
      },
    });
  }

  createAPIKey(label: string): Promise<ResponseAPIKey> {
    return this.makeJsonRequest<ResponseAPIKey>('/api/admin/apikeys', {
      method: 'POST',
      headers: {
        ...this.getAdminHeaders(),
      },
      body: JSON.stringify({ label }),
    });
  }

  revokeAPIKey(id: string): Promise<ResponseAPIKey> {
    return this.makeJsonRequest<ResponseAPIKey>(`/api/admin/apikeys/${id}`, {
      method: 'DELETE',
      headers: {
        ...this.getAdminHeaders(),
      },
    });
  }
}
