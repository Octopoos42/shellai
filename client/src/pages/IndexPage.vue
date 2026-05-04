<template>
  <q-page class="shellai-page">
    <div class="shellai-layout-outer">
      <!-- Icon sidebar: toggle panels -->
      <nav class="panel-sidebar">
        <q-btn
          flat round dense
          :color="panelVisible.workspace ? 'cyan-4' : 'grey-7'"
          icon="storage"
          @click="togglePanel('workspace')"
        >
          <q-tooltip anchor="center right" self="center left">Workspace</q-tooltip>
        </q-btn>
        <q-btn
          flat round dense
          :color="panelVisible.chat ? 'cyan-4' : 'grey-7'"
          icon="chat"
          @click="togglePanel('chat')"
        >
          <q-tooltip anchor="center right" self="center left">Chat</q-tooltip>
        </q-btn>
        <q-btn
          flat round dense
          :color="panelVisible.console ? 'cyan-4' : 'grey-7'"
          icon="terminal"
          @click="togglePanel('console')"
        >
          <q-tooltip anchor="center right" self="center left">Console</q-tooltip>
        </q-btn>
      </nav>

      <!-- Panels area -->
      <div ref="panelsAreaRef" class="shellai-panels-area">
        <!-- Workspace panel -->
        <aside
          v-show="panelVisible.workspace"
          class="panel left-panel"
          :style="workspacePanelStyle"
        >
          <div class="panel-title">Workspace</div>
          <q-card flat bordered class="card-block q-mb-md">
            <q-card-section>
              <div class="text-overline text-cyan-3">Connection</div>
              <div class="text-caption text-grey-5 q-mt-xs">{{ apiBaseUrl }}</div>
              <div class="row q-col-gutter-sm q-mt-sm">
                <div class="col-12">
                  <q-btn
                    :color="apiKeyVerified ? 'primary' : 'grey-7'"
                    :label="apiKeyVerified ? 'API Key Verified' : 'Verify API Key'"
                    :icon="apiKeyVerified ? 'verified' : 'vpn_key'"
                    no-caps
                    class="full-width"
                    :loading="connecting"
                    @click="verifyConnection"
                  />
                </div>
                <div class="col-12">
                  <q-btn
                    flat
                    color="cyan-4"
                    label="Connection Settings"
                    icon="settings"
                    no-caps
                    class="full-width"
                    @click="showSettingsDialog = true"
                  />
                </div>
              </div>
              <q-banner
                v-if="meInfo"
                dense
                rounded
                class="bg-teal-10 text-teal-2 q-mt-sm"
              >
                Key label: {{ meInfo.label }}
              </q-banner>
            </q-card-section>
          </q-card>

          <q-card flat bordered class="card-block q-mb-md">
            <q-card-section>
              <div class="row items-center justify-between">
                <div class="text-overline text-cyan-3">Sessions</div>
                <q-btn flat dense icon="add" color="cyan-4" @click="createSession()" />
              </div>
              <q-list separator class="session-list q-mt-sm">
                <q-item
                  v-for="s in sessions"
                  :key="s.id"
                  clickable
                  active-class="session-active"
                  :active="s.id === activeSessionId"
                  @click="selectSession(s.id)"
                >
                  <q-item-section>
                    <q-item-label>{{ s.title || 'Untitled session' }}</q-item-label>
                    <q-item-label caption>{{ formatTime(s.updated_at) }}</q-item-label>
                  </q-item-section>
                  <q-item-section side>
                    <div class="row no-wrap q-gutter-xs">
                      <q-btn
                        dense
                        flat
                        icon="edit"
                        color="amber-4"
                        @click.stop="openRenameSessionDialog(s)"
                      />
                      <q-btn
                        dense
                        flat
                        icon="delete"
                        color="red-4"
                        @click.stop="deleteSession(s.id)"
                      />
                    </div>
                  </q-item-section>
                </q-item>
                <q-item v-if="sessions.length === 0">
                  <q-item-section class="text-caption text-grey-5">
                    No sessions yet.
                  </q-item-section>
                </q-item>
              </q-list>
            </q-card-section>
          </q-card>

          <q-card flat bordered class="card-block q-mb-md">
            <q-card-section>
              <div class="row items-center justify-between">
                <div class="text-overline text-cyan-3">Third-party APIs</div>
                <q-btn flat dense icon="add" color="cyan-4" @click="openAddApiDialog" />
              </div>
              <q-list dense class="q-mt-sm">
                <q-item v-for="api in thirdPartyApis" :key="api.id" clickable @click="toggleApi(api.id)">
                  <q-item-section avatar>
                    <q-icon :name="activeApiIds.includes(api.id) ? 'check_circle' : 'radio_button_unchecked'" />
                  </q-item-section>
                  <q-item-section>
                    <q-item-label class="tool-name">{{ api.name }}</q-item-label>
                    <q-item-label caption class="tool-desc">{{ api.endpoint }}</q-item-label>
                  </q-item-section>
                  <q-item-section side>
                    <div class="row no-wrap q-gutter-xs">
                      <q-btn dense flat icon="edit" color="amber-4" @click.stop="openEditApiDialog(api)" />
                      <q-btn dense flat icon="delete" color="red-4" @click.stop="removeApi(api.id)" />
                    </div>
                  </q-item-section>
                </q-item>
                <q-item v-if="thirdPartyApis.length === 0">
                  <q-item-section class="text-caption text-grey-5">
                    No API tools yet.
                  </q-item-section>
                </q-item>
              </q-list>
            </q-card-section>
          </q-card>

          <q-card flat bordered class="card-block">
            <q-card-section>
              <div class="row items-center justify-between">
                <div class="text-overline text-cyan-3">Skills</div>
                <q-btn flat dense icon="add" color="cyan-4" @click="openAddSkillDialog" />
              </div>
              <q-list dense class="q-mt-sm">
                <q-item v-for="skill in skills" :key="skill.id" clickable @click="toggleSkill(skill.id)">
                  <q-item-section avatar>
                    <q-icon :name="activeSkillIds.includes(skill.id) ? 'check_circle' : 'radio_button_unchecked'" />
                  </q-item-section>
                  <q-item-section>
                    <q-item-label class="tool-name">
                      {{ skill.name }}
                      <q-chip
                        v-if="skill.is_public"
                        dense
                        size="sm"
                        color="teal-9"
                        text-color="teal-2"
                        class="q-ml-xs"
                      >
                        public
                      </q-chip>
                    </q-item-label>
                    <q-item-label caption class="tool-desc">{{ skill.description }}</q-item-label>
                  </q-item-section>
                  <q-item-section side>
                    <div class="row no-wrap q-gutter-xs">
                      <q-btn
                        dense
                        flat
                        :icon="skill.is_public ? 'visibility_off' : 'share'"
                        :color="skill.is_public ? 'orange-4' : 'teal-4'"
                        @click.stop="toggleSkillVisibility(skill)"
                      />
                      <q-btn dense flat icon="edit" color="amber-4" @click.stop="openEditSkillDialog(skill)" />
                      <q-btn dense flat icon="delete" color="red-4" @click.stop="removeSkill(skill.id)" />
                    </div>
                  </q-item-section>
                </q-item>
                <q-item v-if="skills.length === 0">
                  <q-item-section class="text-caption text-grey-5">
                    No skills yet.
                  </q-item-section>
                </q-item>
              </q-list>

              <q-separator class="q-my-sm" />
              <div class="text-caption text-cyan-2 q-mb-xs">Public marketplace</div>
              <q-list dense>
                <q-item v-for="skill in publicSkills" :key="`public-${skill.id}`">
                  <q-item-section>
                    <q-item-label class="tool-name">{{ skill.name }}</q-item-label>
                    <q-item-label caption class="tool-desc">{{ skill.description }}</q-item-label>
                  </q-item-section>
                </q-item>
                <q-item v-if="publicSkills.length === 0">
                  <q-item-section class="text-caption text-grey-5">
                    No public skills available.
                  </q-item-section>
                </q-item>
              </q-list>
            </q-card-section>
          </q-card>
        </aside>

        <!-- Resize handle: workspace ↔ right panels -->
        <div
          v-show="panelVisible.workspace && (panelVisible.chat || panelVisible.console)"
          class="resize-handle"
          @mousedown="e => startResize(e, 'workspace')"
        ></div>

        <!-- Chat panel -->
        <section
          v-show="panelVisible.chat"
          class="panel terminal-panel chat-panel"
          :style="chatPanelStyle"
        >
          <div class="terminal-header">
            <div class="row items-center q-gutter-sm">
              <q-chip color="cyan-10" text-color="cyan-2" icon="chat">Chat</q-chip>
              <q-select
                v-model="selectedModel"
                :options="modelOptions"
                dense
                outlined
                options-dense
                popup-content-class="shellai-select-menu"
                class="model-select"
                label="Model"
              />
            </div>
            <div class="row q-gutter-sm">
              <q-btn flat color="cyan-4" icon="cleaning_services" no-caps label="Clear" @click="clearTerminal" />
            </div>
          </div>

          <div ref="terminalBody" class="terminal-body">
            <div v-for="line in terminalLines" :key="line.id" class="terminal-line" :class="`kind-${line.kind}`">
              <span class="line-time">{{ line.time }}</span>
              <span class="line-prefix">{{ line.prefix }}</span>
              <template v-if="line.kind === 'assistant'">
                <div class="line-markdown" v-html="renderMarkdown(line.text)"></div>
              </template>
              <template v-else>
                <span class="line-text">{{ visibleTerminalLineText(line) }}</span>
                <div v-if="isToolLineCollapsible(line)" class="q-mt-xs">
                  <q-btn
                    dense
                    flat
                    no-caps
                    size="sm"
                    color="cyan-4"
                    :label="isToolLineExpanded(line.id) ? 'Hide output' : 'Show more'"
                    @click="toggleToolLine(line.id)"
                  />
                </div>
              </template>
            </div>
            <div v-if="terminalLines.length === 0" class="terminal-empty">
              Type your goal. Example: Search for videos on YouTube about distributed tracing.
            </div>
          </div>

          <div class="terminal-input">
            <q-input
              v-model="prompt"
              type="textarea"
              rows="10"
              autogrow
              outlined
              dense
              color="cyan-4"
              placeholder="Ask the agent, or use !sh <command>"
              class="shellai-prompt-box"
              @keyup.enter.exact.prevent="sendPrompt"
            />
            <div class="row q-gutter-sm q-mt-sm">
              <q-btn
                color="primary"
                icon="send"
                label="Run"
                no-caps
                :loading="busy"
                @click="sendPrompt"
              />
              <q-chip clickable color="indigo-10" text-color="indigo-2" @click="prompt = '/help'">/help</q-chip>
              <q-chip clickable color="indigo-10" text-color="indigo-2" @click="prompt = '/compact'">/compact</q-chip>
              <q-chip clickable color="indigo-10" text-color="indigo-2" @click="prompt = '!sh ls -la'">!sh</q-chip>
              <q-space />
              <q-btn
                flat
                color="orange-3"
                icon="stop_circle"
                label="Interrupt"
                no-caps
                :disable="!isChatStreaming"
                @click="interruptChat"
              />
            </div>
          </div>
        </section>

        <!-- Resize handle: chat ↔ console -->
        <div
          v-show="panelVisible.chat && panelVisible.console"
          class="resize-handle"
          @mousedown="e => startResize(e, 'chat')"
        ></div>

        <!-- Console panel -->
        <section
          v-show="panelVisible.console"
          class="panel terminal-panel console-panel"
          :style="consolePanelStyle"
        >
          <div class="terminal-header">
            <div class="row items-center q-gutter-sm">
              <q-chip color="cyan-10" text-color="cyan-2" icon="terminal">Console</q-chip>
              <q-chip color="blue-10" text-color="blue-2" icon="folder_open">{{ consoleCwd || '(server default cwd)' }}</q-chip>
            </div>
            <div class="row q-gutter-sm">
              <q-btn
                flat
                color="blue-3"
                icon="restart_alt"
                no-caps
                label="Reset CWD"
                :disable="!consoleCwd"
                @click="resetConsoleCwd"
              />
              <q-btn flat color="cyan-4" icon="cleaning_services" no-caps label="Clear" @click="consoleLines = []" />
            </div>
          </div>

          <div ref="consoleBody" class="terminal-body">
            <div v-for="line in consoleLines" :key="line.id" class="terminal-line" :class="`kind-${line.kind}`">
              <span class="line-time">{{ line.time }}</span>
              <span class="line-prefix">{{ line.prefix }}</span>
              <span class="line-text">{{ line.text }}</span>
            </div>
            <div v-if="consoleLines.length === 0" class="terminal-empty">
              Enter a shell command and run it directly on the server.
            </div>
          </div>

          <div class="terminal-input">
            <q-input
              v-model="consoleCommand"
              outlined
              dense
              color="cyan-4"
              placeholder="Console command, e.g. pwd"
              @keyup.enter.exact.prevent="runConsoleCommand"
            />
            <div class="row q-gutter-sm q-mt-sm">
              <q-btn
                color="secondary"
                icon="play_arrow"
                label="Execute"
                no-caps
                :loading="consoleBusy"
                @click="runConsoleCommand"
              />
            </div>
          </div>
        </section>
      </div>
    </div>

    <q-dialog v-model="showConfirmDialog" persistent>
      <q-card class="dialog-card">
        <q-card-section>
          <div class="text-h6">{{ confirmDialogTitle }}</div>
          <div class="text-caption text-grey-6 q-mt-xs">
            {{ confirmDialogDescription }}
          </div>
        </q-card-section>
        <q-card-section>
          <q-input
            v-if="pendingAction?.requiresUserApiKey"
            v-model="pendingUserApiKey"
            label="Third-party API Key"
            outlined
            dense
            type="password"
          />
          <q-input
            v-model="pendingActionCommandPreview"
            type="textarea"
            autogrow
            outlined
            readonly
            class="q-mt-sm"
          />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat label="Cancel" color="grey-5" @click="cancelPendingAction" />
          <q-btn color="primary" label="Execute" @click="confirmPendingAction" />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <q-dialog v-model="showToolConfirmDialog" persistent>
      <q-card class="dialog-card">
        <q-card-section>
          <div class="text-h6">Approve tool call</div>
          <div class="text-caption text-grey-6 q-mt-xs">
            The backend agent requested a tool execution. Approve or reject it.
          </div>
        </q-card-section>
        <q-card-section>
          <q-input
            :model-value="pendingToolConfirm?.tool ?? ''"
            label="Tool"
            outlined
            dense
            readonly
          />
          <q-input
            :model-value="pendingToolConfirm?.explanation ?? ''"
            label="Explanation"
            outlined
            dense
            readonly
            class="q-mt-sm"
          />
          <q-input
            :model-value="JSON.stringify(pendingToolConfirm?.args ?? {}, null, 2)"
            type="textarea"
            autogrow
            outlined
            dense
            readonly
            class="q-mt-sm"
            label="Arguments"
          />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat label="Reject" color="red-4" @click="denyPendingToolRequest" />
          <q-btn color="primary" label="Approve" @click="approvePendingToolRequest" />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <q-dialog v-model="showRenameDialog">
      <q-card class="dialog-card">
        <q-card-section class="row items-center justify-between">
          <div class="text-h6">Rename session</div>
          <q-btn dense flat icon="close" v-close-popup />
        </q-card-section>
        <q-card-section>
          <q-input v-model="renameSessionState.title" label="Session title" outlined dense />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat label="Cancel" color="grey-5" v-close-popup />
          <q-btn color="primary" label="Save" @click="confirmRenameSession" />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <q-dialog v-model="showSettingsDialog">
      <q-card class="dialog-card">
        <q-card-section class="row items-center justify-between">
          <div class="text-h6">Connection Settings</div>
          <q-btn dense flat icon="close" v-close-popup />
        </q-card-section>
        <q-card-section>
          <q-input v-model="settingsBaseUrl" label="API Base URL" outlined dense />
          <q-input v-model="settingsApiKey" label="X-API-Key" outlined dense type="password" class="q-mt-sm" />
          <q-select
            v-model="apiKeyTtlPreset"
            :options="ttlOptions"
            option-label="label"
            option-value="value"
            emit-value
            map-options
            outlined
            dense
            popup-content-class="shellai-select-menu"
            class="q-mt-sm"
            label="How long to store API key"
          />
        </q-card-section>
        <q-separator />
        <q-card-section>
          <div class="text-subtitle2">Admin (Basic Auth)</div>
          <q-input v-model="adminUsername" label="Admin Username" outlined dense class="q-mt-sm" />
          <q-input v-model="adminPassword" label="Admin Password" outlined dense type="password" class="q-mt-sm" />
          <q-banner
            v-if="lastCreatedApiKey"
            dense
            rounded
            class="q-mt-sm bg-indigo-10 text-indigo-1"
          >
            <div class="row items-center justify-between q-gutter-sm">
              <div class="col">
                <div class="text-caption">New plaintext API key</div>
                <div class="text-body2 text-weight-bold" style="word-break: break-all;">{{ lastCreatedApiKey }}</div>
              </div>
              <q-btn flat dense icon="content_copy" label="Copy" @click="copyToClipboard(lastCreatedApiKey)" />
            </div>
          </q-banner>
          <q-banner
            v-if="adminFeedback"
            dense
            rounded
            class="q-mt-sm"
            :class="adminFeedbackType === 'positive' ? 'bg-teal-10 text-teal-2' : adminFeedbackType === 'warning' ? 'bg-orange-10 text-orange-2' : 'bg-red-10 text-red-2'"
          >
            {{ adminFeedback }}
          </q-banner>
          <div class="row q-gutter-sm q-mt-sm">
            <q-btn color="secondary" no-caps label="List API Keys" :loading="adminBusy" @click="loadAdminApiKeys" />
            <q-btn flat color="secondary" no-caps label="Create Key" :loading="adminBusy" @click="createAdminApiKey" />
          </div>
          <q-list dense bordered class="q-mt-sm" v-if="adminApiKeys.length > 0">
            <q-item v-for="k in adminApiKeys" :key="k.id">
              <q-item-section>
                <q-item-label>{{ k.label }}</q-item-label>
                <q-item-label caption>{{ k.id }}</q-item-label>
                <q-item-label caption v-if="k.key">new key: {{ k.key }}</q-item-label>
              </q-item-section>
              <q-item-section side>
                <q-btn dense flat icon="block" color="red-4" @click="revokeAdminApiKey(k.id)" />
              </q-item-section>
            </q-item>
          </q-list>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat label="Cancel" color="grey-5" v-close-popup />
          <q-btn color="primary" label="Save" @click="saveSettings" />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <q-dialog v-model="showApiDialog">
      <q-card class="dialog-card">
        <q-card-section class="row items-center justify-between">
          <div class="text-h6">{{ editingApiId ? 'Edit Third-party API' : 'Add Third-party API' }}</div>
          <q-btn dense flat icon="close" v-close-popup />
        </q-card-section>
        <q-card-section>
          <q-input v-model="draftApi.name" label="Name" outlined dense />
          <q-input v-model="draftApi.endpoint" label="Endpoint" outlined dense class="q-mt-sm" />
          <q-input v-model="draftApi.description" label="Description" type="textarea" autogrow outlined dense class="q-mt-sm" />
          <q-input v-model="draftApi.request" label="Request format" type="textarea" autogrow outlined dense class="q-mt-sm" />
          <q-input v-model="draftApi.response" label="Response format" type="textarea" autogrow outlined dense class="q-mt-sm" />
          <q-select
            v-model="draftApi.commandType"
            :options="['bash', 'python']"
            outlined
            dense
            class="q-mt-sm"
            label="Command type"
          />
          <q-input
            v-model="draftApi.commandTemplate"
            type="textarea"
            autogrow
            outlined
            dense
            class="q-mt-sm"
            label="Command template (supports {{query}} {{endpoint}} {{apiKey}})"
          />
          <div class="row q-col-gutter-md q-mt-sm">
            <div class="col-6">
              <q-toggle v-model="draftApi.waitForUserConfirm" label="Wait for user confirmation" />
            </div>
            <div class="col-6">
              <q-toggle v-model="draftApi.needClientProvideApiKey" label="Need client API key" />
            </div>
          </div>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat label="Cancel" color="grey-5" v-close-popup />
          <q-btn color="primary" :label="editingApiId ? 'Save' : 'Add'" @click="addApi" />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <q-dialog v-model="showSkillDialog">
      <q-card class="dialog-card">
        <q-card-section class="row items-center justify-between">
          <div class="text-h6">{{ editingSkillId ? 'Edit Skill' : 'Add Skill' }}</div>
          <q-btn dense flat icon="close" v-close-popup />
        </q-card-section>
        <q-card-section>
          <q-input v-model="draftSkill.name" label="Name" outlined dense />
          <q-input v-model="draftSkill.description" label="Description" outlined dense class="q-mt-sm" />
          <q-input
            v-model="draftSkill.content"
            type="textarea"
            autogrow
            outlined
            dense
            class="q-mt-sm"
            label="Skill content (prepended to user prompt)"
          />
          <q-toggle v-model="draftSkill.is_public" label="Share to public marketplace" class="q-mt-sm" />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat label="Cancel" color="grey-5" v-close-popup />
          <q-btn color="primary" :label="editingSkillId ? 'Save' : 'Add'" @click="addSkill" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue';
import { Notify } from 'quasar';
import MarkdownIt from 'markdown-it';
import hljs from 'highlight.js';
import 'highlight.js/styles/github-dark.css';
import { ShellAIClient } from 'src/services/shellaiClient';
import {
  clearApiKey,
  loadApiBase,
  loadApiKey,
  loadThirdPartyApis,
  saveApiBase,
  saveApiKey,
  saveThirdPartyApis,
} from 'src/services/storage';
import type {
  APIKeyResponse,
  ChatToolDefinition,
  MeResponse,
  SessionResponse,
  SkillSnippet,
  ThirdPartyApiConfig,
} from 'src/types/shellai';

type TerminalKind = 'user' | 'assistant' | 'stdout' | 'stderr' | 'system';

interface TerminalLine {
  id: number;
  kind: TerminalKind;
  prefix: string;
  text: string;
  time: string;
}

interface PlannedAction {
  api?: ThirdPartyApiConfig;
  command: string;
  requiresConfirmation: boolean;
  requiresUserApiKey: boolean;
}

interface SessionRenameState {
  id: string;
  title: string;
}

interface PendingToolConfirm {
  id: string;
  tool: string;
  args?: Record<string, unknown>;
  explanation?: string;
}

// Markdown renderer with highlight.js syntax highlighting
function hljsHighlight(str: string, lang: string): string {
  if (lang && hljs.getLanguage(lang)) {
    try {
      return `<pre class="hljs"><code>${hljs.highlight(str, { language: lang, ignoreIllegals: true }).value}</code></pre>`;
    } catch {
      // fall through to default
    }
  }
  return '';
}

const md: MarkdownIt = new MarkdownIt({
  highlight(str, lang) {
    return hljsHighlight(str, lang) || `<pre class="hljs"><code>${md.utils.escapeHtml(str)}</code></pre>`;
  },
  linkify: true,
  breaks: true,
});

function renderMarkdown(text: string): string {
  return md.render(text);
}

// Panel visibility
const panelVisible = ref({ workspace: true, chat: true, console: true });

function togglePanel(panel: 'workspace' | 'chat' | 'console'): void {
  panelVisible.value[panel] = !panelVisible.value[panel];
}

// Panel resizing
const panelsAreaRef = ref<HTMLElement | null>(null);
const workspaceWidthPx = ref(320);
const chatWidthPx = ref<number | null>(null);

type ResizeTarget = 'workspace' | 'chat';
let resizeTarget: ResizeTarget | null = null;
let resizeStartX = 0;
let resizeStartWidth = 0;
let resizeContainerWidth = 0;

function startResize(e: MouseEvent, target: ResizeTarget): void {
  resizeTarget = target;
  resizeStartX = e.clientX;
  if (target === 'workspace') {
    resizeStartWidth = workspaceWidthPx.value;
  } else {
    if (!panelsAreaRef.value) return;
    const totalWidth = panelsAreaRef.value.getBoundingClientRect().width;
    const workspaceUsed = panelVisible.value.workspace ? workspaceWidthPx.value + 4 : 0;
    resizeContainerWidth = totalWidth - workspaceUsed;
    resizeStartWidth = chatWidthPx.value ?? resizeContainerWidth / 2;
  }
  document.addEventListener('mousemove', onResizeMove);
  document.addEventListener('mouseup', stopResize);
  document.body.style.cursor = 'col-resize';
  document.body.style.userSelect = 'none';
}

function onResizeMove(e: MouseEvent): void {
  if (!resizeTarget) return;
  const dx = e.clientX - resizeStartX;
  if (resizeTarget === 'workspace') {
    workspaceWidthPx.value = Math.max(200, Math.min(600, resizeStartWidth + dx));
  } else {
    chatWidthPx.value = Math.max(200, Math.min(resizeContainerWidth - 200, resizeStartWidth + dx));
  }
}

function stopResize(): void {
  resizeTarget = null;
  document.removeEventListener('mousemove', onResizeMove);
  document.removeEventListener('mouseup', stopResize);
  document.body.style.cursor = '';
  document.body.style.userSelect = '';
}

onUnmounted(stopResize);

const workspacePanelStyle = computed(() => {
  const anyRight = panelVisible.value.chat || panelVisible.value.console;
  if (!anyRight) return { flex: '1 1 0', minWidth: 0 };
  return { flex: `0 0 ${workspaceWidthPx.value}px`, minWidth: 0 };
});

const chatPanelStyle = computed(() => {
  if (!panelVisible.value.console) return { flex: '1 1 0', minWidth: 0 };
  if (chatWidthPx.value === null) return { flex: '1 1 0', minWidth: 0 };
  return { flex: `0 0 ${chatWidthPx.value}px`, minWidth: 0 };
});

const consolePanelStyle = computed(() => ({ flex: '1 1 0', minWidth: 0 }));

const terminalBody = ref<HTMLElement | null>(null);
const consoleBody = ref<HTMLElement | null>(null);

const apiBaseUrl = ref(loadApiBase());
const apiKey = ref(loadApiKey());

const settingsBaseUrl = ref(apiBaseUrl.value);
const settingsApiKey = ref(apiKey.value);
const apiKeyTtlPreset = ref<number>(24 * 60 * 60 * 1000);

const adminUsername = ref(sessionStorage.getItem('shellai.admin.user') ?? '');
const adminPassword = ref(sessionStorage.getItem('shellai.admin.pass') ?? '');
const adminApiKeys = ref<APIKeyResponse[]>([]);

const modelOptions = ['claude', 'openai', 'deepseek'];
const selectedModel = ref('claude');

const showSettingsDialog = ref(false);
const showApiDialog = ref(false);
const showSkillDialog = ref(false);
const showConfirmDialog = ref(false);
const showRenameDialog = ref(false);
const showToolConfirmDialog = ref(false);
const adminFeedback = ref('');
const adminFeedbackType = ref<'positive' | 'negative' | 'warning' | ''>('');
const adminBusy = ref(false);
const lastCreatedApiKey = ref('');
const apiKeyVerified = ref(false);

const prompt = ref('');
const consoleCommand = ref('');
const busy = ref(false);
const consoleBusy = ref(false);
const connecting = ref(false);
const isChatStreaming = ref(false);
const meInfo = ref<MeResponse | null>(null);

const sessions = ref<SessionResponse[]>([]);
const activeSessionId = ref('');

const terminalLines = ref<TerminalLine[]>([]);
let lineCounter = 0;
const expandedToolLineIds = ref<number[]>([]);
const consoleLines = ref<TerminalLine[]>([]);
let consoleLineCounter = 0;
const consoleCwd = ref('');

const thirdPartyApis = ref<ThirdPartyApiConfig[]>(loadThirdPartyApis());
const activeApiIds = ref<string[]>(thirdPartyApis.value.map((api) => api.id));
const skills = ref<SkillSnippet[]>([]);
const publicSkills = ref<SkillSnippet[]>([]);
const activeSkillIds = ref<string[]>([]);

const draftApi = ref<ThirdPartyApiConfig>(newDraftApi());
const draftSkill = ref<SkillSnippet>(newDraftSkill());
const editingApiId = ref<string | null>(null);
const editingSkillId = ref<string | null>(null);

const pendingAction = ref<PlannedAction | null>(null);
const pendingUserApiKey = ref('');
const pendingToolConfirm = ref<PendingToolConfirm | null>(null);
const renameSessionState = ref<SessionRenameState>({ id: '', title: '' });
const chatAbortController = ref<AbortController | null>(null);

const ttlOptions = [
  { label: 'Only this browser session', value: 0 },
  { label: '1 hour', value: 60 * 60 * 1000 },
  { label: '1 day', value: 24 * 60 * 60 * 1000 },
  { label: '7 days', value: 7 * 24 * 60 * 60 * 1000 },
  { label: '30 days', value: 30 * 24 * 60 * 60 * 1000 },
];

const shellai = computed(() => new ShellAIClient({
  baseUrl: () => apiBaseUrl.value,
  apiKey: () => apiKey.value,
  adminAuth: () => {
    const token = `${adminUsername.value}:${adminPassword.value}`;
    return btoa(token);
  },
}));

const pendingActionCommandPreview = computed(() => {
  if (!pendingAction.value) {
    return '';
  }
  return pendingAction.value.command.replaceAll('{{apiKey}}', pendingUserApiKey.value || '<api-key>');
});

const confirmDialogTitle = computed(() => {
  if (pendingAction.value?.api) {
    return 'Confirm API Action';
  }
  return 'Confirm Shell Command';
});

const confirmDialogDescription = computed(() => {
  if (pendingAction.value?.api) {
    return 'The agent generated a command based on your configured API. Confirm before execution.';
  }
  return 'You are about to execute a non-interactive shell command. Confirm before execution.';
});

const TOOL_OUTPUT_VISIBLE_LINES = 10;

function addTerminalLine(kind: TerminalKind, prefix: string, text: string): number {
  lineCounter += 1;
  terminalLines.value.push({
    id: lineCounter,
    kind,
    prefix,
    text,
    time: new Date().toLocaleTimeString(),
  });
  void nextTick(() => {
    terminalBody.value?.scrollTo({ top: terminalBody.value.scrollHeight, behavior: 'smooth' });
  });
  return lineCounter;
}

function updateLine(id: number, text: string): void {
  const target = terminalLines.value.find((line) => line.id === id);
  if (!target) {
    return;
  }
  target.text = text;
  void nextTick(() => {
    terminalBody.value?.scrollTo({ top: terminalBody.value.scrollHeight, behavior: 'smooth' });
  });
}

function clearTerminal(): void {
  terminalLines.value = [];
  expandedToolLineIds.value = [];
}

function isToolLine(line: TerminalLine): boolean {
  return line.prefix === 'TOOL';
}

function toolLineCount(text: string): number {
  return text.split(/\r?\n/).length;
}

function isToolLineCollapsible(line: TerminalLine): boolean {
  return isToolLine(line) && toolLineCount(line.text) > TOOL_OUTPUT_VISIBLE_LINES;
}

function isToolLineExpanded(lineId: number): boolean {
  return expandedToolLineIds.value.includes(lineId);
}

function visibleTerminalLineText(line: TerminalLine): string {
  if (!isToolLineCollapsible(line) || isToolLineExpanded(line.id)) {
    return line.text;
  }

  return line.text.split(/\r?\n/).slice(0, TOOL_OUTPUT_VISIBLE_LINES).join('\n');
}

function toggleToolLine(lineId: number): void {
  if (expandedToolLineIds.value.includes(lineId)) {
    expandedToolLineIds.value = expandedToolLineIds.value.filter((id) => id !== lineId);
    return;
  }
  expandedToolLineIds.value = [...expandedToolLineIds.value, lineId];
}

function addConsoleLine(kind: TerminalKind, prefix: string, text: string): number {
  consoleLineCounter += 1;
  consoleLines.value.push({
    id: consoleLineCounter,
    kind,
    prefix,
    text,
    time: new Date().toLocaleTimeString(),
  });
  void nextTick(() => {
    consoleBody.value?.scrollTo({ top: consoleBody.value.scrollHeight, behavior: 'smooth' });
  });
  return consoleLineCounter;
}

function resetConsoleCwd(): void {
  consoleCwd.value = '';
  addConsoleLine('system', 'CWD', 'Console cwd reset to server default.');
}

function normalizeCwd(value: string): string {
  return value.replace(/\\/g, '/');
}

function resolveCdCommandCwd(currentCwd: string, rawCommand: string): string | null {
  const match = rawCommand.trim().match(/^cd(?:\s+(.+))?$/i);
  if (!match) {
    return null;
  }

  let target = (match[1] ?? '').trim();
  if (!target || target === '.') {
    return normalizeCwd(currentCwd);
  }

  if ((target.startsWith('"') && target.endsWith('"')) || (target.startsWith("'") && target.endsWith("'"))) {
    target = target.slice(1, -1).trim();
  }

  target = normalizeCwd(target);

  if (/^[A-Za-z]:\//.test(target) || target.startsWith('/')) {
    return target;
  }

  const base = normalizeCwd(currentCwd || '');
  if (!base) {
    return null;
  }

  const parts = base.split('/').filter((x) => x.length > 0);
  for (const seg of target.split('/')) {
    if (!seg || seg === '.') {
      continue;
    }
    if (seg === '..') {
      parts.pop();
      continue;
    }
    parts.push(seg);
  }

  if (/^[A-Za-z]:/.test(base)) {
    const drive = base.slice(0, 2);
    return `${drive}/${parts.slice(1).join('/')}`;
  }
  return `/${parts.join('/')}`;
}

function formatTime(value: string): string {
  return new Date(value).toLocaleString();
}

function newDraftApi(): ThirdPartyApiConfig {
  return {
    id: crypto.randomUUID(),
    name: '',
    endpoint: '',
    description: '',
    waitForUserConfirm: true,
    needClientProvideApiKey: false,
    request: '',
    response: '',
    commandType: 'bash',
    commandTemplate: 'curl -sS "{{endpoint}}?q={{query}}"',
  };
}

function newDraftSkill(): SkillSnippet {
  return {
    id: crypto.randomUUID(),
    name: '',
    description: '',
    content: '',
    is_public: false,
  };
}

function saveSettings(): void {
  apiBaseUrl.value = settingsBaseUrl.value.trim();
  apiKey.value = settingsApiKey.value.trim();
  apiKeyVerified.value = false;

  saveApiBase(apiBaseUrl.value);
  if (apiKey.value) {
    saveApiKey(apiKey.value, apiKeyTtlPreset.value || undefined);
  } else {
    clearApiKey();
  }

  sessionStorage.setItem('shellai.admin.user', adminUsername.value);
  sessionStorage.setItem('shellai.admin.pass', adminPassword.value);

  showSettingsDialog.value = false;
  Notify.create({ type: 'positive', message: 'Settings saved' });
}

function setAdminFeedback(message: string, type: 'positive' | 'negative' | 'warning'): void {
  adminFeedback.value = message;
  adminFeedbackType.value = type;
}

async function copyToClipboard(text: string): Promise<void> {
  await navigator.clipboard.writeText(text);
  Notify.create({ type: 'positive', message: 'Copied to clipboard' });
}

async function verifyConnection(): Promise<void> {
  connecting.value = true;
  try {
    const me = await shellai.value.getMe();
    meInfo.value = me;
    apiKeyVerified.value = true;
    await refreshSessions();
    await refreshSkills();
    await refreshPublicSkills();
    Notify.create({ type: 'positive', message: `Connected as ${me.label}` });
  } catch (error) {
    apiKeyVerified.value = false;
    Notify.create({ type: 'negative', message: String(error) });
  } finally {
    connecting.value = false;
  }
}

async function refreshSessions(): Promise<void> {
  sessions.value = await shellai.value.listSessions();
  if (!activeSessionId.value && sessions.value.length > 0) {
    await selectSession(sessions.value[0]?.id ?? '');
  }
}

function makeSessionTitle(seed?: string): string {
  const trimmed = (seed ?? '').trim();
  if (!trimmed) {
    return `Session ${new Date().toLocaleString()}`;
  }

  const compact = trimmed.replace(/\s+/g, ' ');
  return compact.length > 48 ? `${compact.slice(0, 48)}...` : compact;
}

async function createSession(seedTitle?: string): Promise<void> {
  try {
    const session = await shellai.value.createSession(makeSessionTitle(seedTitle));
    sessions.value.unshift(session);
    await selectSession(session.id);
  } catch (error) {
    Notify.create({ type: 'negative', message: String(error) });
  }
}

function openRenameSessionDialog(session: SessionResponse): void {
  renameSessionState.value = {
    id: session.id,
    title: session.title,
  };
  showRenameDialog.value = true;
}

async function confirmRenameSession(): Promise<void> {
  const target = renameSessionState.value;
  if (!target.id) {
    return;
  }
  const title = target.title.trim();
  if (!title) {
    Notify.create({ type: 'warning', message: 'Session title cannot be empty.' });
    return;
  }

  try {
    const updated = await shellai.value.renameSession(target.id, title);
    sessions.value = sessions.value.map((session) => (session.id === updated.id ? updated : session));
    showRenameDialog.value = false;
  } catch (error) {
    Notify.create({ type: 'negative', message: String(error) });
  }
}

async function selectSession(id: string): Promise<void> {
  if (!id) {
    return;
  }
  try {
    activeSessionId.value = id;
    const detail = await shellai.value.getSession(id);
    clearTerminal();
    for (const msg of detail.messages) {
      if (msg.role === 'user') {
        addTerminalLine('user', 'USER', msg.content);
      } else if (msg.role === 'assistant') {
        addTerminalLine('assistant', 'AI', msg.content);
      } else if (msg.role === 'tool_call') {
        addTerminalLine('system', 'TOOL', `Tool call:\n${msg.content}`);
      } else if (msg.role === 'tool_result') {
        addTerminalLine('system', 'TOOL', `Tool result:\n${msg.content}`);
      } else {
        addTerminalLine('system', 'SYS', msg.content);
      }
    }
  } catch (error) {
    Notify.create({ type: 'negative', message: String(error) });
  }
}

async function deleteSession(id: string): Promise<void> {
  try {
    await shellai.value.deleteSession(id);
    sessions.value = sessions.value.filter((s) => s.id !== id);
    if (activeSessionId.value === id) {
      activeSessionId.value = '';
      clearTerminal();
    }
  } catch (error) {
    Notify.create({ type: 'negative', message: String(error) });
  }
}

function toggleSkill(id: string): void {
  if (activeSkillIds.value.includes(id)) {
    activeSkillIds.value = activeSkillIds.value.filter((x) => x !== id);
    return;
  }
  activeSkillIds.value = [...activeSkillIds.value, id];
}

function toggleApi(id: string): void {
  if (activeApiIds.value.includes(id)) {
    activeApiIds.value = activeApiIds.value.filter((x) => x !== id);
    return;
  }
  activeApiIds.value = [...activeApiIds.value, id];
}

function openAddApiDialog(): void {
  editingApiId.value = null;
  draftApi.value = newDraftApi();
  showApiDialog.value = true;
}

function openEditApiDialog(api: ThirdPartyApiConfig): void {
  editingApiId.value = api.id;
  draftApi.value = {
    ...api,
  };
  showApiDialog.value = true;
}

function addApi(): void {
  const payload = {
    ...draftApi.value,
    id: editingApiId.value ?? crypto.randomUUID(),
  };
  if (!payload.name || !payload.endpoint || !payload.commandTemplate) {
    Notify.create({ type: 'warning', message: 'Name, endpoint and command template are required.' });
    return;
  }

  if (editingApiId.value) {
    thirdPartyApis.value = thirdPartyApis.value.map((api) => (api.id === editingApiId.value ? payload : api));
  } else {
    thirdPartyApis.value = [payload, ...thirdPartyApis.value];
    activeApiIds.value = [payload.id, ...activeApiIds.value];
  }

  saveThirdPartyApis(thirdPartyApis.value);
  editingApiId.value = null;
  draftApi.value = newDraftApi();
  showApiDialog.value = false;
}

function removeApi(id: string): void {
  thirdPartyApis.value = thirdPartyApis.value.filter((api) => api.id !== id);
  activeApiIds.value = activeApiIds.value.filter((x) => x !== id);
  saveThirdPartyApis(thirdPartyApis.value);
}

async function addSkill(): Promise<void> {
  const payload = {
    ...draftSkill.value,
  };
  if (!payload.name || !payload.content) {
    Notify.create({ type: 'warning', message: 'Skill name and content are required.' });
    return;
  }

  try {
    if (editingSkillId.value) {
      const updated = await shellai.value.updateSkill(editingSkillId.value, {
        name: payload.name,
        description: payload.description,
        content: payload.content,
        is_public: payload.is_public,
      });
      skills.value = skills.value.map((skill) => (skill.id === editingSkillId.value ? updated : skill));
      await refreshPublicSkills();
    } else {
      const created = await shellai.value.createSkill({
        name: payload.name,
        description: payload.description,
        content: payload.content,
        is_public: payload.is_public,
      });
      skills.value = [created, ...skills.value];
    }

    editingSkillId.value = null;
    draftSkill.value = newDraftSkill();
    showSkillDialog.value = false;
  } catch (error) {
    Notify.create({ type: 'negative', message: String(error) });
  }
}

function openAddSkillDialog(): void {
  editingSkillId.value = null;
  draftSkill.value = newDraftSkill();
  showSkillDialog.value = true;
}

function openEditSkillDialog(skill: SkillSnippet): void {
  editingSkillId.value = skill.id;
  draftSkill.value = {
    ...skill,
  };
  showSkillDialog.value = true;
}

async function removeSkill(id: string): Promise<void> {
  try {
    await shellai.value.deleteSkill(id);
    skills.value = skills.value.filter((skill) => skill.id !== id);
    activeSkillIds.value = activeSkillIds.value.filter((x) => x !== id);
  } catch (error) {
    Notify.create({ type: 'negative', message: String(error) });
  }
}

async function toggleSkillVisibility(skill: SkillSnippet): Promise<void> {
  try {
    const updated = await shellai.value.updateSkill(skill.id, {
      is_public: !skill.is_public,
    });
    skills.value = skills.value.map((current) => (current.id === updated.id ? updated : current));
    if (updated.is_public) {
      Notify.create({ type: 'positive', message: `Skill "${updated.name}" is now public.` });
    } else {
      Notify.create({ type: 'positive', message: `Skill "${updated.name}" is now private.` });
    }
    await refreshPublicSkills();
  } catch (error) {
    Notify.create({ type: 'negative', message: String(error) });
  }
}

async function refreshSkills(): Promise<void> {
  const list = await shellai.value.listSkills();
  skills.value = list;
  const ids = new Set(list.map((skill) => skill.id));
  activeSkillIds.value = activeSkillIds.value.filter((id) => ids.has(id));
}

async function refreshPublicSkills(): Promise<void> {
  publicSkills.value = await shellai.value.listPublicSkills();
}

function planAction(userInput: string): PlannedAction | null {
  const trimmed = userInput.trim();
  if (!trimmed) {
    return null;
  }

  if (trimmed.startsWith('!sh ')) {
    return {
      command: trimmed.slice(4).trim(),
      requiresConfirmation: true,
      requiresUserApiKey: false,
    };
  }
  return null;
}

function cancelPendingAction(): void {
  pendingAction.value = null;
  pendingUserApiKey.value = '';
  showConfirmDialog.value = false;
}

async function confirmPendingAction(): Promise<void> {
  if (!pendingAction.value) {
    return;
  }
  const action = pendingAction.value;
  cancelPendingAction();
  await runShell(action.command.replaceAll('{{apiKey}}', pendingUserApiKey.value));
}

function denyPendingToolRequest(): void {
  if (!pendingToolConfirm.value || !activeSessionId.value) {
    return;
  }

  const payload = pendingToolConfirm.value;
  pendingToolConfirm.value = null;
  showToolConfirmDialog.value = false;

  void (async () => {
    try {
      await shellai.value.confirmToolCall(activeSessionId.value, payload.id, false);
    } catch (error) {
      addTerminalLine('system', 'SYS', `Failed to reject tool call: ${String(error)}`);
    }
  })();
}

async function approvePendingToolRequest(): Promise<void> {
  if (!pendingToolConfirm.value || !activeSessionId.value) {
    return;
  }

  const payload = pendingToolConfirm.value;
  pendingToolConfirm.value = null;
  showToolConfirmDialog.value = false;

  try {
    await shellai.value.confirmToolCall(activeSessionId.value, payload.id, true);
  } catch (error) {
    addTerminalLine('system', 'SYS', `Failed to approve tool call: ${String(error)}`);
  }
}

async function runShell(command: string): Promise<void> {
  addTerminalLine('system', 'PLAN', `Execute command:\n${command}`);
  await shellai.value.streamShell(command, {
    onStdout: ({ text }) => {
      addTerminalLine('stdout', 'OUT', text);
    },
    onStderr: ({ text }) => {
      addTerminalLine('stderr', 'ERR', text);
    },
    onExit: ({ code, error }) => {
      const suffix = error ? ` (${error})` : '';
      addTerminalLine('system', 'EXIT', `code=${code}${suffix}`);
    },
  });
}

async function runConsoleCommand(): Promise<void> {
  const command = consoleCommand.value.trim();
  if (!command || consoleBusy.value) {
    return;
  }

  consoleBusy.value = true;
  consoleCommand.value = '';
  addConsoleLine('system', 'CMD', command);

  try {
    const startCwd = consoleCwd.value;
    await shellai.value.streamShell(command, {
      onStdout: ({ text }) => {
        addConsoleLine('stdout', 'OUT', text);
      },
      onStderr: ({ text }) => {
        addConsoleLine('stderr', 'ERR', text);
      },
      onExit: ({ code, cwd, error }) => {
        if (cwd) {
          consoleCwd.value = normalizeCwd(cwd);
        }
        if (code === 0) {
          const resolved = resolveCdCommandCwd(consoleCwd.value || startCwd, command);
          if (resolved) {
            consoleCwd.value = resolved;
          }
        }
        const suffix = error ? ` (${error})` : '';
        const cwdSuffix = consoleCwd.value ? ` cwd=${consoleCwd.value}` : '';
        addConsoleLine('system', 'EXIT', `code=${code}${suffix}${cwdSuffix}`);
      },
    }, consoleCwd.value || undefined);
  } catch (error) {
    addConsoleLine('system', 'SYS', `Command failed: ${String(error)}`);
  } finally {
    consoleBusy.value = false;
  }
}

function interruptChat(): void {
  if (!isChatStreaming.value || !chatAbortController.value) {
    return;
  }

  chatAbortController.value.abort();
  addTerminalLine('system', 'INTERRUPT', 'Chat stream interrupted by user.');
}

function buildChatPrompt(raw: string): string {
  const picked = skills.value.filter((skill) => activeSkillIds.value.includes(skill.id));
  if (picked.length === 0) {
    return raw;
  }

  const skillText = picked.map((skill) => `# Skill: ${skill.name}\n${skill.content}`).join('\n\n');
  return `${skillText}\n\n# User Request\n${raw}`;
}

function selectedThirdPartyTools(): ChatToolDefinition[] {
  return thirdPartyApis.value
    .filter((tool) => activeApiIds.value.includes(tool.id))
    .map((tool) => ({
      name: tool.name,
      endpoint: tool.endpoint,
      description: tool.description,
      waitForUserConfirm: tool.waitForUserConfirm,
      needClientProvideApiKey: tool.needClientProvideApiKey,
      request: tool.request,
      response: tool.response,
      commandType: tool.commandType,
      commandTemplate: tool.commandTemplate,
    }));
}

async function sendPrompt(): Promise<void> {
  const input = prompt.value.trim();
  if (!input || busy.value) {
    return;
  }

  if (!activeSessionId.value) {
    await createSession(input);
    if (!activeSessionId.value) {
      Notify.create({ type: 'warning', message: 'Please create or select a session first.' });
      return;
    }
  }

  prompt.value = '';
  addTerminalLine('user', 'USER', input);

  const action = planAction(input);
  if (action) {
    if (action.api) {
      addTerminalLine('system', 'MATCH', `Matched API: ${action.api.name}`);
    }
    pendingAction.value = action;
    if (action.requiresConfirmation) {
      showConfirmDialog.value = true;
      return;
    }
    busy.value = true;
    try {
      await runShell(action.command);
    } catch (error) {
      addTerminalLine('system', 'SYS', `Command failed: ${String(error)}`);
    } finally {
      busy.value = false;
    }
    return;
  }

  busy.value = true;
  let aiLineId: number | null = null;
  const ensureAiLine = (): number => {
    if (aiLineId !== null) {
      return aiLineId;
    }
    aiLineId = addTerminalLine('assistant', 'AI', '');
    return aiLineId;
  };
  const finalPrompt = buildChatPrompt(input);
  chatAbortController.value = new AbortController();
  isChatStreaming.value = true;

  try {
    await shellai.value.streamChat(
      activeSessionId.value,
      finalPrompt,
      selectedModel.value,
      selectedThirdPartyTools(),
      {
        onPlan: ({ description }) => {
          addTerminalLine('system', 'PLAN', description);
        },
        onToolRequest: ({ id, tool, args, explanation }) => {
          pendingToolConfirm.value = {
            id,
            tool,
            ...(args ? { args } : {}),
            ...(explanation ? { explanation } : {}),
          };
          showToolConfirmDialog.value = true;
          addTerminalLine('system', 'TOOL', `Tool request: ${tool}${explanation ? ` — ${explanation}` : ''}`);
        },
        onToolResult: ({ tool, exit_code, stdout, stderr, rejected }) => {
          if (rejected) {
            addTerminalLine('system', 'TOOL', `Tool ${tool} was rejected.`);
            return;
          }
          const chunks = [`tool=${tool}`, `exit=${exit_code}`];
          if (stdout?.trim()) {
            chunks.push(`stdout:\n${stdout}`);
          }
          if (stderr?.trim()) {
            chunks.push(`stderr:\n${stderr}`);
          }
          addTerminalLine('system', 'TOOL', chunks.join('\n'));
        },
        onToolRejected: ({ tool }) => {
          addTerminalLine('system', 'TOOL', `Tool request rejected: ${tool}`);
        },
        onToken: ({ text }) => {
          const targetLineId = ensureAiLine();
          const current = terminalLines.value.find((line) => line.id === targetLineId)?.text ?? '';
          updateLine(targetLineId, current + text);
        },
        onDone: ({ content }) => {
          updateLine(ensureAiLine(), content);
          void refreshSessions();
        },
        onErrorEvent: ({ message, error_code }) => {
          updateLine(ensureAiLine(), `Error(${error_code}): ${message}`);
        },
      },
      chatAbortController.value.signal,
    );
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') {
      if (aiLineId === null) {
        aiLineId = addTerminalLine('assistant', 'AI', '[interrupted]');
      } else if (!(terminalLines.value.find((line) => line.id === aiLineId)?.text ?? '').trim()) {
        updateLine(aiLineId, '[interrupted]');
      }
    } else {
      updateLine(ensureAiLine(), `Request failed: ${String(error)}`);
    }
  } finally {
    isChatStreaming.value = false;
    chatAbortController.value = null;
    busy.value = false;
  }
}

async function loadAdminApiKeys(): Promise<void> {
  adminBusy.value = true;
  try {
    adminApiKeys.value = await shellai.value.listAPIKeys();
    setAdminFeedback(`Loaded ${adminApiKeys.value.length} API keys.`, 'positive');
    Notify.create({ type: 'positive', message: `Loaded ${adminApiKeys.value.length} API keys` });
  } catch (error) {
    setAdminFeedback(String(error), 'negative');
    Notify.create({ type: 'negative', message: String(error) });
  } finally {
    adminBusy.value = false;
  }
}

async function createAdminApiKey(): Promise<void> {
  const label = `ui-${new Date().toISOString()}`;
  adminBusy.value = true;
  try {
    const created = await shellai.value.createAPIKey(label);
    adminApiKeys.value = [created, ...adminApiKeys.value];
    lastCreatedApiKey.value = created.key ?? '';
    setAdminFeedback(`Created key for ${created.label}. The plaintext key is shown below once.`, 'positive');
    Notify.create({ type: 'positive', message: 'Created API key' });
  } catch (error) {
    setAdminFeedback(String(error), 'negative');
    Notify.create({ type: 'negative', message: String(error) });
  } finally {
    adminBusy.value = false;
  }
}

async function revokeAdminApiKey(id: string): Promise<void> {
  adminBusy.value = true;
  try {
    await shellai.value.revokeAPIKey(id);
    setAdminFeedback(`Revoked API key ${id}.`, 'positive');
    await loadAdminApiKeys();
  } catch (error) {
    setAdminFeedback(String(error), 'negative');
    Notify.create({ type: 'negative', message: String(error) });
  } finally {
    adminBusy.value = false;
  }
}

onMounted(async () => {
  if (!apiKey.value) {
    showSettingsDialog.value = true;
    return;
  }
  await verifyConnection();
});
</script>

<style scoped>
.shellai-page {
  height: calc(100vh - 58px);
  overflow: hidden;
  padding: 8px;
  box-sizing: border-box;
}

/* Outer wrapper: sidebar + panels */
.shellai-layout-outer {
  display: flex;
  flex-direction: row;
  height: 100%;
  gap: 0;
}

/* Icon sidebar */
.panel-sidebar {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  padding: 8px 4px;
  width: 44px;
  flex-shrink: 0;
  border-right: 1px solid rgba(81, 196, 255, 0.15);
  background: rgba(4, 16, 30, 0.6);
  border-radius: 12px 0 0 12px;
}

/* Panels area: all visible panels laid out horizontally */
.shellai-panels-area {
  display: flex;
  flex-direction: row;
  flex: 1;
  gap: 0;
  min-width: 0;
  height: 100%;
  padding: 0 4px 0 4px;
  overflow: hidden;
}

/* Drag resize handle between panels */
.resize-handle {
  width: 4px;
  flex-shrink: 0;
  cursor: col-resize;
  background: rgba(81, 196, 255, 0.12);
  border-radius: 2px;
  margin: 8px 2px;
  transition: background 0.15s;
}

.resize-handle:hover {
  background: rgba(81, 196, 255, 0.4);
}

.panel {
  border-radius: 14px;
  border: 1px solid rgba(57, 170, 237, 0.3);
  background: linear-gradient(145deg, rgba(2, 15, 33, 0.92), rgba(5, 24, 44, 0.9));
  box-shadow: 0 14px 36px rgba(0, 0, 0, 0.35);
  margin: 4px 2px;
}

.left-panel {
  overflow-y: auto;
  padding: 12px;
}

.panel-title {
  color: #89d8ff;
  text-transform: uppercase;
  letter-spacing: 0.18em;
  font-size: 12px;
  margin-bottom: 8px;
}

.card-block {
  background: rgba(4, 22, 39, 0.88);
  border-color: rgba(104, 216, 255, 0.2);
}

.tool-name {
  color: #f5fbff;
}

.tool-desc {
  color: #f5fbff !important;
  opacity: 0.95;
}

.session-list {
  max-height: 260px;
  overflow-y: auto;
}

.session-active {
  background: rgba(23, 110, 162, 0.5);
}

.terminal-panel {
  display: grid;
  grid-template-rows: auto 1fr auto;
  overflow: hidden;
}

.chat-panel,
.console-panel {
  min-width: 0;
}

.terminal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 12px;
  border-bottom: 1px solid rgba(96, 200, 255, 0.25);
}

.model-select {
  min-width: 160px;
}

.terminal-body {
  overflow-y: auto;
  padding: 12px;
  background:
    linear-gradient(rgba(11, 29, 48, 0.38), rgba(6, 14, 26, 0.92)),
    repeating-linear-gradient(0deg, rgba(88, 150, 205, 0.08) 0px, rgba(88, 150, 205, 0.08) 1px, transparent 1px, transparent 26px);
}

.terminal-line {
  font-family: 'JetBrains Mono', 'Cascadia Code', 'Fira Code', monospace;
  white-space: pre-wrap;
  margin-bottom: 8px;
  font-size: 13px;
  line-height: 1.45;
}

.line-time {
  color: #6d8ba5;
  margin-right: 8px;
}

.line-prefix {
  margin-right: 8px;
  font-weight: 700;
}

.kind-user .line-prefix { color: #4dd4ff; }
.kind-assistant .line-prefix { color: #95f6c2; }
.kind-stdout .line-prefix { color: #c1f8ff; }
.kind-stderr .line-prefix { color: #ff928e; }
.kind-system .line-prefix { color: #f4d57f; }

.kind-user .line-text { color: #9bddff; }
.kind-assistant .line-text { color: #d6ffe7; }
.kind-stdout .line-text { color: #dff8ff; }
.kind-stderr .line-text { color: #ffb2af; }
.kind-system .line-text { color: #f9e2ab; }

/* Markdown content rendered inside assistant lines */
.kind-assistant .line-markdown {
  display: inline;
  color: #d6ffe7;
  white-space: normal;
  font-family: inherit;
  font-size: inherit;
  line-height: inherit;
  vertical-align: top;
}

.kind-assistant:deep(.line-markdown) {
  display: inline-block;
  vertical-align: top;
  max-width: 100%;
  color: #d6ffe7;
  white-space: normal;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
  font-size: 13.5px;
  line-height: 1.6;
}

:deep(.line-markdown p) {
  margin: 0 0 0.5em 0;
}

:deep(.line-markdown p:last-child) {
  margin-bottom: 0;
}

:deep(.line-markdown h1),
:deep(.line-markdown h2),
:deep(.line-markdown h3),
:deep(.line-markdown h4) {
  color: #9ceeff;
  margin: 0.6em 0 0.3em;
  font-weight: 700;
  line-height: 1.3;
}

:deep(.line-markdown h1) { font-size: 1.3em; }
:deep(.line-markdown h2) { font-size: 1.15em; }
:deep(.line-markdown h3) { font-size: 1.05em; }

:deep(.line-markdown ul),
:deep(.line-markdown ol) {
  margin: 0.3em 0 0.5em 1.5em;
  padding: 0;
}

:deep(.line-markdown li) {
  margin: 0.1em 0;
}

:deep(.line-markdown code) {
  background: rgba(20, 50, 80, 0.7);
  border: 1px solid rgba(80, 180, 230, 0.25);
  border-radius: 4px;
  padding: 0.1em 0.4em;
  font-family: 'JetBrains Mono', 'Cascadia Code', 'Fira Code', monospace;
  font-size: 0.9em;
  color: #b8f0ff;
}

:deep(.line-markdown pre) {
  margin: 0.5em 0;
  border-radius: 8px;
  overflow-x: auto;
  border: 1px solid rgba(80, 180, 230, 0.2);
}

:deep(.line-markdown pre code) {
  background: none;
  border: none;
  padding: 0;
  font-size: 0.88em;
  color: inherit;
}

:deep(.line-markdown pre.hljs) {
  padding: 10px 14px;
  background: #0d1f30;
}

:deep(.line-markdown blockquote) {
  border-left: 3px solid rgba(81, 196, 255, 0.5);
  margin: 0.4em 0;
  padding: 0.2em 0.8em;
  color: #9acde8;
}

:deep(.line-markdown a) {
  color: #60cfff;
  text-decoration: underline;
}

:deep(.line-markdown table) {
  border-collapse: collapse;
  width: 100%;
  margin: 0.5em 0;
  font-size: 0.9em;
}

:deep(.line-markdown th),
:deep(.line-markdown td) {
  border: 1px solid rgba(81, 196, 255, 0.25);
  padding: 4px 10px;
  text-align: left;
}

:deep(.line-markdown th) {
  background: rgba(20, 50, 80, 0.5);
  color: #9ceeff;
}

:deep(.line-markdown hr) {
  border: none;
  border-top: 1px solid rgba(81, 196, 255, 0.25);
  margin: 0.6em 0;
}

.terminal-empty {
  color: #6c89a2;
  font-style: italic;
}

.terminal-input {
  border-top: 1px solid rgba(96, 200, 255, 0.25);
  padding: 12px;
}

.dialog-card {
  width: min(900px, 92vw);
  background: #0b1524;
}

@media (max-width: 900px) {
  .shellai-panels-area {
    flex-direction: column;
    overflow-y: auto;
  }

  .resize-handle {
    display: none;
  }

  .panel {
    min-height: 50vh;
    width: 100% !important;
    flex: none !important;
  }
}
</style>
