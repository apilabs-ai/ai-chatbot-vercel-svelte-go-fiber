import { test } from 'node:test';
import assert from 'node:assert/strict';
import { streamChat, type StreamPart } from '../src/lib/chat/api.ts';

function sseResponse(chunks: string[]): Response {
	const enc = new TextEncoder();
	return new Response(
		new ReadableStream({
			start(c) {
				for (const ch of chunks) c.enqueue(enc.encode(ch));
				c.close();
			},
		}),
		{ status: 200 }
	);
}

test('event line and its data line split across reads', async () => {
	const big = 'A'.repeat(50_000);
	globalThis.fetch = async () =>
		sseResponse([
			'event: reasoning.delta\ndata: {"delta":"hmm"}\n\nevent: part\n',
			`data: {"type":"image","content":"${big.slice(0, 10)}`,
			`${big.slice(10)}","meta":{"media_type":"image/png"}}\n\n`,
			'event: message.completed\ndata: {"conversation_id":"c1","message_id":"m2","user_message_id":"m1"}\n\n',
		]);

	const parts: StreamPart[] = [];
	let reasoning = '';
	let completed: unknown;
	await streamChat({ apiBaseUrl: 'http://x' }, { input: 'hi' }, {
		onPart: (p) => parts.push(p),
		onReasoningDelta: (d) => (reasoning += d),
		onCompleted: (c) => (completed = c),
	});

	assert.equal(reasoning, 'hmm');
	assert.equal(parts.length, 1);
	assert.equal(parts[0].type, 'image');
	assert.equal(parts[0].content?.length, big.length);
	assert.deepEqual(completed, { conversation_id: 'c1', message_id: 'm2', user_message_id: 'm1' });
});
