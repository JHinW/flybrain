var fs = require('fs');
var path = require('path');
var Anthropic = require('@anthropic-ai/sdk');

var envLoaded = false;

function loadEnv() {
  if (envLoaded) return;
  envLoaded = true;

  var envPath = path.resolve(__dirname, '..', '.env');
  if (fs.existsSync(envPath)) {
    if (typeof process.loadEnvFile === 'function') {
      try {
        process.loadEnvFile(envPath);
        return;
      } catch (e) {
        // Fall back to manual parsing if process.loadEnvFile fails
      }
    }
    try {
      var content = fs.readFileSync(envPath, 'utf8');
      var lines = content.split(/\r?\n/);
      for (var i = 0; i < lines.length; i++) {
        var line = lines[i].trim();
        if (!line || line.startsWith('#')) continue;
        var eq = line.indexOf('=');
        if (eq !== -1) {
          var key = line.slice(0, eq).trim();
          var val = line.slice(eq + 1).trim();
          if ((val.startsWith('"') && val.endsWith('"')) || (val.startsWith("'") && val.endsWith("'"))) {
            val = val.slice(1, -1);
          }
          if (process.env[key] === undefined) {
            process.env[key] = val;
          }
        }
      }
    } catch (err) {
      process.stderr.write('[ai] Warning: could not parse .env file: ' + err.message + '\n');
    }
  }
}

/**
 * Normalizes custom base URLs.
 * The Anthropic SDK appends `/v1/messages` to `baseURL`.
 * If a user provides an endpoint ending in `/v1`, `/v1/`, or `/v1/messages`,
 * strip that suffix so the SDK doesn't generate `/v1/v1/messages`.
 */
function normalizeBaseURL(url) {
  if (!url || typeof url !== 'string') return undefined;
  var trimmed = url.trim();
  if (!trimmed) return undefined;
  trimmed = trimmed.replace(/\/+$/, '');
  trimmed = trimmed.replace(/\/v1\/messages$/, '');
  trimmed = trimmed.replace(/\/v1$/, '');
  return trimmed || undefined;
}

/**
 * Parse custom headers from either a JSON string or comma-separated Key=Value pairs.
 */
function parseCustomHeaders(headersInput) {
  if (!headersInput) return undefined;
  if (typeof headersInput === 'object') return headersInput;
  try {
    return JSON.parse(headersInput);
  } catch (e) {
    var headers = {};
    var pairs = headersInput.split(',');
    for (var i = 0; i < pairs.length; i++) {
      var pair = pairs[i].trim();
      var eq = pair.indexOf('=');
      if (eq !== -1) {
        var k = pair.slice(0, eq).trim();
        var v = pair.slice(eq + 1).trim();
        if (k) headers[k] = v;
      }
    }
    return Object.keys(headers).length > 0 ? headers : undefined;
  }
}

/**
 * Resolves AI configuration from environment variables and optional overrides.
 */
function getConfig(overrides) {
  overrides = overrides || {};
  if (!overrides.skipEnv) {
    loadEnv();
  }

  var rawBaseURL = overrides.baseURL ||
    process.env.ANTHROPIC_BASE_URL ||
    process.env.ANTHROPIC_ENDPOINT ||
    process.env.ANTHROPIC_API_URL ||
    process.env.AI_BASE_URL ||
    process.env.CUSTOM_AI_ENDPOINT ||
    process.env.CUSTOM_AI_BASE_URL;

  var baseURL = normalizeBaseURL(rawBaseURL);

  var apiKey = overrides.apiKey ||
    process.env.ANTHROPIC_API_KEY ||
    process.env.AI_API_KEY ||
    process.env.CUSTOM_AI_API_KEY ||
    (baseURL ? 'custom-endpoint-key' : undefined);

  var authToken = overrides.authToken ||
    process.env.ANTHROPIC_AUTH_TOKEN ||
    process.env.AI_AUTH_TOKEN;

  var customHeaders = overrides.defaultHeaders ||
    parseCustomHeaders(process.env.ANTHROPIC_CUSTOM_HEADERS || process.env.AI_CUSTOM_HEADERS);

  var chatModel = overrides.chatModel ||
    process.env.ANTHROPIC_CHAT_MODEL ||
    process.env.ANTHROPIC_MODEL ||
    process.env.AI_MODEL ||
    'claude-sonnet-4-20250514';

  var agentModel = overrides.agentModel ||
    process.env.ANTHROPIC_AGENT_MODEL ||
    process.env.ANTHROPIC_MODEL ||
    process.env.AI_MODEL ||
    'claude-3-5-haiku-20241022';

  var maxTokens = parseInt(
    overrides.maxTokens ||
    process.env.ANTHROPIC_MAX_TOKENS ||
    process.env.AI_MAX_TOKENS ||
    '2048',
    10
  ) || 2048;

  return {
    rawBaseURL: rawBaseURL,
    baseURL: baseURL,
    apiKey: apiKey,
    authToken: authToken,
    customHeaders: customHeaders,
    chatModel: chatModel,
    agentModel: agentModel,
    maxTokens: maxTokens,
    isCustomEndpoint: Boolean(baseURL)
  };
}

/**
 * Creates an Anthropic client instance with custom endpoint configuration.
 */
function createClient(options) {
  var config = getConfig(options);
  var clientOptions = {};

  if (config.baseURL) {
    clientOptions.baseURL = config.baseURL;
  }
  if (config.apiKey) {
    clientOptions.apiKey = config.apiKey;
  }
  if (config.authToken) {
    clientOptions.authToken = config.authToken;
  }
  if (config.customHeaders) {
    clientOptions.defaultHeaders = config.customHeaders;
  }
  if (options && options.fetch) {
    clientOptions.fetch = options.fetch;
  }

  return new Anthropic(clientOptions);
}

var defaultClient = null;

function getClient() {
  if (!defaultClient) {
    defaultClient = createClient();
  }
  return defaultClient;
}

/**
 * Extracts the assistant's message text from an Anthropic message response.
 * Handles models with thinking blocks (DeepSeek R1 / Flash, Claude 3.7) where
 * the text block is preceded by a thinking block.
 */
function extractText(response) {
  if (!response) return '';
  var content = response.content;
  if (typeof content === 'string') return content;
  if (Array.isArray(content)) {
    var textParts = [];
    for (var i = 0; i < content.length; i++) {
      var item = content[i];
      if (item && item.type === 'text' && typeof item.text === 'string') {
        textParts.push(item.text);
      }
    }
    if (textParts.length > 0) {
      return textParts.join('\n');
    }
    for (var j = 0; j < content.length; j++) {
      if (content[j] && typeof content[j].text === 'string') return content[j].text;
      if (content[j] && typeof content[j].thinking === 'string') return content[j].thinking;
    }
  }
  return '';
}

module.exports = {
  loadEnv: loadEnv,
  normalizeBaseURL: normalizeBaseURL,
  parseCustomHeaders: parseCustomHeaders,
  getConfig: getConfig,
  createClient: createClient,
  getClient: getClient,
  extractText: extractText
};
