var assert = require('assert');
var ai = require('../server/ai');
var decide = require('../agent/decide');

console.log('Running AI custom endpoint tests...\n');

var passed = 0;
var total = 0;

function test(name, fn) {
  total++;
  try {
    fn();
    passed++;
    console.log('  PASS: ' + name);
  } catch (err) {
    console.error('  FAIL: ' + name);
    console.error('    ' + err.message);
  }
}

// 1. URL Normalization
test('normalizeBaseURL strips trailing slashes', function() {
  assert.strictEqual(ai.normalizeBaseURL('https://api.example.com/'), 'https://api.example.com');
  assert.strictEqual(ai.normalizeBaseURL('https://api.example.com///'), 'https://api.example.com');
});

test('normalizeBaseURL normalizes /v1 and /v1/ suffixes', function() {
  assert.strictEqual(ai.normalizeBaseURL('https://api.example.com/v1'), 'https://api.example.com');
  assert.strictEqual(ai.normalizeBaseURL('https://api.example.com/v1/'), 'https://api.example.com');
  assert.strictEqual(ai.normalizeBaseURL('http://localhost:8000/proxy/v1'), 'http://localhost:8000/proxy');
});

test('normalizeBaseURL strips /v1/messages', function() {
  assert.strictEqual(ai.normalizeBaseURL('https://api.example.com/v1/messages'), 'https://api.example.com');
});

test('normalizeBaseURL handles falsy and empty values', function() {
  assert.strictEqual(ai.normalizeBaseURL(''), undefined);
  assert.strictEqual(ai.normalizeBaseURL(null), undefined);
  assert.strictEqual(ai.normalizeBaseURL(undefined), undefined);
});

// 2. Custom Headers Parsing
test('parseCustomHeaders parses JSON strings', function() {
  var headers = ai.parseCustomHeaders('{"X-Custom": "123", "Authorization": "Bearer abc"}');
  assert.deepStrictEqual(headers, { "X-Custom": "123", "Authorization": "Bearer abc" });
});

test('parseCustomHeaders parses key=value strings', function() {
  var headers = ai.parseCustomHeaders('X-Custom=123, X-Title=FlyBrain');
  assert.deepStrictEqual(headers, { "X-Custom": "123", "X-Title": "FlyBrain" });
});

test('parseCustomHeaders handles invalid inputs gracefully', function() {
  assert.strictEqual(ai.parseCustomHeaders(''), undefined);
  assert.strictEqual(ai.parseCustomHeaders(null), undefined);
});

// 3. Configuration & Fallbacks
test('getConfig respects custom endpoint and provides fallback key if none set', function() {
  var origBase = process.env.ANTHROPIC_BASE_URL;
  var origKey = process.env.ANTHROPIC_API_KEY;
  var origAiKey = process.env.AI_API_KEY;
  var origCustomKey = process.env.CUSTOM_AI_API_KEY;
  try {
    delete process.env.ANTHROPIC_API_KEY;
    delete process.env.AI_API_KEY;
    delete process.env.CUSTOM_AI_API_KEY;
    process.env.ANTHROPIC_BASE_URL = 'http://localhost:11434/v1';

    var cfg = ai.getConfig({ skipEnv: true });
    assert.strictEqual(cfg.isCustomEndpoint, true);
    assert.strictEqual(cfg.baseURL, 'http://localhost:11434');
    assert.strictEqual(cfg.apiKey, 'custom-endpoint-key');
  } finally {
    if (origBase !== undefined) process.env.ANTHROPIC_BASE_URL = origBase;
    else delete process.env.ANTHROPIC_BASE_URL;
    if (origKey !== undefined) process.env.ANTHROPIC_API_KEY = origKey;
    if (origAiKey !== undefined) process.env.AI_API_KEY = origAiKey;
    if (origCustomKey !== undefined) process.env.CUSTOM_AI_API_KEY = origCustomKey;
  }
});

test('getConfig respects model overrides (chat and agent)', function() {
  var origChat = process.env.ANTHROPIC_CHAT_MODEL;
  var origAgent = process.env.ANTHROPIC_AGENT_MODEL;
  var origModel = process.env.ANTHROPIC_MODEL;
  try {
    process.env.ANTHROPIC_MODEL = 'custom-general-model';
    process.env.ANTHROPIC_CHAT_MODEL = 'custom-chat-model';
    process.env.ANTHROPIC_AGENT_MODEL = 'custom-agent-model';

    var cfg = ai.getConfig();
    assert.strictEqual(cfg.chatModel, 'custom-chat-model');
    assert.strictEqual(cfg.agentModel, 'custom-agent-model');
  } finally {
    if (origChat !== undefined) process.env.ANTHROPIC_CHAT_MODEL = origChat;
    else delete process.env.ANTHROPIC_CHAT_MODEL;
    if (origAgent !== undefined) process.env.ANTHROPIC_AGENT_MODEL = origAgent;
    else delete process.env.ANTHROPIC_AGENT_MODEL;
    if (origModel !== undefined) process.env.ANTHROPIC_MODEL = origModel;
    else delete process.env.ANTHROPIC_MODEL;
  }
});

// 4. Custom Client Fetch Dispatch
test('createClient routes to custom endpoint /v1/messages', async function() {
  var requestedUrl = null;
  var requestedHeaders = null;
  var mockFetch = async function(url, init) {
    requestedUrl = url;
    requestedHeaders = init && init.headers;
    return new Response(JSON.stringify({
      id: 'msg_test',
      type: 'message',
      role: 'assistant',
      content: [{ type: 'text', text: '{"action":"wait","reasoning":"test"}' }]
    }), { status: 200, headers: { 'Content-Type': 'application/json' } });
  };

  var client = ai.createClient({
    baseURL: 'http://custom-ai.internal:9000/v1',
    apiKey: 'my-secret-key',
    defaultHeaders: { 'X-Proxy-Route': 'node-1' },
    fetch: mockFetch
  });

  var res = await client.messages.create({
    model: 'custom-model',
    max_tokens: 100,
    messages: [{ role: 'user', content: 'test' }]
  });

  assert.strictEqual(requestedUrl, 'http://custom-ai.internal:9000/v1/messages');
  assert.strictEqual(res.content[0].text, '{"action":"wait","reasoning":"test"}');
});

// 5. Agent cleanJsonText
test('cleanJsonText cleans markdown and raw JSON', function() {
  assert.strictEqual(decide.cleanJsonText('{"action":"wait"}'), '{"action":"wait"}');
  assert.strictEqual(decide.cleanJsonText('```json\n{"action":"wait"}\n```'), '{"action":"wait"}');
  assert.strictEqual(decide.cleanJsonText('```\n{"action":"wait"}\n```'), '{"action":"wait"}');
});

setTimeout(function() {
  console.log('\nResult: ' + passed + ' passed / ' + (total - passed) + ' failed / ' + total + ' total');
  if (passed !== total) {
    process.exit(1);
  }
}, 100);
