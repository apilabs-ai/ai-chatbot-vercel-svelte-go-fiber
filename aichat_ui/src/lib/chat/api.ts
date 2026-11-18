/**
 * Chat API client for the embedded AI chat backend.
 * Matches the normalized SSE contract: message.delta, message.completed, usage, error.
 */

const DEFAULT_HEADERS = {
	'Content-Type': 'application/json',
	Accept: 'text/event-stream',
};



export interface StreamChatRequest {
	input: string;
	conversation_id?: string;
	tenant_id?: string;
	workspace_id?: string;
	model?: string;
	/** Opt-in backend tools */
	tools?: Array<'web_search' | 'image_generation'>;
}

export interface ModelInfo {
	id: string;
	reasoning: boolean;
	default: boolean;
}

export interface CompletedInfo {
	conversation_id: string;
	/** Backend ids, used for checkpoint restore */
	user_message_id?: string;
	message_id?: string;
}

export interface ConversationListItem {
	id: string;
	tenant_id?: string;
	workspace_id?: string;
	user_id?: string;
	title?: string;
	created_at?: string;
	updated_at?: string;
}

export interface MessageListItem {
	id: string;
	conversation_id: string;
	role: 'user' | 'assistant';
	content: string;
	provider?: string;
	model?: string;
	input_tokens?: number;
	output_tokens?: number;
	created_at?: string;
}

// --- SSE event types (normalized contract) ---

export type SSEDeltaEvent = { delta: string };
export type SSECompletedEvent = CompletedInfo;
export type SSEReasoningDeltaEvent = { delta: string };
export type SSEUsageEvent = { input_tokens: number; output_tokens: number };
export type SSEErrorEvent = { message: string };

export type SSEEvent =
	| { event: 'message.delta'; data: SSEDeltaEvent }
	| { event: 'reasoning.delta'; data: SSEReasoningDeltaEvent }
	| { event: 'message.completed'; data: SSECompletedEvent }
	| { event: 'usage'; data: SSEUsageEvent }
	| { event: 'error'; data: SSEErrorEvent };

/** Structured part from backend (code, artifact, plan, queue, confirmation, sources, image) */
export interface StreamPart {
	type: string;
	content?: string;
	meta?: Record<string, unknown>;
}

export interface StreamCallbacks {
	onDelta?: (delta: string) => void;
	onPart?: (part: StreamPart) => void;
	onReasoningDelta?: (delta: string) => void;
	onCompleted?: (info: CompletedInfo) => void;
	onUsage?: (inputTokens: number, outputTokens: number) => void;
	onError?: (message: string) => void;
}

function ensureBaseUrl(url: string): string {
	const u = url.replace(/\/+$/, '');
	return u.includes('/v1') ? u : `${u}/v1`;
}

function authHeaders(authToken?: string): Record<string, string> {
	if (!authToken?.trim()) return {};
	return { Authorization: `Bearer ${authToken.trim()}` };
}
export interface ChatApiConfig {
	apiBaseUrl: string;
	authToken?: string;
	tenantId?: string;
	workspaceId?: string;
}

export async function streamChat(
	config: ChatApiConfig,
	request: StreamChatRequest,
	callbacks: StreamCallbacks
): Promise<void> {
	const base = ensureBaseUrl(config.apiBaseUrl);
	const url = `${base}/responses:stream`;
	const body: Record<string, unknown> = {
		input: request.input,
		...(request.conversation_id && { conversation_id: request.conversation_id }),
		...(request.model && { model: request.model }),
		...(request.tools?.length && { tools: request.tools }),
	};
	if (config.tenantId) body.tenant_id = config.tenantId;
	if (config.workspaceId) body.workspace_id = config.workspaceId;
	if (request.tenant_id) body.tenant_id = request.tenant_id;
	if (request.workspace_id) body.workspace_id = request.workspace_id;

	const res = await fetch(url, {
		method: 'POST',
		headers: { ...DEFAULT_HEADERS, ...authHeaders(config.authToken) },
		body: JSON.stringify(body),
	});

	if (!res.ok) {
		let errMessage = `Request failed: ${res.status}`;
		try {
			const j = await res.json();
			if (j?.message) errMessage = j.message;
			else if (j?.error) errMessage = typeof j.error === 'string' ? j.error : JSON.stringify(j.error);
		} catch {
			const t = await res.text();
			if (t) errMessage = t.slice(0, 200);
		}
		callbacks.onError?.(errMessage);
		return;
	}

	const reader = res.body?.getReader();
	if (!reader) {
		callbacks.onError?.('No response body');
		return;
	}

	const decoder = new TextDecoder();
	let buffer = '';
	// Must survive across reads: a large data line (e.g. a base64 image) can
	// arrive in a later chunk than its "event:" line.
	let currentEvent: string | null = null;

	try {
		while (true) {
			const { done, value } = await reader.read();
			if (done) break;
			buffer += decoder.decode(value, { stream: true });
			const lines = buffer.split(/\n/);
			buffer = lines.pop() ?? '';

			for (const line of lines) {
				if (line.startsWith('event:')) {
					currentEvent = line.slice(6).trim();
				} else if (line.startsWith('data:') && currentEvent) {
					const raw = line.slice(5).trim();
					if (raw === '[DONE]' || !raw) continue;
					try {
						const data = JSON.parse(raw) as Record<string, unknown>;
						switch (currentEvent) {
							case 'message.delta':
								if (typeof data.delta === 'string') callbacks.onDelta?.(data.delta);
								break;
							case 'part':
								if (typeof data.type === 'string')
									callbacks.onPart?.({
										type: data.type,
										content: typeof data.content === 'string' ? data.content : undefined,
										meta: data.meta && typeof data.meta === 'object' ? (data.meta as Record<string, unknown>) : undefined
									});
								break;
							case 'reasoning.delta':
								if (typeof data.delta === 'string') callbacks.onReasoningDelta?.(data.delta);
								break;
							case 'message.completed':
								if (typeof data.conversation_id === 'string')
									callbacks.onCompleted?.(data as unknown as CompletedInfo);
								break;
							case 'usage':
								callbacks.onUsage?.(
									Number(data.input_tokens) || 0,
									Number(data.output_tokens) || 0
								);
								break;
							case 'error':
								callbacks.onError?.(typeof data.message === 'string' ? data.message : 'Unknown error');
								break;
						}
					} catch {
						// skip malformed data line
					}
					currentEvent = null;
				}
			}
		}
	} finally {
		reader.releaseLock();
	}
}

/**
 * GET /v1/conversations — list conversations for the authenticated context.
 */
export async function getConversations(
	config: ChatApiConfig
): Promise<ConversationListItem[]> {
	const base = ensureBaseUrl(config.apiBaseUrl);
	const url = config.tenantId
		? `${base}/conversations?tenant_id=${encodeURIComponent(config.tenantId)}`
		: `${base}/conversations`;

	const res = await fetch(url, {
		method: 'GET',
		headers: { ...authHeaders(config.authToken) },
	});

	if (!res.ok) {
		if (res.status === 401 || res.status === 403) return [];
		throw new Error(`Failed to load conversations: ${res.status}`);
	}

	const data = await res.json();
	if (Array.isArray(data)) return data as ConversationListItem[];
	if (data?.conversations && Array.isArray(data.conversations))
		return data.conversations as ConversationListItem[];
	return [];
}

/**
 * GET /v1/conversations/:id/messages — load messages for a conversation.
 */
export async function getConversationMessages(
	config: ChatApiConfig,
	conversationId: string
): Promise<MessageListItem[]> {
	const base = ensureBaseUrl(config.apiBaseUrl);
	const url = `${base}/conversations/${encodeURIComponent(conversationId)}/messages`;

	const res = await fetch(url, {
		method: 'GET',
		headers: { ...authHeaders(config.authToken) },
	});

	if (!res.ok) {
		if (res.status === 404) return [];
		throw new Error(`Failed to load messages: ${res.status}`);
	}

	const data = await res.json();
	if (Array.isArray(data)) return data as MessageListItem[];
	if (data?.messages && Array.isArray(data.messages))
		return data.messages as MessageListItem[];
	return [];
}

/**
 * GET /v1/models — allow-listed models the backend accepts.
 */
export async function getModels(config: ChatApiConfig): Promise<ModelInfo[]> {
	const res = await fetch(`${ensureBaseUrl(config.apiBaseUrl)}/models`, {
		headers: { ...authHeaders(config.authToken) },
	});
	if (!res.ok) throw new Error(`Failed to load models: ${res.status}`);
	const data = await res.json();
	return Array.isArray(data) ? (data as ModelInfo[]) : [];
}

/**
 * POST /v1/conversations/:id/restore — drop every message after messageId.
 */
export async function restoreConversation(
	config: ChatApiConfig,
	conversationId: string,
	messageId: string
): Promise<void> {
	const url = `${ensureBaseUrl(config.apiBaseUrl)}/conversations/${encodeURIComponent(conversationId)}/restore`;
	const res = await fetch(url, {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json',
			...authHeaders(config.authToken),
			...(config.tenantId && { 'X-Tenant-ID': config.tenantId }),
		},
		body: JSON.stringify({ message_id: messageId }),
	});
	if (!res.ok) throw new Error(`Failed to restore checkpoint: ${res.status}`);
}
