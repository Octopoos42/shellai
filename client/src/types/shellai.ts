export interface ResponseErrorApi {
  error_code: string;
  message: string;
  details?: Record<string, unknown>;
}

export interface ResponseMe {
  id: string;
  label: string;
  created_at: string;
}

export interface ResponseSession {
  id: string;
  title: string;
  created_at: string;
  updated_at: string;
}

export interface ResponseMessage {
  id: string;
  role: 'user' | 'assistant' | 'system' | 'tool_call' | 'tool_result';
  content: string;
  created_at: string;
}

export interface ResponseSessionWithMessages extends ResponseSession {
  messages: ResponseMessage[];
}

export interface ResponseAPIKey {
  id: string;
  label: string;
  key?: string;
  created_at: string;
  revoked_at?: string | null;
}

export interface EventChatToken {
  text: string;
}

export interface EventChatDone {
  content: string;
}

export interface EventChatPlan {
  description: string;
}

export interface EventChatToolRequest {
  id: string;
  tool: string;
  args?: Record<string, unknown>;
  explanation?: string;
}

export interface EventChatToolResult {
  tool: string;
  stdout?: string;
  stderr?: string;
  exit_code: number;
  rejected?: boolean;
}

export interface EventChatToolRejected {
  tool: string;
}

export interface EventShellChunk {
  text: string;
}

export interface EventShellExit {
  code: number;
  cwd?: string;
  error?: string;
}

export interface ConfigThirdPartyApi {
  id: string;
  name: string;
  endpoint: string;
  description: string;
  waitForUserConfirm: boolean;
  needClientProvideApiKey: boolean;
  request: string;
  response: string;
  commandType: 'bash' | 'python';
  commandTemplate: string;
}

export interface DefinitionChatTool {
  name: string;
  endpoint: string;
  description: string;
  waitForUserConfirm: boolean;
  needClientProvideApiKey: boolean;
  request: string;
  response: string;
  commandType: 'bash' | 'python';
  commandTemplate: string;
}

export interface SnippetSkill {
  id: string;
  name: string;
  description: string;
  content: string;
  is_public: boolean;
  created_at?: string;
  updated_at?: string;
}

export interface ValueStored<T> {
  value: T;
  expiresAt?: number;
}
