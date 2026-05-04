// Legacy compatibility wrapper. The exported API surface now lives in shellaiClient.ts.
export { ShellAIClient as AwshClient } from './shellaiClient';

  updateSkill(id: string, payload: Partial<Pick<SkillSnippet, 'name' | 'description' | 'content' | 'is_public'>>): Promise<SkillSnippet> {
    return this.requestJSON<SkillSnippet>(`/api/skills/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(payload),
    });
  }

  deleteSkill(id: string): Promise<void> {
    return this.requestJSON<void>(`/api/skills/${id}`, {
      method: 'DELETE',
    });
  }

  listAPIKeys(): Promise<APIKeyResponse[]> {
    return this.requestJSON<APIKeyResponse[]>('/api/admin/apikeys', {
      method: 'GET',
      headers: {
        ...this.adminHeaders(),
      },
    });
  }

  createAPIKey(label: string): Promise<APIKeyResponse> {
    return this.requestJSON<APIKeyResponse>('/api/admin/apikeys', {
      method: 'POST',
      headers: {
        ...this.adminHeaders(),
      },
      body: JSON.stringify({ label }),
    });
  }

  revokeAPIKey(id: string): Promise<APIKeyResponse> {
    return this.requestJSON<APIKeyResponse>(`/api/admin/apikeys/${id}`, {
      method: 'DELETE',
      headers: {
        ...this.adminHeaders(),
      },
    });
  }
}
